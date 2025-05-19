package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"regexp"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var HashPassword func(password string) (string, error) = hashPassword
var IsPasswordValid func(password, original string) bool = isPasswordValid

func hashPassword(password string) (string, error) {
	// Generate a bcrypt hash
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	// Convert the hashed bytes to a string
	hashedPassword := string(hashedBytes)

	return hashedPassword, nil
}

func isPasswordValid(password, original string) bool {
	// Compare the password with the hash
	err := bcrypt.CompareHashAndPassword([]byte(original), []byte(password))
	return err == nil
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

// IsValidEmail checks if an email address is valid
func IsValidEmail(email string) bool {
	if len(email) == 0 {
		return false
	}
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

// PaginateAndSort prepares a query with pagination and sorting
func PaginateAndSort(query *gorm.DB, pageSize int, pageNumber int, skipPagination bool, sort string) (*gorm.DB, int64, int, error) {
	var totalCount int64

	// Apply sorting
	if sort != "" {
		if sort[0] == '-' {
			query = query.Order(sort[1:] + " DESC")
		} else {
			query = query.Order(sort + " ASC")
		}
	} else {
		query = query.Order("id ASC")
	}

	// Count total records
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	// Calculate total pages
	var totalPages int
	if pageSize > 0 {
		totalPages = int(totalCount) / pageSize
		if int(totalCount)%pageSize != 0 {
			totalPages++
		}
	}

	// Apply pagination
	if !skipPagination && pageSize > 0 {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	return query, totalCount, totalPages, nil
}
