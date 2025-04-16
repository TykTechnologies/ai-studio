package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/TykTechnologies/midsommar/v2/proxy"
)

// Config holds the configuration for the mockproxy
type Config struct {
	ProxyPort   int          `json:"proxyPort"`
	LLMs        []MockLLM    `json:"llms"`
	Datasources []Datasource `json:"datasources"`
	Users       []User       `json:"users"`
}

type MockLLM struct {
	Name        string   `json:"name"`
	Vendor      string   `json:"vendor"`
	APIEndpoint string   `json:"apiEndpoint"`
	APIKey      string   `json:"apiKey"`
	Active      bool     `json:"active"`
	Models      []string `json:"models"`
}

type Datasource struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Active bool   `json:"active"`
}

type User struct {
	ID     uint   `json:"id"`
	Email  string `json:"email"`
	APIKey string `json:"apiKey"`
}

func main() {
	// Parse command line arguments
	configPath := flag.String("conf", "", "Path to the configuration file")
	flag.Parse()

	if *configPath == "" {
		log.Fatal("Configuration file path is required. Use --conf flag.")
	}

	// Load configuration from file
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create custom proxy dependencies
	deps := NewMockDependencies(cfg)

	// Create proxy config
	proxyConfig := &proxy.Config{
		Port: cfg.ProxyPort,
	}

	// Create and start the proxy
	p := proxy.NewEmbeddedProxy(deps, proxyConfig)

	// Setup signal handling for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Start the proxy in a goroutine
	go func() {
		fmt.Printf("Starting mockproxy on port %d\n", cfg.ProxyPort)
		if err := p.Start(); err != nil {
			log.Fatalf("Failed to start proxy: %v", err)
		}
	}()

	// Wait for signal to shutdown
	<-stop

	fmt.Println("Shutting down...")
}

// loadConfig loads the configuration from the specified file path
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}
