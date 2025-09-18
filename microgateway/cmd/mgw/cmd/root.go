// cmd/mgw/cmd/root.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TykTechnologies/midsommar/microgateway/internal/cli"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	cfgFile string
	url     string
	token   string
	format  string
	verbose bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mgw",
	Short: "Microgateway CLI - AI/LLM Gateway Management Tool",
	Long: `mgw is a command-line interface for managing the Microgateway AI/LLM platform.

It provides easy access to all microgateway management operations including:
- LLM configuration and management
- Application and credential management  
- Budget tracking and enforcement
- Analytics and usage reporting
- Token management and authentication

Examples:
  mgw llm list                                    # List all LLMs
  mgw app create --name="My App" --email=me@example.com  # Create new app
  mgw token create --app-id=1 --name="API Key"   # Generate API token
  mgw analytics summary 1                        # View usage analytics`,
	
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Initialize CLI client with global flags
		if err := cli.Initialize(url, token, format, verbose); err != nil {
			fmt.Printf("Error initializing CLI: %v\n", err)
			os.Exit(1)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.mgw.yaml)")
	rootCmd.PersistentFlags().StringVar(&url, "url", "", "microgateway API URL (default: http://localhost:8080)")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "authentication token")
	rootCmd.PersistentFlags().StringVar(&format, "format", "table", "output format: table, json, yaml")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Add subcommands
	rootCmd.AddCommand(llmCmd)
	rootCmd.AddCommand(appCmd)
	rootCmd.AddCommand(tokenCmd)
	rootCmd.AddCommand(budgetCmd)
	rootCmd.AddCommand(analyticsCmd)
	rootCmd.AddCommand(pricingCmd)
	rootCmd.AddCommand(pluginCmd)
	rootCmd.AddCommand(systemCmd)
	
	// Hub-and-spoke commands
	rootCmd.AddCommand(namespaceCmd)
	rootCmd.AddCommand(edgeCmd)
}

// CLIConfig represents the CLI configuration structure
type CLIConfig struct {
	URL    string `yaml:"url" json:"url"`
	Token  string `yaml:"token" json:"token"`
	Format string `yaml:"format" json:"format"`
}

// loadConfigFile loads configuration from a file
func loadConfigFile(configPath string) error {
	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist")
	}

	// Read file content
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config CLIConfig
	
	// Determine file format by extension
	ext := strings.ToLower(filepath.Ext(configPath))
	
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse YAML config: %w", err)
		}
	case ".json":
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse JSON config: %w", err)
		}
	default:
		// Try YAML as default
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse config (unknown format, tried YAML): %w", err)
		}
	}

	// Apply configuration values if not already set via flags or environment
	if url == "" && config.URL != "" {
		url = config.URL
	}
	if token == "" && config.Token != "" {
		token = config.Token
	}
	if format == "table" && config.Format != "" { // Only override default, not explicit flag
		format = config.Format
	}

	return nil
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	// Set defaults from environment
	if url == "" {
		if envURL := os.Getenv("MGW_URL"); envURL != "" {
			url = envURL
		} else {
			url = "http://localhost:8080"
		}
	}

	if token == "" {
		if envToken := os.Getenv("MGW_TOKEN"); envToken != "" {
			token = envToken
		}
	}

	if format == "" {
		if envFormat := os.Getenv("MGW_FORMAT"); envFormat != "" {
			format = envFormat
		} else {
			format = "table"
		}
	}

	// Load from config file if specified
	if cfgFile != "" {
		if err := loadConfigFile(cfgFile); err != nil {
			fmt.Printf("Error loading config file '%s': %v\n", cfgFile, err)
			os.Exit(1)
		}
	}
}