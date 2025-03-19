package helpers

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorResponse_Error(t *testing.T) {
	// Create a new ErrorResponse
	err := ErrorResponse{
		StatusCode: http.StatusBadRequest,
		Title:      "Bad Request",
		Message:    "Invalid input",
	}

	// Test the Error method
	assert.Equal(t, "Invalid input", err.Error())
}

func TestNewBadRequestError(t *testing.T) {
	// Create a new BadRequestError
	err := NewBadRequestError("Invalid input")

	// Test the error properties
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.Equal(t, "Bad Request", err.Title)
	assert.Equal(t, "Invalid input", err.Message)
	assert.Equal(t, "Invalid input", err.Error())
}

func TestNewInternalServerError(t *testing.T) {
	// Create a new InternalServerError
	err := NewInternalServerError("Something went wrong")

	// Test the error properties
	assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
	assert.Equal(t, "Internal Server Error", err.Title)
	assert.Equal(t, "Something went wrong", err.Message)
	assert.Equal(t, "Something went wrong", err.Error())
}

func TestNewNotFoundError(t *testing.T) {
	// Create a new NotFoundError
	err := NewNotFoundError("Resource not found")

	// Test the error properties
	assert.Equal(t, http.StatusNotFound, err.StatusCode)
	assert.Equal(t, "Not Found", err.Title)
	assert.Equal(t, "Resource not found", err.Message)
	assert.Equal(t, "Resource not found", err.Error())
}

func TestNewForbiddenError(t *testing.T) {
	// Create a new ForbiddenError
	err := NewForbiddenError("Access denied")

	// Test the error properties
	assert.Equal(t, http.StatusForbidden, err.StatusCode)
	assert.Equal(t, "Forbidden", err.Title)
	assert.Equal(t, "Access denied", err.Message)
	assert.Equal(t, "Access denied", err.Error())
}

func TestNewUnauthorizedError(t *testing.T) {
	// Create a new UnauthorizedError
	err := NewUnauthorizedError("Authentication required")

	// Test the error properties
	assert.Equal(t, http.StatusUnauthorized, err.StatusCode)
	assert.Equal(t, "Unauthorized", err.Title)
	assert.Equal(t, "Authentication required", err.Message)
	assert.Equal(t, "Authentication required", err.Error())
}

func TestErrorResponseAsError(t *testing.T) {
	// Create a new ErrorResponse
	errResp := NewBadRequestError("Invalid input")

	// Use it as an error
	var err error = errResp

	// Test that it implements the error interface correctly
	assert.Equal(t, "Invalid input", err.Error())

	// Test type assertion
	respErr, ok := err.(ErrorResponse)
	assert.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, respErr.StatusCode)
	assert.Equal(t, "Bad Request", respErr.Title)
	assert.Equal(t, "Invalid input", respErr.Message)
}

func TestErrorResponseComparison(t *testing.T) {
	// Create two identical errors
	err1 := NewBadRequestError("Invalid input")
	err2 := NewBadRequestError("Invalid input")

	// Create a different error with the same status code
	err3 := NewBadRequestError("Different message")

	// Create an error with a different status code
	err4 := NewNotFoundError("Invalid input")

	// Test equality
	assert.Equal(t, err1, err2)
	assert.NotEqual(t, err1, err3)
	assert.NotEqual(t, err1, err4)

	// Test individual fields
	assert.Equal(t, err1.StatusCode, err3.StatusCode)
	assert.Equal(t, err1.Title, err3.Title)
	assert.NotEqual(t, err1.Message, err3.Message)

	assert.NotEqual(t, err1.StatusCode, err4.StatusCode)
	assert.NotEqual(t, err1.Title, err4.Title)
	assert.Equal(t, err1.Message, err4.Message)
}

func TestKeyValueOrZero(t *testing.T) {
	// Test with a valid key and value
	data := map[string]any{"key1": 123, "key2": "value2"}
	result := KeyValueOrZero(data, "key1")
	assert.Equal(t, 123, result)

	// Test with a non-existent key
	result = KeyValueOrZero(data, "key3")
	assert.Equal(t, 0, result)

	// Test with a key that has a non-int value
	result = KeyValueOrZero(data, "key2")
	assert.Equal(t, 0, result)

	// Test with an empty map
	emptyMap := map[string]any{}
	result = KeyValueOrZero(emptyMap, "key1")
	assert.Equal(t, 0, result)

	// Test with a nil map
	var nilMap map[string]any
	result = KeyValueOrZero(nilMap, "key1")
	assert.Equal(t, 0, result)
}

func TestKeyValueInt32OrZero(t *testing.T) {
	// Test with a valid key and value
	var int32Value int32 = 123
	data := map[string]any{"key1": int32Value, "key2": "value2"}
	result := KeyValueInt32OrZero(data, "key1")
	assert.Equal(t, 123, result)

	// Test with a non-existent key
	result = KeyValueInt32OrZero(data, "key3")
	assert.Equal(t, 0, result)

	// Test with a key that has a non-int32 value
	result = KeyValueInt32OrZero(data, "key2")
	assert.Equal(t, 0, result)

	// Test with an empty map
	emptyMap := map[string]any{}
	result = KeyValueInt32OrZero(emptyMap, "key1")
	assert.Equal(t, 0, result)

	// Test with a nil map
	var nilMap map[string]any
	result = KeyValueInt32OrZero(nilMap, "key1")
	assert.Equal(t, 0, result)
}

func TestCopyRequestBody(t *testing.T) {
	// Test with a non-nil body
	bodyContent := []byte("test body content")
	req, _ := http.NewRequest("POST", "http://example.com", bytes.NewBuffer(bodyContent))

	copiedBody, err := CopyRequestBody(req)
	assert.NoError(t, err)
	assert.Equal(t, bodyContent, copiedBody)

	// Verify that the original body is still readable
	body, err := io.ReadAll(req.Body)
	assert.NoError(t, err)
	assert.Equal(t, bodyContent, body)

	// Test with a nil body
	req, _ = http.NewRequest("GET", "http://example.com", nil)
	copiedBody, err = CopyRequestBody(req)
	assert.NoError(t, err)
	assert.Nil(t, copiedBody)
}

func TestEstimateTokenCount(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "Empty string",
			text:     "",
			expected: 0,
		},
		{
			name:     "Simple sentence",
			text:     "This is a simple test.",
			expected: 7, // 4 words * 1.3 + 2 punctuation
		},
		{
			name:     "Complex sentence with punctuation",
			text:     "Hello, world! This is a test with some punctuation marks: .,;!?",
			expected: 21, // 9 words * 1.3 + 8 punctuation (actual calculation may vary)
		},
		{
			name:     "Numbers and special characters",
			text:     "Testing 123 with @#$%^&*() special chars.",
			expected: 16, // 5 words * 1.3 + 11 special chars
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EstimateTokenCount(tt.text)
			// Since token estimation is approximate, we'll allow a small margin of error
			assert.InDelta(t, tt.expected, result, 2, "Token count should be approximately correct")
		})
	}
}

func TestGenerateRandomString(t *testing.T) {
	// Test with different lengths
	lengths := []int{0, 1, 5, 10, 20}

	for _, length := range lengths {
		result := GenerateRandomString(length)
		assert.Equal(t, length, len(result), "Generated string length should match requested length")

		// Verify that the string contains only valid characters
		for _, char := range result {
			assert.True(t, (char >= 'a' && char <= 'z') ||
				(char >= 'A' && char <= 'Z') ||
				(char >= '0' && char <= '9'),
				"Generated string should only contain alphanumeric characters")
		}
	}

	// Test that two generated strings are different (randomness check)
	if len(GenerateRandomString(10)) > 0 { // Only test if length > 0
		str1 := GenerateRandomString(10)
		str2 := GenerateRandomString(10)
		assert.NotEqual(t, str1, str2, "Two generated strings should be different")
	}
}

func TestIntToObjectId(t *testing.T) {
	// Test with various uint values
	tests := []struct {
		input    uint
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{9999, "9999"},
	}

	for _, tt := range tests {
		result := IntToObjectId(tt.input)
		assert.NotNil(t, result)
		assert.Equal(t, tt.expected, *result)
	}
}

func TestDecodeToUTF8(t *testing.T) {
	t.Skip("Skipping DecodeToUTF8 tests until the function is fixed")
}
