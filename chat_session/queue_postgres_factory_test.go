package chat_session

import (
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/config"
)

func TestPostgreSQLQueueFactory_Integration(t *testing.T) {
	// Test creating PostgreSQL queue factory from configuration
	cfg := config.QueueConfig{
		Type:       "postgres",
		BufferSize: 50,
		PostgreSQL: config.PostgreSQLQueueConfig{
			ReconnectInterval:   "3s",
			MaxReconnectRetries: 15,
			NotifyTimeout:       "10s",
		},
	}

	factory, err := CreateQueueFactory(cfg)
	if err != nil {
		t.Fatalf("Failed to create PostgreSQL queue factory: %v", err)
	}

	// Verify it's the right type
	if _, ok := factory.(*DeferredPostgreSQLQueueFactory); !ok {
		t.Errorf("Expected DeferredPostgreSQLQueueFactory, got %T", factory)
	}

	// Test queue creation without DATABASE_URL (should fail gracefully)
	queue, err := factory.CreateQueue("test-session", nil)
	if err == nil {
		t.Error("Expected error when DATABASE_URL is not set, but got nil")
		if queue != nil {
			queue.Close() // Clean up if somehow created
		}
	}

	// Test with DATABASE_URL but invalid connection (should fail gracefully)
	oldURL := os.Getenv("DATABASE_URL")
	defer func() {
		if oldURL == "" {
			os.Unsetenv("DATABASE_URL")
		} else {
			os.Setenv("DATABASE_URL", oldURL)
		}
	}()

	os.Setenv("DATABASE_URL", "postgres://invalid:invalid@localhost:1234/invalid?sslmode=disable")
	queue, err = factory.CreateQueue("test-session", nil)
	if err == nil {
		t.Error("Expected error with invalid DATABASE_URL, but got nil")
		if queue != nil {
			queue.Close() // Clean up if somehow created
		}
	}
}

func TestCreateQueueFactory_PostgreSQL(t *testing.T) {
	// Test that PostgreSQL is recognized as a valid queue type
	cfg := config.QueueConfig{
		Type:       "postgres",
		BufferSize: 100,
	}

	factory, err := CreateQueueFactory(cfg)
	if err != nil {
		t.Fatalf("PostgreSQL should be supported queue type, got error: %v", err)
	}

	if factory == nil {
		t.Fatal("Factory should not be nil for postgres type")
	}
}

func TestCreateDefaultQueueFactory_PostgreSQL(t *testing.T) {
	// Save original value
	originalType := os.Getenv("QUEUE_TYPE")
	defer func() {
		if originalType == "" {
			os.Unsetenv("QUEUE_TYPE")
		} else {
			os.Setenv("QUEUE_TYPE", originalType)
		}
	}()

	// Set to postgres
	os.Setenv("QUEUE_TYPE", "postgres")

	// This should create a PostgreSQL factory
	factory := CreateDefaultQueueFactory()
	if factory == nil {
		t.Fatal("Factory should not be nil")
	}

	// Should be deferred PostgreSQL factory
	if _, ok := factory.(*DeferredPostgreSQLQueueFactory); !ok {
		t.Errorf("Expected DeferredPostgreSQLQueueFactory when QUEUE_TYPE=postgres, got %T", factory)
	}
}