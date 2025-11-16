package helpers

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/gin-gonic/gin"
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

func NewPaymentRequiredError(message string) ErrorResponse {
	return ErrorResponse{
		StatusCode: http.StatusPaymentRequired,
		Title:      "Payment Required",
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

// SendErrorResponse sends a standardized error response to the client
// It handles different error types, particularly ErrorResponse
func SendErrorResponse(c *gin.Context, err error) {
	switch e := err.(type) {
	case ErrorResponse:
		c.JSON(e.StatusCode, gin.H{
			"errors": []gin.H{{
				"title":  e.Title,
				"detail": e.Message,
			}},
		})
	default:
		// Unexpected error type
		c.JSON(http.StatusInternalServerError, gin.H{
			"errors": []gin.H{{
				"title":  "Internal Server Error",
				"detail": err.Error(),
			}},
		})
	}
}

// JSONMapAccessor provides convenient access to values in a JSONMap
type JSONMapAccessor struct {
	data map[string]interface{}
}

// NewJSONMapAccessor creates a new JSONMapAccessor
func NewJSONMapAccessor(data map[string]interface{}) *JSONMapAccessor {
	return &JSONMapAccessor{data: data}
}

// GetString retrieves a string value from the JSONMap
func (a *JSONMapAccessor) GetString(key, defaultValue string) string {
	if v, ok := a.data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}

		return ""
	}

	return defaultValue
}

// GetSlice retrieves a []interface{} value from the JSONMap
func (a *JSONMapAccessor) GetSlice(key string) []interface{} {
	if v, ok := a.data[key]; ok {
		if slice, ok := v.([]interface{}); ok {
			return slice
		}
	}

	return nil
}

func ValidateEmailDomain(email string) error {
	appConfig := config.Get()

	if len(appConfig.FilterSignupDomains) == 0 {
		return nil
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return fmt.Errorf("invalid email address")
	}

	domain := strings.ToLower(parts[1])
	for _, allowed := range appConfig.FilterSignupDomains {
		if strings.ToLower(allowed) == domain {
			return nil
		}
	}

	return fmt.Errorf("email domain '%s' is not permitted", domain)
}

func HashString(s string) string {
	hasher := sha256.New()
	hasher.Write([]byte(s))
	return hex.EncodeToString(hasher.Sum(nil))
}

// CalculatePKCEChallengeS256 calculates the S256 PKCE code challenge.
// It takes the code verifier, computes its SHA256 hash, and then Base64 URL encodes the hash.
func CalculatePKCEChallengeS256(codeVerifier string) string {
	// Calculate SHA256 hash
	sha256Hash := sha256.Sum256([]byte(codeVerifier))

	// Base64 URL encode the hash
	// The standard base64.URLEncoding uses padding, which should be removed for PKCE.
	challenge := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(sha256Hash[:])
	return challenge
}

func DaysLeft(targetDate time.Time) int {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	normalizedTarget := targetDate.UTC().Truncate(24 * time.Hour)

	return int(normalizedTarget.Sub(now).Hours() / 24)
}
