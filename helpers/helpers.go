package helpers

import (
	"bytes"
	"io"
	"net/http"
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
