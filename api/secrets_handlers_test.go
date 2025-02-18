package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecretsHandlersWithoutKey(t *testing.T) {
	// Save current env and restore after test
	originalKey := os.Getenv("TYK_AI_SECRET_KEY")
	defer os.Setenv("TYK_AI_SECRET_KEY", originalKey)

	// Unset the key for testing
	os.Unsetenv("TYK_AI_SECRET_KEY")

	api, _ := setupTestAPI(t)

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "create secret without key",
			method:         "POST",
			path:           "/api/v1/secrets",
			expectedStatus: http.StatusServiceUnavailable,
			expectedError:  "Secrets functionality is disabled. TYK_AI_SECRET_KEY environment variable is not set.",
		},
		{
			name:           "get secret without key",
			method:         "GET",
			path:           "/api/v1/secrets/1",
			expectedStatus: http.StatusServiceUnavailable,
			expectedError:  "Secrets functionality is disabled. TYK_AI_SECRET_KEY environment variable is not set.",
		},
		{
			name:           "update secret without key",
			method:         "PATCH",
			path:           "/api/v1/secrets/1",
			expectedStatus: http.StatusServiceUnavailable,
			expectedError:  "Secrets functionality is disabled. TYK_AI_SECRET_KEY environment variable is not set.",
		},
		{
			name:           "delete secret without key",
			method:         "DELETE",
			path:           "/api/v1/secrets/1",
			expectedStatus: http.StatusServiceUnavailable,
			expectedError:  "Secrets functionality is disabled. TYK_AI_SECRET_KEY environment variable is not set.",
		},
		{
			name:           "list secrets without key",
			method:         "GET",
			path:           "/api/v1/secrets",
			expectedStatus: http.StatusServiceUnavailable,
			expectedError:  "Secrets functionality is disabled. TYK_AI_SECRET_KEY environment variable is not set.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.path, nil)
			api.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedError, response.Errors[0].Detail)
		})
	}
}
