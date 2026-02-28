package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
)

func TestSecretsHandlersWithoutKey(t *testing.T) {
	// Clear the global store to simulate no encryption key
	prevStore := secrets.Store()
	secrets.SetStore(nil)
	defer secrets.SetStore(prevStore)

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
			expectedError:  "Secrets functionality is disabled. Encryption key is not configured.",
		},
		{
			name:           "get secret without key",
			method:         "GET",
			path:           "/api/v1/secrets/1",
			expectedStatus: http.StatusServiceUnavailable,
			expectedError:  "Secrets functionality is disabled. Encryption key is not configured.",
		},
		{
			name:           "update secret without key",
			method:         "PATCH",
			path:           "/api/v1/secrets/1",
			expectedStatus: http.StatusServiceUnavailable,
			expectedError:  "Secrets functionality is disabled. Encryption key is not configured.",
		},
		{
			name:           "delete secret without key",
			method:         "DELETE",
			path:           "/api/v1/secrets/1",
			expectedStatus: http.StatusServiceUnavailable,
			expectedError:  "Secrets functionality is disabled. Encryption key is not configured.",
		},
		{
			name:           "list secrets without key",
			method:         "GET",
			path:           "/api/v1/secrets",
			expectedStatus: http.StatusServiceUnavailable,
			expectedError:  "Secrets functionality is disabled. Encryption key is not configured.",
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
