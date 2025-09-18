// internal/cli/config.go
package cli

import (
	"fmt"
)

var (
	// Global client instance
	client *Client
	
	// Global configuration
	outputFormat string
	verboseMode  bool
)

// Initialize sets up the global CLI client and configuration
func Initialize(apiURL, apiToken, format string, verbose bool) error {
	if apiURL == "" {
		return fmt.Errorf("API URL is required")
	}

	if apiToken == "" {
		return fmt.Errorf("authentication token is required")
	}

	// Create HTTP client
	client = NewClient(apiURL, apiToken)
	
	// Set global configuration
	outputFormat = format
	verboseMode = verbose

	if verboseMode {
		fmt.Printf("Initialized CLI client for %s\n", apiURL)
	}

	return nil
}

// GetClient returns the global client instance
func GetClient() *Client {
	return client
}

// GetFormat returns the configured output format
func GetFormat() string {
	return outputFormat
}

// IsVerbose returns whether verbose mode is enabled
func IsVerbose() bool {
	return verboseMode
}