// test_config_parsing.go - Test environment variable parsing for plugins
package main

import (
	"fmt"
	"os"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/caarlos0/env/v9"
)

func main() {
	// Set test environment variable
	os.Setenv("PLUGINS_CONFIG_PATH", "./examples/plugins-file-collectors.yaml")

	// Parse config
	cfg := &config.Config{}
	if err := env.Parse(cfg); err != nil {
		fmt.Printf("Failed to parse config: %v\n", err)
		return
	}

	fmt.Printf("Plugins.ConfigPath: '%s'\n", cfg.Plugins.ConfigPath)
	fmt.Printf("Plugins.ConfigServiceURL: '%s'\n", cfg.Plugins.ConfigServiceURL)
	
	// Test validation
	if err := cfg.Validate(); err != nil {
		fmt.Printf("Config validation failed: %v\n", err)
		return
	}
	
	fmt.Printf("Config validation passed\n")
}