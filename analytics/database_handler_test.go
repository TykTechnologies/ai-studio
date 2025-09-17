package analytics

import (
	"errors"
	"testing"
)

func TestSanitizeError(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "Nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "Token in error",
			err:      errors.New("authentication failed: token=secret123"),
			expected: "authentication failed: token=***REDACTED***",
		},
		{
			name:     "API key with quotes",
			err:      errors.New("invalid api_key=\"sk-abc123def456\""),
			expected: "invalid api_key=***REDACTED***",
		},
		{
			name:     "Authorization header simple",
			err:      errors.New("request failed: authorization=token123"),
			expected: "request failed: authorization=***REDACTED***",
		},
		{
			name:     "Password with equals",
			err:      errors.New("login failed password=mypassword123"),
			expected: "login failed password=***REDACTED***",
		},
		{
			name:     "Secret with colon",
			err:      errors.New("config error secret: mysecretvalue"),
			expected: "config error secret: ***REDACTED***",
		},
		{
			name:     "Multiple patterns",
			err:      errors.New("error: token=abc123 secret=def456"),
			expected: "error: token=***REDACTED*** secret=***REDACTED***",
		},
		{
			name:     "Case insensitive",
			err:      errors.New("TOKEN=value SECRET=another"),
			expected: "TOKEN=***REDACTED*** SECRET=***REDACTED***",
		},
		{
			name:     "Credential with spaces",
			err:      errors.New("error credential = value123"),
			expected: "error credential=***REDACTED***",
		},
		{
			name:     "No sensitive data",
			err:      errors.New("connection timeout to database"),
			expected: "connection timeout to database",
		},
		{
			name:     "Bearer token",
			err:      errors.New("auth failed bearer=token123"),
			expected: "auth failed bearer=***REDACTED***",
		},
		{
			name:     "False positive prevention",
			err:      errors.New("authentication service is down"),
			expected: "authentication service is down",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizeError(tc.err)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}