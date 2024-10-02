package helpers

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"unicode"
)

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
