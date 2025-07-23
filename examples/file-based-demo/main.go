// Package main provides a comprehensive file-based demo of the AI Gateway Library.
// This demo shows how to use JSON configuration files to set up a fully functional
// AI gateway without requiring a database.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/TykTechnologies/midsommar/v2/examples/file-based-demo/services"
	"github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
)

const (
	defaultPort  = 9090
	configDir    = "config"
	analyticsDir = "analytics"
)

func main() {
	fmt.Printf("🚀 AI Gateway Library - File-Based Demo\n")
	fmt.Printf("========================================\n\n")

	// Get the current working directory for config files
	workDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}

	configPath := filepath.Join(workDir, configDir)
	analyticsPath := filepath.Join(workDir, analyticsDir)

	fmt.Printf("📁 Configuration directory: %s\n", configPath)
	fmt.Printf("📊 Analytics directory: %s\n", analyticsPath)

	// Check if config files exist
	if err := checkConfigFiles(configPath); err != nil {
		log.Fatalf("Configuration check failed: %v", err)
	}

	// Initialize file-based services
	fmt.Printf("\n🔧 Initializing file-based services...\n")

	gatewayService, err := services.NewFileGatewayService(configPath)
	if err != nil {
		log.Fatalf("Failed to initialize gateway service: %v", err)
	}
	fmt.Printf("✅ Gateway service initialized\n")

	budgetService, err := services.NewFileBudgetService(configPath)
	if err != nil {
		log.Fatalf("Failed to initialize budget service: %v", err)
	}
	fmt.Printf("✅ Budget service initialized\n")

	analyticsHandler, err := services.NewFileAnalyticsHandler(analyticsPath)
	if err != nil {
		log.Fatalf("Failed to initialize analytics handler: %v", err)
	}
	fmt.Printf("✅ Analytics handler initialized\n")

	// Print configuration summary
	printConfigSummary(gatewayService, budgetService)

	// Create the AI Gateway
	fmt.Printf("\n🌐 Creating AI Gateway...\n")
	gateway := aigateway.NewWithAnalytics(
		gatewayService,
		budgetService,
		analyticsHandler,
		&aigateway.Config{Port: defaultPort},
	)
	fmt.Printf("✅ AI Gateway created on port %d\n", defaultPort)

	// Setup graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start the gateway server
	go func() {
		fmt.Printf("\n🚀 Starting AI Gateway server...\n")
		fmt.Printf("Server will be available at: http://localhost:%d\n", defaultPort)
		fmt.Printf("\nAvailable endpoints:\n")
		fmt.Printf("  • POST /llm/rest/{llmSlug}/chat/completions    - LLM API calls\n")
		fmt.Printf("  • POST /llm/stream/{llmSlug}/chat/completions  - Streaming LLM calls\n")
		fmt.Printf("  • GET  /.well-known/oauth-protected-resource   - OAuth2 metadata\n")

		printUsageExamples()

		if err := gateway.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Gateway server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	fmt.Printf("\n🛑 Shutdown signal received...\n")

	// Print analytics summary before shutdown
	analyticsHandler.PrintSummary()

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Printf("🔄 Gracefully stopping server...\n")
	if err := gateway.Stop(shutdownCtx); err != nil {
		log.Printf("Gateway shutdown error: %v", err)
		os.Exit(1)
	}

	fmt.Printf("✅ AI Gateway stopped gracefully\n")
}

// checkConfigFiles verifies that all required configuration files exist
func checkConfigFiles(configDir string) error {
	requiredFiles := []string{
		"llms.json",
		"credentials.json",
		"apps.json",
		"pricing.json",
		"budgets.json",
	}

	fmt.Printf("\n📋 Checking configuration files...\n")
	for _, file := range requiredFiles {
		filePath := filepath.Join(configDir, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return fmt.Errorf("required configuration file not found: %s", filePath)
		}
		fmt.Printf("✅ %s found\n", file)
	}

	return nil
}

// printConfigSummary prints a summary of the loaded configuration
func printConfigSummary(gatewayService *services.FileGatewayService, budgetService *services.FileBudgetService) {
	fmt.Printf("\n📊 Configuration Summary:\n")

	// Get active LLMs
	llms, err := gatewayService.GetActiveLLMs()
	if err != nil {
		fmt.Printf("❌ Failed to get LLMs: %v\n", err)
	} else {
		fmt.Printf("🤖 Active LLMs: %d\n", len(llms))
		for _, llm := range llms {
			apiKeyDisplay := "***"
			if llm.APIKey != "" {
				apiKeyDisplay = llm.APIKey[:min(8, len(llm.APIKey))] + "***"
			}
			fmt.Printf("   • %s (%s) - %s - API Key: %s\n",
				llm.Name, llm.Vendor, llm.DefaultModel, apiKeyDisplay)
		}
	}

	// Note about datasources and tools (not implemented in this demo)
	fmt.Printf("🗄️  Active Datasources: 0 (not implemented in file-based demo)\n")
	fmt.Printf("🔧 Active Tools: 0 (not implemented in file-based demo)\n")

	fmt.Printf("💰 Budget tracking: Enabled\n")
}

// printUsageExamples prints example curl commands for testing the gateway
func printUsageExamples() {
	fmt.Printf("\n📖 Usage Examples:\n")
	fmt.Printf("===================\n")

	// Set environment variables reminder
	fmt.Printf("\n⚠️  IMPORTANT: Set your API keys as environment variables:\n")
	fmt.Printf("   export OPENAI_API_KEY=your_openai_key_here\n")
	fmt.Printf("   export ANTHROPIC_API_KEY=your_anthropic_key_here\n")
	fmt.Printf("   export GOOGLE_AI_API_KEY=your_google_ai_key_here\n")

	fmt.Printf("\n🔑 Available Credentials (use in Authorization header):\n")
	fmt.Printf("   • demo-key-12345      (Demo Chat App - has access to GPT-4, GPT-3.5, Claude)\n")
	fmt.Printf("   • budget-key-67890    (Budget Limited App - only GPT-3.5, $5 limit)\n")
	fmt.Printf("   • premium-key-abcde   (Premium App - access to all models, $1000 limit)\n")

	fmt.Printf("\n💬 Example API calls:\n")

	// GPT-4 example
	fmt.Printf("\n1. Chat with GPT-4:\n")
	fmt.Printf("curl -X POST http://localhost:%d/llm/rest/gpt4/chat/completions \\\n", defaultPort)
	fmt.Printf("  -H \"Content-Type: application/json\" \\\n")
	fmt.Printf("  -H \"Authorization: Bearer demo-key-12345\" \\\n")
	fmt.Printf("  -d '{\n")
	fmt.Printf("    \"model\": \"gpt-4\",\n")
	fmt.Printf("    \"messages\": [\n")
	fmt.Printf("      {\"role\": \"user\", \"content\": \"Hello! Can you tell me about AI gateways?\"}\n")
	fmt.Printf("    ],\n")
	fmt.Printf("    \"max_tokens\": 150\n")
	fmt.Printf("  }'\n")

	// Claude example
	fmt.Printf("\n2. Chat with Claude:\n")
	fmt.Printf("curl -X POST http://localhost:%d/llm/rest/claude35sonnet/messages \\\n", defaultPort)
	fmt.Printf("  -H \"Content-Type: application/json\" \\\n")
	fmt.Printf("  -H \"Authorization: Bearer demo-key-12345\" \\\n")
	fmt.Printf("  -d '{\n")
	fmt.Printf("    \"model\": \"claude-3-5-sonnet-20241022\",\n")
	fmt.Printf("    \"max_tokens\": 150,\n")
	fmt.Printf("    \"messages\": [\n")
	fmt.Printf("      {\"role\": \"user\", \"content\": \"Explain the benefits of AI gateways\"}\n")
	fmt.Printf("    ]\n")
	fmt.Printf("  }'\n")

	// Budget test example
	fmt.Printf("\n3. Test budget limits (will likely be blocked):\n")
	fmt.Printf("curl -X POST http://localhost:%d/llm/rest/gpt35turbo/chat/completions \\\n", defaultPort)
	fmt.Printf("  -H \"Content-Type: application/json\" \\\n")
	fmt.Printf("  -H \"Authorization: Bearer budget-key-67890\" \\\n")
	fmt.Printf("  -d '{\n")
	fmt.Printf("    \"model\": \"gpt-3.5-turbo\",\n")
	fmt.Printf("    \"messages\": [{\"role\": \"user\", \"content\": \"Hello!\"}]\n")
	fmt.Printf("  }'\n")

	fmt.Printf("\n📈 Monitor the console for real-time analytics and budget notifications!\n")
	fmt.Printf("📁 Check the analytics/ directory for JSON logs of all activity.\n")
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
