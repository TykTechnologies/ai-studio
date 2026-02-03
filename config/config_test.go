package config

import (
	"os"
	"testing"
)

func TestConfigWithoutEnvFile(t *testing.T) {
	// Save original env vars
	origPort := os.Getenv("SERVER_PORT")
	origDBURL := os.Getenv("DATABASE_URL")

	// Clean up after test
	defer func() {
		os.Setenv("SERVER_PORT", origPort)
		os.Setenv("DATABASE_URL", origDBURL)
		globalConfig = nil // Reset global config for other tests
	}()

	// Set test environment variables
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("DATABASE_URL", "test.db")

	// Get config
	conf := Get("")

	// Verify environment variables are read correctly
	if conf.ServerPort != "9090" {
		t.Errorf("Expected ServerPort to be '9090', got '%s'", conf.ServerPort)
	}
	if conf.DatabaseURL != "test.db" {
		t.Errorf("Expected DatabaseURL to be 'test.db', got '%s'", conf.DatabaseURL)
	}

	// Verify defaults are set for unspecified values
	if conf.DatabaseType != "sqlite" {
		t.Errorf("Expected default DatabaseType to be 'sqlite', got '%s'", conf.DatabaseType)
	}
	if conf.DefaultSignupMode != "both" {
		t.Errorf("Expected default DefaultSignupMode to be 'both', got '%s'", conf.DefaultSignupMode)
	}
}

func TestConfigWithEnvFile(t *testing.T) {
	// Save original env vars
	origPort := os.Getenv("SERVER_PORT")
	origDBURL := os.Getenv("DATABASE_URL")
	origDBType := os.Getenv("DATABASE_TYPE")

	// Clean up after test
	defer func() {
		os.Remove(".env")
		os.Setenv("SERVER_PORT", origPort)
		os.Setenv("DATABASE_URL", origDBURL)
		os.Setenv("DATABASE_TYPE", origDBType)
		globalConfig = nil // Reset global config for other tests
	}()

	// Clear env vars before test so .env file values can be loaded
	os.Unsetenv("SERVER_PORT")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("DATABASE_TYPE")
	globalConfig = nil // Reset global config to force reload

	// Create temporary .env file
	envContent := "SERVER_PORT=8888\nDATABASE_URL=env.db\nDATABASE_TYPE=postgres"
	err := os.WriteFile(".env", []byte(envContent), 0644)
	if err != nil {
		t.Fatal("Failed to create test .env file:", err)
	}

	// Get config - this will load the .env file internally
	conf := Get("")

	// Verify .env file values are read correctly
	if conf.ServerPort != "8888" {
		t.Errorf("Expected ServerPort to be '8888', got '%s'", conf.ServerPort)
	}
	if conf.DatabaseURL != "env.db" {
		t.Errorf("Expected DatabaseURL to be 'env.db', got '%s'", conf.DatabaseURL)
	}
	if conf.DatabaseType != "postgres" {
		t.Errorf("Expected DatabaseType to be 'postgres', got '%s'", conf.DatabaseType)
	}
}

func TestConfigEnvOverridesFile(t *testing.T) {
	// Save original env vars
	origPort := os.Getenv("SERVER_PORT")
	origDBURL := os.Getenv("DATABASE_URL")

	// Clean up after test
	defer func() {
		os.Remove(".env")
		os.Setenv("SERVER_PORT", origPort)
		os.Setenv("DATABASE_URL", origDBURL)
		globalConfig = nil // Reset global config for other tests
	}()

	// Clear DATABASE_URL so it can be loaded from .env file
	os.Unsetenv("DATABASE_URL")
	globalConfig = nil // Reset global config to force reload

	// Create temporary .env file first
	envContent := "SERVER_PORT=8888\nDATABASE_URL=env.db"
	err := os.WriteFile(".env", []byte(envContent), 0644)
	if err != nil {
		t.Fatal("Failed to create test .env file:", err)
	}

	// Set environment variable to override .env file value
	os.Setenv("SERVER_PORT", "7777")

	// Reset global config to force reload
	globalConfig = nil

	// Get config
	conf := Get("")

	// Verify environment variable takes precedence over .env file
	if conf.ServerPort != "7777" {
		t.Errorf("Expected ServerPort to be '7777' (from env var), got '%s'", conf.ServerPort)
	}
	// Verify other .env values are still read
	if conf.DatabaseURL != "env.db" {
		t.Errorf("Expected DatabaseURL to be 'env.db', got '%s'", conf.DatabaseURL)
	}
}
