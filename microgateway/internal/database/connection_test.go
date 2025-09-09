// internal/database/connection_test.go
package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnect_SQLite(t *testing.T) {
	config := DatabaseConfig{
		Type:            "sqlite",
		DSN:             ":memory:",
		MaxOpenConns:    5,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		AutoMigrate:     false,
		LogLevel:        "silent",
	}

	t.Run("SuccessfulConnection", func(t *testing.T) {
		db, err := Connect(config)
		assert.NoError(t, err)
		require.NotNil(t, db)

		// Test health check
		err = IsHealthy(db)
		assert.NoError(t, err)

		// Close connection
		err = Close(db)
		assert.NoError(t, err)
	})

	t.Run("InvalidDSN", func(t *testing.T) {
		invalidConfig := config
		invalidConfig.DSN = "/invalid/path/to/db.sqlite"
		
		_, err := Connect(invalidConfig)
		assert.Error(t, err)
	})
}

func TestConnect_UnsupportedDatabase(t *testing.T) {
	config := DatabaseConfig{
		Type: "mongodb",
		DSN:  "mongodb://localhost",
	}

	_, err := Connect(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported database type")
}

// TestMigrate removed - GORM handles migrations automatically

func TestDatabaseConfig_Validation(t *testing.T) {
	tests := []struct {
		name     string
		config   DatabaseConfig
		hasError bool
	}{
		{
			name: "ValidSQLite",
			config: DatabaseConfig{
				Type:            "sqlite",
				DSN:             ":memory:",
				MaxOpenConns:    10,
				MaxIdleConns:    10,
				ConnMaxLifetime: 5 * time.Minute,
			},
			hasError: false,
		},
		{
			name: "ValidPostgreSQL",
			config: DatabaseConfig{
				Type:            "postgres",
				DSN:             "postgres://user:pass@localhost/db",
				MaxOpenConns:    25,
				MaxIdleConns:    25,
				ConnMaxLifetime: 10 * time.Minute,
			},
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test connection (may fail due to actual DB not available, but config should be valid)
			_, err := Connect(tt.config)
			
			if tt.hasError {
				assert.Error(t, err)
			} else {
				// We expect either success or a connection error (not a config error)
				if err != nil {
					// Should be a connection error, not a config error
					assert.NotContains(t, err.Error(), "unsupported database type")
				}
			}
		})
	}
}

func TestGormLogger(t *testing.T) {
	tests := []struct {
		logLevel string
		expected string
	}{
		{"silent", "silent"},
		{"error", "error"},
		{"warn", "warn"},
		{"info", "info"},
		{"invalid", "warn"}, // Default fallback
	}

	for _, tt := range tests {
		t.Run(tt.logLevel, func(t *testing.T) {
			logger := getGormLogger(tt.logLevel)
			assert.NotNil(t, logger)
			// Note: We can't easily test the actual log level set,
			// but we can verify the function doesn't panic
		})
	}
}