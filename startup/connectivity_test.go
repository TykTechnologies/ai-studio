package startup

import (
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/config"
)

func TestDatabaseConnectivity(t *testing.T) {
	tests := []struct {
		name         string
		databaseType string
		databaseURL  string
		expectError  bool
	}{
		{
			name:         "valid sqlite database",
			databaseType: "sqlite",
			databaseURL:  ":memory:",
			expectError:  false,
		},
		{
			name:         "invalid database type",
			databaseType: "invalid",
			databaseURL:  "test.db",
			expectError:  true,
		},
		{
			name:         "invalid sqlite file path",
			databaseType: "sqlite",
			databaseURL:  "/invalid/path/that/does/not/exist/test.db",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.AppConf{
				DatabaseType: tt.databaseType,
				DatabaseURL:  tt.databaseURL,
			}

			err := testDatabaseConnectivity(cfg)
			
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestQueueConnectivity(t *testing.T) {
	tests := []struct {
		name        string
		queueType   string
		expectError bool
	}{
		{
			name:        "inmemory queue",
			queueType:   "inmemory",
			expectError: false,
		},
		{
			name:        "postgres queue (no DATABASE_URL)",
			queueType:   "postgres",
			expectError: true, // Should fail without DATABASE_URL
		},
		{
			name:        "invalid queue type",
			queueType:   "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queueConfig := config.QueueConfig{
				Type:       tt.queueType,
				BufferSize: 100,
			}

			err := testQueueConnectivity(queueConfig)
			
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNATSURLParsing(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		expectedHost string
		expectedPort string
	}{
		{
			name:         "standard nats URL",
			url:          "nats://localhost:4222",
			expectedHost: "localhost",
			expectedPort: "4222",
		},
		{
			name:         "nats URL with auth",
			url:          "nats://user:pass@localhost:4222",
			expectedHost: "localhost",
			expectedPort: "4222",
		},
		{
			name:         "host:port only",
			url:          "localhost:4222",
			expectedHost: "localhost",
			expectedPort: "4222",
		},
		{
			name:         "host only",
			url:          "localhost",
			expectedHost: "localhost",
			expectedPort: "4222",
		},
		{
			name:         "empty URL",
			url:          "",
			expectedHost: "localhost",
			expectedPort: "4222",
		},
		{
			name:         "different port",
			url:          "nats://myhost:9999",
			expectedHost: "myhost",
			expectedPort: "9999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, err := parseNATSURL(tt.url)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if host != tt.expectedHost {
				t.Errorf("expected host %s, got %s", tt.expectedHost, host)
			}

			if port != tt.expectedPort {
				t.Errorf("expected port %s, got %s", tt.expectedPort, port)
			}
		})
	}
}

func TestConnectivityIntegration(t *testing.T) {
	// Skip integration test if not in integration test environment
	if os.Getenv("INTEGRATION_TEST") == "" {
		t.Skip("Skipping integration test - set INTEGRATION_TEST env var to run")
	}

	cfg := &config.AppConf{
		DatabaseType: "sqlite",
		DatabaseURL:  ":memory:",
		QueueConfig: config.QueueConfig{
			Type:       "inmemory",
			BufferSize: 100,
		},
	}

	err := TestConnectivity(cfg)
	if err != nil {
		t.Errorf("connectivity test failed: %v", err)
	}
}