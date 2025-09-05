// internal/config/config_test.go
package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_WithDefaults(t *testing.T) {
	t.Run("DefaultConfig", func(t *testing.T) {
		// Clear any existing env vars
		os.Clearenv()
		
		// Set minimum required env vars for validation
		os.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
		defer os.Unsetenv("ENCRYPTION_KEY")

		cfg, err := Load("")
		assert.NoError(t, err)
		require.NotNil(t, cfg)

		// Test default values
		assert.Equal(t, 8080, cfg.Server.Port)
		assert.Equal(t, "0.0.0.0", cfg.Server.Host)
		assert.False(t, cfg.Server.TLSEnabled)
		assert.Equal(t, "sqlite", cfg.Database.Type)
		assert.True(t, cfg.Cache.Enabled)
		assert.Equal(t, "info", cfg.Observability.LogLevel)
		assert.Equal(t, "json", cfg.Observability.LogFormat)
	})
}

func TestLoad_WithEnvironmentVariables(t *testing.T) {
	t.Run("CustomConfig", func(t *testing.T) {
		// Set environment variables
		envVars := map[string]string{
			"PORT":                    "9090",
			"HOST":                    "127.0.0.1",
			"DATABASE_TYPE":           "postgres",
			"DATABASE_DSN":            "postgres://user:pass@localhost:5432/test",
			"LOG_LEVEL":               "debug",
			"CACHE_MAX_SIZE":          "2000",
			"ANALYTICS_BUFFER_SIZE":   "500",
			"JWT_SECRET":              "test-secret-key",
			"ENCRYPTION_KEY":          "12345678901234567890123456789012",
		}

		// Set environment variables
		for key, value := range envVars {
			os.Setenv(key, value)
		}
		defer func() {
			for key := range envVars {
				os.Unsetenv(key)
			}
		}()

		cfg, err := Load("")
		assert.NoError(t, err)
		require.NotNil(t, cfg)

		// Verify custom values
		assert.Equal(t, 9090, cfg.Server.Port)
		assert.Equal(t, "127.0.0.1", cfg.Server.Host)
		assert.Equal(t, "postgres", cfg.Database.Type)
		assert.Equal(t, "postgres://user:pass@localhost:5432/test", cfg.Database.DSN)
		assert.Equal(t, "debug", cfg.Observability.LogLevel)
		assert.Equal(t, 2000, cfg.Cache.MaxSize)
		assert.Equal(t, 500, cfg.Analytics.BufferSize)
		assert.Equal(t, "test-secret-key", cfg.Security.JWTSecret)
		assert.Equal(t, "12345678901234567890123456789012", cfg.Security.EncryptionKey)
	})
}

func TestLoad_WithEnvFile(t *testing.T) {
	t.Run("ValidEnvFile", func(t *testing.T) {
		// Create temporary .env file
		envContent := `PORT=7777
HOST=192.168.1.1
DATABASE_TYPE=sqlite
LOG_LEVEL=warn
JWT_SECRET=file-secret-key
ENCRYPTION_KEY=12345678901234567890123456789012
`
		tempFile, err := os.CreateTemp("", "test-*.env")
		require.NoError(t, err)
		defer os.Remove(tempFile.Name())

		_, err = tempFile.WriteString(envContent)
		require.NoError(t, err)
		tempFile.Close()

		cfg, err := Load(tempFile.Name())
		assert.NoError(t, err)
		require.NotNil(t, cfg)

		// Verify values from file
		assert.Equal(t, 7777, cfg.Server.Port)
		assert.Equal(t, "192.168.1.1", cfg.Server.Host)
		assert.Equal(t, "sqlite", cfg.Database.Type)
		assert.Equal(t, "warn", cfg.Observability.LogLevel)
		assert.Equal(t, "file-secret-key", cfg.Security.JWTSecret)
		assert.Equal(t, "12345678901234567890123456789012", cfg.Security.EncryptionKey)
	})

	t.Run("NonexistentEnvFile", func(t *testing.T) {
		// Set required encryption key
		os.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
		defer os.Unsetenv("ENCRYPTION_KEY")
		
		cfg, err := Load("nonexistent.env")
		assert.NoError(t, err) // Should not fail, just warn
		assert.NotNil(t, cfg)
	})
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name          string
		modifyConfig  func(*Config)
		expectError   bool
		errorContains string
	}{
		{
			name: "ValidConfig",
			modifyConfig: func(c *Config) {
				c.Security.EncryptionKey = "12345678901234567890123456789012"
				c.Security.JWTSecret = "valid-jwt-secret"
			},
			expectError: false,
		},
		{
			name: "InvalidDatabaseType",
			modifyConfig: func(c *Config) {
				c.Database.Type = "mongodb"
			},
			expectError:   true,
			errorContains: "unsupported database type",
		},
		{
			name: "InvalidPort",
			modifyConfig: func(c *Config) {
				c.Server.Port = -1
			},
			expectError:   true,
			errorContains: "invalid server port",
		},
		{
			name: "InvalidPortTooHigh",
			modifyConfig: func(c *Config) {
				c.Server.Port = 70000
			},
			expectError:   true,
			errorContains: "invalid server port",
		},
		{
			name: "TLSEnabledWithoutCerts",
			modifyConfig: func(c *Config) {
				c.Server.TLSEnabled = true
				c.Server.TLSCertPath = ""
			},
			expectError:   true,
			errorContains: "TLS enabled but cert/key paths not provided",
		},
		{
			name: "InvalidCacheSize",
			modifyConfig: func(c *Config) {
				c.Cache.MaxSize = -1
			},
			expectError:   true,
			errorContains: "cache max size must be positive",
		},
		{
			name: "InvalidAnalyticsBufferSize",
			modifyConfig: func(c *Config) {
				c.Analytics.BufferSize = 0
			},
			expectError:   true,
			errorContains: "analytics buffer size must be positive",
		},
		{
			name: "InvalidRetentionDays",
			modifyConfig: func(c *Config) {
				c.Analytics.RetentionDays = 0
			},
			expectError:   true,
			errorContains: "analytics retention days must be at least 1",
		},
		{
			name: "InvalidEncryptionKeyLength",
			modifyConfig: func(c *Config) {
				c.Security.EncryptionKey = "too-short"
			},
			expectError:   true,
			errorContains: "encryption key must be exactly 32 characters",
		},
		{
			name: "InvalidBCryptCost",
			modifyConfig: func(c *Config) {
				c.Security.BCryptCost = 2
			},
			expectError:   true,
			errorContains: "bcrypt cost must be between 4 and 31",
		},
		{
			name: "InvalidLogLevel",
			modifyConfig: func(c *Config) {
				c.Observability.LogLevel = "invalid"
			},
			expectError:   true,
			errorContains: "invalid log level",
		},
		{
			name: "InvalidLogFormat",
			modifyConfig: func(c *Config) {
				c.Observability.LogFormat = "xml"
			},
			expectError:   true,
			errorContains: "invalid log format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start with valid config
			cfg := &Config{
				Server: ServerConfig{
					Port: 8080,
					Host: "0.0.0.0",
				},
				Database: DatabaseConfig{
					Type: "sqlite",
				},
				Cache: CacheConfig{
					MaxSize: 1000,
				},
				Analytics: AnalyticsConfig{
					BufferSize:    100,
					RetentionDays: 30,
				},
				Security: SecurityConfig{
					EncryptionKey: "12345678901234567890123456789012",
					BCryptCost:    10,
				},
				Observability: ObservabilityConfig{
					LogLevel:  "info",
					LogFormat: "json",
				},
			}

			// Apply modifications
			if tt.modifyConfig != nil {
				tt.modifyConfig(cfg)
			}

			// Test validation
			err := cfg.Validate()
			
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_HelperMethods(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			TLSEnabled: true,
		},
		Observability: ObservabilityConfig{
			LogLevel: "debug",
		},
	}

	t.Run("IsDevelopment", func(t *testing.T) {
		assert.True(t, cfg.IsDevelopment())

		cfg.Observability.LogLevel = "info"
		assert.False(t, cfg.IsDevelopment())

		cfg.Observability.LogLevel = "trace"
		assert.True(t, cfg.IsDevelopment())
	})

	t.Run("IsProduction", func(t *testing.T) {
		cfg.Observability.LogLevel = "info"
		cfg.Server.TLSEnabled = true
		assert.True(t, cfg.IsProduction())

		cfg.Server.TLSEnabled = false
		assert.False(t, cfg.IsProduction())

		cfg.Server.TLSEnabled = true
		cfg.Observability.LogLevel = "debug"
		assert.False(t, cfg.IsProduction())
	})

	t.Run("GetDatabaseConfig", func(t *testing.T) {
		cfg.Database = DatabaseConfig{
			Type:            "postgres",
			DSN:             "test-dsn",
			MaxOpenConns:    50,
			MaxIdleConns:    50,
			ConnMaxLifetime: 10 * time.Minute,
			AutoMigrate:     true,
			LogLevel:        "debug",
		}

		dbConfig := cfg.GetDatabaseConfig()
		assert.Equal(t, cfg.Database.Type, dbConfig.Type)
		assert.Equal(t, cfg.Database.DSN, dbConfig.DSN)
		assert.Equal(t, cfg.Database.MaxOpenConns, dbConfig.MaxOpenConns)
		assert.Equal(t, cfg.Database.MaxIdleConns, dbConfig.MaxIdleConns)
		assert.Equal(t, cfg.Database.ConnMaxLifetime, dbConfig.ConnMaxLifetime)
		assert.Equal(t, cfg.Database.AutoMigrate, dbConfig.AutoMigrate)
		assert.Equal(t, cfg.Database.LogLevel, dbConfig.LogLevel)
	})
}

func TestConfig_TimeoutParsing(t *testing.T) {
	t.Run("ValidTimeouts", func(t *testing.T) {
		envVars := map[string]string{
			"READ_TIMEOUT":     "45s",
			"WRITE_TIMEOUT":    "60s",
			"IDLE_TIMEOUT":     "180s",
			"SHUTDOWN_TIMEOUT": "45s",
			"CACHE_TTL":        "2h",
			"ENCRYPTION_KEY":   "12345678901234567890123456789012",
		}

		for key, value := range envVars {
			os.Setenv(key, value)
		}
		defer func() {
			for key := range envVars {
				os.Unsetenv(key)
			}
		}()

		cfg, err := Load("")
		assert.NoError(t, err)

		assert.Equal(t, 45*time.Second, cfg.Server.ReadTimeout)
		assert.Equal(t, 60*time.Second, cfg.Server.WriteTimeout)
		assert.Equal(t, 180*time.Second, cfg.Server.IdleTimeout)
		assert.Equal(t, 45*time.Second, cfg.Server.ShutdownTimeout)
		assert.Equal(t, 2*time.Hour, cfg.Cache.TTL)
	})
}

func TestConfig_BooleanParsing(t *testing.T) {
	t.Run("BooleanValues", func(t *testing.T) {
		envVars := map[string]string{
			"TLS_ENABLED":          "true",
			"TLS_CERT_PATH":        "/test/cert.pem",
			"TLS_KEY_PATH":         "/test/key.pem",
			"CACHE_ENABLED":        "false",
			"ANALYTICS_ENABLED":    "true",
			"ENABLE_METRICS":       "false",
			"DB_AUTO_MIGRATE":      "true",
			"ANALYTICS_REALTIME":   "false",
			"ENCRYPTION_KEY":       "12345678901234567890123456789012",
		}

		for key, value := range envVars {
			os.Setenv(key, value)
		}
		defer func() {
			for key := range envVars {
				os.Unsetenv(key)
			}
		}()

		cfg, err := Load("")
		assert.NoError(t, err)

		assert.True(t, cfg.Server.TLSEnabled)
		assert.False(t, cfg.Cache.Enabled)
		assert.True(t, cfg.Analytics.Enabled)
		assert.False(t, cfg.Observability.EnableMetrics)
		assert.True(t, cfg.Database.AutoMigrate)
		assert.False(t, cfg.Analytics.EnableRealtime)
	})
}

func TestConfig_IntegerParsing(t *testing.T) {
	t.Run("IntegerValues", func(t *testing.T) {
		envVars := map[string]string{
			"PORT":                    "9999",
			"DB_MAX_OPEN_CONNS":       "50",
			"DB_MAX_IDLE_CONNS":       "25",
			"CACHE_MAX_SIZE":          "5000",
			"ANALYTICS_BUFFER_SIZE":   "2000",
			"ANALYTICS_RETENTION_DAYS": "180",
			"BCRYPT_COST":             "12",
			"TOKEN_LENGTH":            "64",
			"ENCRYPTION_KEY":          "12345678901234567890123456789012",
		}

		for key, value := range envVars {
			os.Setenv(key, value)
		}
		defer func() {
			for key := range envVars {
				os.Unsetenv(key)
			}
		}()

		cfg, err := Load("")
		assert.NoError(t, err)

		assert.Equal(t, 9999, cfg.Server.Port)
		assert.Equal(t, 50, cfg.Database.MaxOpenConns)
		assert.Equal(t, 25, cfg.Database.MaxIdleConns)
		assert.Equal(t, 5000, cfg.Cache.MaxSize)
		assert.Equal(t, 2000, cfg.Analytics.BufferSize)
		assert.Equal(t, 180, cfg.Analytics.RetentionDays)
		assert.Equal(t, 12, cfg.Security.BCryptCost)
		assert.Equal(t, 64, cfg.Security.TokenLength)
	})
}

func TestConfig_Validation_EdgeCases(t *testing.T) {
	t.Run("EmptyEncryptionKey", func(t *testing.T) {
		cfg := &Config{
			Server:   ServerConfig{Port: 8080},
			Database: DatabaseConfig{Type: "sqlite"},
			Cache:    CacheConfig{MaxSize: 1000},
			Analytics: AnalyticsConfig{
				BufferSize:    100,
				RetentionDays: 30,
			},
			Security: SecurityConfig{
				EncryptionKey: "",
				BCryptCost:    10,
			},
			Observability: ObservabilityConfig{
				LogLevel:  "info",
				LogFormat: "json",
			},
		}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "encryption key must be exactly 32 characters")
	})

	t.Run("ValidTLSConfig", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Port:        8080,
				TLSEnabled:  true,
				TLSCertPath: "/path/to/cert.pem",
				TLSKeyPath:  "/path/to/key.pem",
			},
			Database: DatabaseConfig{Type: "sqlite"},
			Cache:    CacheConfig{MaxSize: 1000},
			Analytics: AnalyticsConfig{
				BufferSize:    100,
				RetentionDays: 30,
			},
			Security: SecurityConfig{
				EncryptionKey: "12345678901234567890123456789012",
				BCryptCost:    10,
			},
			Observability: ObservabilityConfig{
				LogLevel:  "info",
				LogFormat: "json",
			},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("MinimumValidConfig", func(t *testing.T) {
		cfg := &Config{
			Server:   ServerConfig{Port: 1},
			Database: DatabaseConfig{Type: "sqlite"},
			Cache:    CacheConfig{MaxSize: 1},
			Analytics: AnalyticsConfig{
				BufferSize:    1,
				RetentionDays: 1,
			},
			Security: SecurityConfig{
				EncryptionKey: "12345678901234567890123456789012",
				BCryptCost:    4, // Minimum
			},
			Observability: ObservabilityConfig{
				LogLevel:  "trace",
				LogFormat: "text",
			},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("MaximumValidConfig", func(t *testing.T) {
		cfg := &Config{
			Server:   ServerConfig{Port: 65535},
			Database: DatabaseConfig{Type: "postgres"},
			Cache:    CacheConfig{MaxSize: 1000000},
			Analytics: AnalyticsConfig{
				BufferSize:    1000000,
				RetentionDays: 3650, // 10 years
			},
			Security: SecurityConfig{
				EncryptionKey: "12345678901234567890123456789012",
				BCryptCost:    31, // Maximum
			},
			Observability: ObservabilityConfig{
				LogLevel:  "panic",
				LogFormat: "json",
			},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})
}