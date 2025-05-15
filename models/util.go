package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/bcrypt"
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
