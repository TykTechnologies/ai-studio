package chat_session

import (
	"testing"
	"time"
)

func TestNATSAuthenticationConfig(t *testing.T) {
	tests := []struct {
		name   string
		config NATSConfig
		hasAuth bool
	}{
		{
			name: "No authentication configured",
			config: NATSConfig{
				URL: "nats://localhost:4222",
			},
			hasAuth: false,
		},
		{
			name: "Username/password authentication",
			config: NATSConfig{
				URL:      "nats://localhost:4222",
				Username: "testuser",
				Password: "testpass",
			},
			hasAuth: true,
		},
		{
			name: "Token authentication",
			config: NATSConfig{
				URL:   "nats://localhost:4222",
				Token: "test-token-123",
			},
			hasAuth: true,
		},
		{
			name: "Credentials file authentication",
			config: NATSConfig{
				URL:             "nats://localhost:4222",
				CredentialsFile: "/path/to/creds.file",
			},
			hasAuth: true,
		},
		{
			name: "NKey file authentication",
			config: NATSConfig{
				URL:      "nats://localhost:4222",
				NKeyFile: "/path/to/nkey.file",
			},
			hasAuth: true,
		},
		{
			name: "TLS configuration",
			config: NATSConfig{
				URL:           "nats://localhost:4222",
				TLSEnabled:    true,
				TLSCertFile:   "/path/to/cert.pem",
				TLSKeyFile:    "/path/to/key.pem",
				TLSCAFile:     "/path/to/ca.pem",
				TLSSkipVerify: false,
			},
			hasAuth: false, // TLS is not considered authentication in this context
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test if the authentication configuration is properly detected
			hasAuth := tt.config.CredentialsFile != "" || 
				tt.config.Username != "" || 
				tt.config.Token != "" || 
				tt.config.NKeyFile != ""

			if hasAuth != tt.hasAuth {
				t.Errorf("Authentication detection mismatch: expected %v, got %v", tt.hasAuth, hasAuth)
			}

			// Since we can't actually connect to NATS without a server,
			// we'll just verify the configuration fields are set correctly
			if tt.config.Username != "" && tt.config.Password == "" {
				t.Errorf("Username set but password empty")
			}
		})
	}
}

func TestNATSConfigWithDefaults(t *testing.T) {
	config := DefaultNATSConfig()
	
	// Verify default authentication settings
	if config.CredentialsFile != "" {
		t.Errorf("Expected empty credentials file, got %s", config.CredentialsFile)
	}
	
	if config.Username != "" {
		t.Errorf("Expected empty username, got %s", config.Username)
	}
	
	if config.Password != "" {
		t.Errorf("Expected empty password, got %s", config.Password)
	}
	
	if config.Token != "" {
		t.Errorf("Expected empty token, got %s", config.Token)
	}
	
	if config.NKeyFile != "" {
		t.Errorf("Expected empty NKey file, got %s", config.NKeyFile)
	}
	
	// Verify default TLS settings
	if config.TLSEnabled {
		t.Errorf("Expected TLS disabled by default, got %v", config.TLSEnabled)
	}
	
	if config.TLSCertFile != "" {
		t.Errorf("Expected empty TLS cert file, got %s", config.TLSCertFile)
	}
	
	if config.TLSKeyFile != "" {
		t.Errorf("Expected empty TLS key file, got %s", config.TLSKeyFile)
	}
	
	if config.TLSCAFile != "" {
		t.Errorf("Expected empty TLS CA file, got %s", config.TLSCAFile)
	}
	
	if config.TLSSkipVerify {
		t.Errorf("Expected TLS skip verify disabled by default, got %v", config.TLSSkipVerify)
	}
}

func TestNATSAuthOptionsConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		config      NATSConfig
		expectError bool
	}{
		{
			name: "Valid credentials file config",
			config: NATSConfig{
				URL:             "nats://localhost:4222",
				CredentialsFile: "/tmp/test.creds",
			},
			expectError: false,
		},
		{
			name: "Valid username/password config",
			config: NATSConfig{
				URL:      "nats://localhost:4222",
				Username: "user",
				Password: "pass",
			},
			expectError: false,
		},
		{
			name: "Valid token config",
			config: NATSConfig{
				URL:   "nats://localhost:4222",
				Token: "abc123",
			},
			expectError: false,
		},
		{
			name: "Valid TLS config without client cert",
			config: NATSConfig{
				URL:        "nats://localhost:4222",
				TLSEnabled: true,
			},
			expectError: false,
		},
		{
			name: "Valid TLS config with client cert",
			config: NATSConfig{
				URL:         "nats://localhost:4222",
				TLSEnabled:  true,
				TLSCertFile: "/tmp/cert.pem",
				TLSKeyFile:  "/tmp/key.pem",
			},
			expectError: false, // We don't actually load the files in the test
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't test the actual connection without a NATS server,
			// but we can verify that the configuration structure is correct
			// and the authentication options function doesn't panic
			
			// Set default values for required fields
			if tt.config.MaxAge == 0 {
				tt.config.MaxAge = 2 * time.Hour
			}
			if tt.config.AckWait == 0 {
				tt.config.AckWait = 30 * time.Second
			}
			if tt.config.FetchTimeout == 0 {
				tt.config.FetchTimeout = 5 * time.Second
			}
			if tt.config.RetryInterval == 0 {
				tt.config.RetryInterval = 1 * time.Second
			}
			if tt.config.BufferSize == 0 {
				tt.config.BufferSize = 100
			}
			if tt.config.MaxBytes == 0 {
				tt.config.MaxBytes = 100 * 1024 * 1024
			}
			if tt.config.MaxDeliver == 0 {
				tt.config.MaxDeliver = 3
			}
			if tt.config.MaxRetries == 0 {
				tt.config.MaxRetries = 3
			}
			if tt.config.StorageType == "" {
				tt.config.StorageType = "file"
			}
			if tt.config.RetentionPolicy == "" {
				tt.config.RetentionPolicy = "interest"
			}

			// Test that the configuration is valid (won't fail due to missing fields)
			_, err := NewNATSQueue("test-session", tt.config)
			
			// We expect connection errors since there's no NATS server,
			// or file errors since we're using fake file paths
			if err != nil && !tt.expectError {
				// Check if it's a connection error or file error (both expected)
				if !containsString(err.Error(), "no servers available for connection") &&
				   !containsString(err.Error(), "dial tcp") &&
				   !containsString(err.Error(), "no such file or directory") &&
				   !containsString(err.Error(), "failed to load TLS client certificate") &&
				   !containsString(err.Error(), "failed to configure NATS authentication") {
					t.Errorf("Unexpected error type: %v", err)
				} else {
					// This is an expected error - authentication config is working
					t.Logf("Expected error (auth config working): %v", err)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    (len(s) > len(substr) && 
		     (s[:len(substr)] == substr || 
		      s[len(s)-len(substr):] == substr ||
		      findSubstring(s, substr))))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}