package analytics

import (
	"context"
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

func TestDatabaseHandler_Cancellation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.LLMChatRecord{})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	h := &DatabaseHandler{
		ctx: ctx,
		db:  db,
		// A send to a nil channel blocks forever,
		// forcing the select to choose the context case.
		chatRecordChan: nil,
		recStarted:     true,
	}

	cancel()

	// This should not panic and should return immediately
	assert.NotPanics(t, func() {
		h.RecordChatRecord(&models.LLMChatRecord{Name: "test-record"})
	})

	var count int64
	err = db.Model(&models.LLMChatRecord{}).Count(&count).Error
	require.NoError(t, err)

	assert.Equal(t, int64(0), count, "Record should not have been created because context was cancelled")
}
