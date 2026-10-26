package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/pwhash"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

var HashPassword func(password string) (string, error) = hashPassword
var IsPasswordValid func(password, original string) bool = isPasswordValid

// PasswordHashCost returns the bcrypt cost that password and client-secret
// hashes are created with in standard builds (pkg/pwhash owns it). It is
// bcrypt.DefaultCost in every binary; only a test binary can lower it,
// through UseMinPasswordCostForTests.
func PasswordHashCost() int { return pwhash.BcryptCost() }

// UseMinPasswordCostForTests lowers the bcrypt cost to bcrypt.MinCost. One
// DefaultCost hash takes ~0.8 s under -race, and most test harnesses create a
// user. Verification reads the cost from the hash itself, so tests that check
// a hash still pass. It panics outside a test binary, so production can never
// lower the cost. (PBKDF2 hashes, written in FIPS mode, already use a low
// iteration count in test binaries.)
func UseMinPasswordCostForTests() {
	if !testing.Testing() {
		panic("models.UseMinPasswordCostForTests called outside a test binary")
	}
	pwhash.UseMinBcryptCostForTests()
}

// hashPassword hashes with bcrypt in standard builds and PBKDF2-HMAC-SHA256
// in FIPS 140-3 mode (pkg/pwhash).
func hashPassword(password string) (string, error) {
	return pwhash.Hash(password)
}

// isPasswordValid accepts either hash format in every build.
func isPasswordValid(password, original string) bool {
	return pwhash.Verify(password, original)
}

// JSONMap is a custom type for map[string]interface{} to implement sql.Scanner and driver.Valuer
type JSONMap map[string]interface{}

// Implement the sql.Scanner interface for JSONMap
func (j *JSONMap) Scan(value interface{}) error {
	return JSONScan(value, j)
}

// Implement the driver.Valuer interface for JSONMap
func (j JSONMap) Value() (driver.Value, error) {
	return JSONValue(j)
}

// StringMap is a custom type for map[string]string to implement sql.Scanner and driver.Valuer
type StringMap map[string]string

// Implement the sql.Scanner interface for StringMap
func (s *StringMap) Scan(value interface{}) error {
	return JSONScan(value, s)
}

// Implement the driver.Valuer interface for StringMap
func (s StringMap) Value() (driver.Value, error) {
	return JSONValue(s)
}

// JSONValue is a helper function for implementing driver.Valuer for JSON types
func JSONValue(v interface{}) (driver.Value, error) {
	if v == nil {
		return nil, nil
	}

	jsonData, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return string(jsonData), nil
}

// JSONScan is a helper function for implementing sql.Scanner for JSON types
func JSONScan(value, dest interface{}) error {
	switch v := value.(type) {
	case []byte:
		if err := json.Unmarshal(v, dest); err != nil {
			return fmt.Errorf("failed to unmarshal JSON value: %v", err)
		}
	case string:
		if err := json.Unmarshal([]byte(v), dest); err != nil {
			return fmt.Errorf("failed to unmarshal JSON value: %v", err)
		}
	default:
		return fmt.Errorf("failed to unmarshal JSON value: %v", value)
	}

	return nil
}

func SameIDs(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}

	count := make(map[uint]int)
	for _, id := range a {
		count[id]++
	}

	for _, id := range b {
		count[id]--
	}

	for id := range count {
		if count[id] != 0 {
			return false
		}
	}

	return true
}

func PaginateAndSort(query *gorm.DB, pageSize int, pageNumber int, skipPagination bool, sort string) (*gorm.DB, int64, int, error) {
	var totalCount int64

	if sort != "" {
		if sort[0] == '-' {
			query = query.Order(sort[1:] + " DESC")
		} else {
			query = query.Order(sort + " ASC")
		}
	} else {
		query = query.Order("id ASC")
	}

	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	var totalPages int
	if pageSize > 0 {
		totalPages = int(totalCount) / pageSize
		if int(totalCount)%pageSize != 0 {
			totalPages++
		}
	}

	if !skipPagination && pageSize > 0 {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	return query, totalCount, totalPages, nil
}
