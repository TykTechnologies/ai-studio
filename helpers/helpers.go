package helpers

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"unicode"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

type ErrorResponse struct {
	StatusCode int
	Title      string
	Message    string
}

func (e ErrorResponse) Error() string {
	return e.Message
}

// Helper functions
func NewBadRequestError(message string) ErrorResponse {
	return ErrorResponse{
		StatusCode: http.StatusBadRequest,
		Title:      "Bad Request",
		Message:    message,
	}
}

func NewInternalServerError(message string) ErrorResponse {
	return ErrorResponse{
		StatusCode: http.StatusInternalServerError,
		Title:      "Internal Server Error",
		Message:    message,
	}
}

func NewNotFoundError(message string) ErrorResponse {
	return ErrorResponse{
		StatusCode: http.StatusNotFound,
		Title:      "Not Found",
		Message:    message,
	}
}

func NewForbiddenError(message string) ErrorResponse {
	return ErrorResponse{
		StatusCode: http.StatusForbidden,
		Title:      "Forbidden",
		Message:    message,
	}
}

func NewUnauthorizedError(message string) ErrorResponse {
	return ErrorResponse{
		StatusCode: http.StatusUnauthorized,
		Title:      "Unauthorized",
		Message:    message,
	}
}

func KeyValueOrZero(dat map[string]any, key string) int {
	if val, ok := dat[key]; ok {
		val, ok := val.(int)
		if ok {
			return val
		}
	}
	return 0
}

func KeyValueInt32OrZero(dat map[string]any, key string) int {
	if val, ok := dat[key]; ok {
		val, ok := val.(int32)
		if ok {
			return int(val)
		}
	}
	return 0
}

func CopyRequestBody(r *http.Request) ([]byte, error) {
	// Check if the body is nil
	if r.Body == nil {
		return nil, nil
	}

	// Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	// Restore the io.ReadCloser to its original state
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	// Return the copied body
	return body, nil
}

func EstimateTokenCount(text string) int {
	// Constants for estimation
	const (
		averageWordLength = 4.7
		tokensPerWord     = 1.3
	)

	// Split the text into words
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	// Count the number of words
	wordCount := len(words)

	// Estimate the number of tokens
	estimatedTokens := int(float64(wordCount) * tokensPerWord)

	// Add an estimate for punctuation and special characters
	nonAlphanumericCount := 0
	for _, char := range text {
		if !unicode.IsLetter(char) && !unicode.IsNumber(char) && !unicode.IsSpace(char) {
			nonAlphanumericCount++
		}
	}

	estimatedTokens += nonAlphanumericCount

	return estimatedTokens
}

func DecodeToUTF8(s string) (string, error) {
	// Step 1: Decode base64
	decodedBytes, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("base64 decoding failed: %v", err)
	}

	// Step 2 & 3: Convert to UTF-8
	// This example assumes the original encoding was Windows-1252 (a common encoding)
	// Replace this with the correct encoding if known
	reader := transform.NewReader(strings.NewReader(string(decodedBytes)), charmap.Windows1252.NewDecoder())
	utf8Bytes, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("conversion to UTF-8 failed: %v", err)
	}

	return string(utf8Bytes), nil
}

func GenerateRandomString(length int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)

	for i := range b {
		b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
	}

	return string(b)
}

func IntToObjectId(id uint) *string {
	strID := fmt.Sprintf("%d", id)
	return &strID
}
