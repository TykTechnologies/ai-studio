// cmd/mgw/cmd/llm.go
package cmd

import (
	"fmt"
	"strconv"

	"github.com/TykTechnologies/midsommar/microgateway/internal/cli"
	"github.com/spf13/cobra"
)

// llmCmd represents the llm command
var llmCmd = &cobra.Command{
	Use:   "llm",
	Short: "Manage LLM configurations",
	Long: `Manage LLM (Large Language Model) configurations in the microgateway.

This command allows you to create, list, update, and delete LLM configurations,
as well as view usage statistics for each LLM provider.`,
}

// llmListCmd lists all LLMs
var llmListCmd = &cobra.Command{
	Use:   "list",
	Short: "List LLM configurations",
	Long:  "List all LLM configurations with optional filtering by vendor and active status.",
	RunE: func(cmd *cobra.Command, args []string) error {
		vendor, _ := cmd.Flags().GetString("vendor")
		active, _ := cmd.Flags().GetBool("active")
		page, _ := cmd.Flags().GetInt("page")
		limit, _ := cmd.Flags().GetInt("limit")

		params := make(map[string]string)
		if vendor != "" {
			params["vendor"] = vendor
		}
		if active {
			params["active"] = "true"
		} else {
			params["active"] = "false"
		}
		params["page"] = strconv.Itoa(page)
		params["limit"] = strconv.Itoa(limit)

		resp, err := cli.GetClient().GetWithQuery("/api/v1/llms", params)
		if err != nil {
			return fmt.Errorf("failed to list LLMs: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// llmCreateCmd creates a new LLM
var llmCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new LLM configuration",
	Long:  "Create a new LLM configuration with the specified parameters.",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		vendor, _ := cmd.Flags().GetString("vendor")
		endpoint, _ := cmd.Flags().GetString("endpoint")
		apiKey, _ := cmd.Flags().GetString("api-key")
		defaultModel, _ := cmd.Flags().GetString("model")
		maxTokens, _ := cmd.Flags().GetInt("max-tokens")
		timeout, _ := cmd.Flags().GetInt("timeout")
		retries, _ := cmd.Flags().GetInt("retries")
		active, _ := cmd.Flags().GetBool("active")
		budget, _ := cmd.Flags().GetFloat64("budget")
		rateLimit, _ := cmd.Flags().GetInt("rate-limit")

		if name == "" {
			return fmt.Errorf("name is required")
		}
		if vendor == "" {
			return fmt.Errorf("vendor is required")
		}
		if defaultModel == "" {
			return fmt.Errorf("model is required")
		}

		req := cli.CreateLLMRequest{
			Name:           name,
			Vendor:         vendor,
			Endpoint:       endpoint,
			APIKey:         apiKey,
			DefaultModel:   defaultModel,
			MaxTokens:      maxTokens,
			TimeoutSeconds: timeout,
			RetryCount:     retries,
			IsActive:       active,
			MonthlyBudget:  budget,
			RateLimitRPM:   rateLimit,
		}

		resp, err := cli.GetClient().Post("/api/v1/llms", req)
		if err != nil {
			return fmt.Errorf("failed to create LLM: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return cli.PrintOutput(resp.Data)
	},
}

// llmGetCmd gets a specific LLM
var llmGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get LLM configuration by ID",
	Long:  "Retrieve detailed information about a specific LLM configuration.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		
		resp, err := cli.GetClient().Get("/api/v1/llms/" + id)
		if err != nil {
			return fmt.Errorf("failed to get LLM: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// llmUpdateCmd updates an LLM
var llmUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update LLM configuration",
	Long:  "Update an existing LLM configuration with new parameters.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		
		req := cli.UpdateLLMRequest{}
		
		if name, _ := cmd.Flags().GetString("name"); name != "" {
			req.Name = &name
		}
		if endpoint, _ := cmd.Flags().GetString("endpoint"); endpoint != "" {
			req.Endpoint = &endpoint
		}
		if apiKey, _ := cmd.Flags().GetString("api-key"); apiKey != "" {
			req.APIKey = &apiKey
		}
		if model, _ := cmd.Flags().GetString("model"); model != "" {
			req.DefaultModel = &model
		}
		if cmd.Flags().Changed("max-tokens") {
			val, _ := cmd.Flags().GetInt("max-tokens")
			req.MaxTokens = &val
		}
		if cmd.Flags().Changed("timeout") {
			val, _ := cmd.Flags().GetInt("timeout")
			req.TimeoutSeconds = &val
		}
		if cmd.Flags().Changed("retries") {
			val, _ := cmd.Flags().GetInt("retries")
			req.RetryCount = &val
		}
		if cmd.Flags().Changed("active") {
			val, _ := cmd.Flags().GetBool("active")
			req.IsActive = &val
		}
		if cmd.Flags().Changed("budget") {
			val, _ := cmd.Flags().GetFloat64("budget")
			req.MonthlyBudget = &val
		}
		if cmd.Flags().Changed("rate-limit") {
			val, _ := cmd.Flags().GetInt("rate-limit")
			req.RateLimitRPM = &val
		}

		resp, err := cli.GetClient().Put("/api/v1/llms/"+id, req)
		if err != nil {
			return fmt.Errorf("failed to update LLM: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return cli.PrintOutput(resp.Data)
	},
}

// llmDeleteCmd deletes an LLM
var llmDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete LLM configuration",
	Long:  "Delete (soft delete) an LLM configuration. This will disable the LLM but preserve historical data.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		
		resp, err := cli.GetClient().Delete("/api/v1/llms/" + id)
		if err != nil {
			return fmt.Errorf("failed to delete LLM: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return nil
	},
}

// llmStatsCmd gets LLM statistics
var llmStatsCmd = &cobra.Command{
	Use:   "stats <id>",
	Short: "Get LLM usage statistics",
	Long:  "Retrieve usage statistics for a specific LLM including request counts, token usage, and costs.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		
		resp, err := cli.GetClient().Get("/api/v1/llms/" + id + "/stats")
		if err != nil {
			return fmt.Errorf("failed to get LLM stats: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

func init() {
	// Add subcommands
	llmCmd.AddCommand(llmListCmd)
	llmCmd.AddCommand(llmCreateCmd)
	llmCmd.AddCommand(llmGetCmd)
	llmCmd.AddCommand(llmUpdateCmd)
	llmCmd.AddCommand(llmDeleteCmd)
	llmCmd.AddCommand(llmStatsCmd)

	// llm list flags
	llmListCmd.Flags().String("vendor", "", "filter by vendor (openai, anthropic, google, vertex, ollama)")
	llmListCmd.Flags().Bool("active", true, "filter by active status")
	llmListCmd.Flags().Int("page", 1, "page number")
	llmListCmd.Flags().Int("limit", 20, "items per page")

	// llm create flags
	llmCreateCmd.Flags().String("name", "", "LLM name (required)")
	llmCreateCmd.Flags().String("vendor", "", "LLM vendor: openai, anthropic, google, vertex, ollama (required)")
	llmCreateCmd.Flags().String("endpoint", "", "API endpoint URL (required for ollama)")
	llmCreateCmd.Flags().String("api-key", "", "API key (required for openai, anthropic)")
	llmCreateCmd.Flags().String("model", "", "default model name (required)")
	llmCreateCmd.Flags().Int("max-tokens", 4096, "maximum tokens per request")
	llmCreateCmd.Flags().Int("timeout", 30, "request timeout in seconds")
	llmCreateCmd.Flags().Int("retries", 3, "retry count for failed requests")
	llmCreateCmd.Flags().Bool("active", true, "whether LLM is active")
	llmCreateCmd.Flags().Float64("budget", 0, "monthly budget limit")
	llmCreateCmd.Flags().Int("rate-limit", 0, "requests per minute limit")
	llmCreateCmd.MarkFlagRequired("name")
	llmCreateCmd.MarkFlagRequired("vendor")
	llmCreateCmd.MarkFlagRequired("model")

	// llm update flags
	llmUpdateCmd.Flags().String("name", "", "LLM name")
	llmUpdateCmd.Flags().String("endpoint", "", "API endpoint URL")
	llmUpdateCmd.Flags().String("api-key", "", "API key")
	llmUpdateCmd.Flags().String("model", "", "default model name")
	llmUpdateCmd.Flags().Int("max-tokens", 0, "maximum tokens per request")
	llmUpdateCmd.Flags().Int("timeout", 0, "request timeout in seconds")
	llmUpdateCmd.Flags().Int("retries", 0, "retry count for failed requests")
	llmUpdateCmd.Flags().Bool("active", true, "whether LLM is active")
	llmUpdateCmd.Flags().Float64("budget", 0, "monthly budget limit")
	llmUpdateCmd.Flags().Int("rate-limit", 0, "requests per minute limit")
}