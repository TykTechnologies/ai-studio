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
		detailed, _ := cmd.Flags().GetBool("detailed")

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

		// Show detailed output if requested
		if detailed {
			return cli.PrintOutput(resp.Data)
		}

		// Show compact table by default
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

// llmFiltersCmd shows filters associated with an LLM
var llmFiltersCmd = &cobra.Command{
	Use:   "filters <llm-id>",
	Short: "Show filters associated with an LLM",
	Long:  "Display all filters currently associated with a specific LLM.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		llmID := args[0]
		
		resp, err := cli.GetClient().Get("/api/v1/llms/" + llmID + "/filters")
		if err != nil {
			return fmt.Errorf("failed to get LLM filters: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// llmAddFilterCmd adds a filter to an LLM
var llmAddFilterCmd = &cobra.Command{
	Use:   "add-filter <llm-id> <filter-id>",
	Short: "Add a filter to an LLM",
	Long:  "Associate a filter with an LLM. The filter will be applied to all requests to this LLM.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		llmID := args[0]
		filterID := args[1]
		
		// Get current filters
		resp, err := cli.GetClient().Get("/api/v1/llms/" + llmID + "/filters")
		if err != nil {
			return fmt.Errorf("failed to get current filters: %w", err)
		}
		
		// Extract current filter IDs
		var currentFilters []uint
		if data, ok := resp.Data.([]interface{}); ok {
			for _, item := range data {
				if filterMap, ok := item.(map[string]interface{}); ok {
					if id, ok := filterMap["id"].(float64); ok {
						currentFilters = append(currentFilters, uint(id))
					}
				}
			}
		}
		
		// Add new filter ID
		newFilterID, err := strconv.ParseUint(filterID, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid filter ID: %s", filterID)
		}
		
		// Check if already exists
		for _, existingID := range currentFilters {
			if existingID == uint(newFilterID) {
				return fmt.Errorf("filter %s is already associated with LLM %s", filterID, llmID)
			}
		}
		
		currentFilters = append(currentFilters, uint(newFilterID))
		
		// Update filter associations
		reqData := map[string]interface{}{
			"filter_ids": currentFilters,
		}
		
		updateResp, err := cli.GetClient().Put("/api/v1/llms/"+llmID+"/filters", reqData)
		if err != nil {
			return fmt.Errorf("failed to add filter: %w", err)
		}
		
		cli.PrintSuccess(updateResp.Message)
		return nil
	},
}

// llmRemoveFilterCmd removes a filter from an LLM
var llmRemoveFilterCmd = &cobra.Command{
	Use:   "remove-filter <llm-id> <filter-id>",
	Short: "Remove a filter from an LLM",
	Long:  "Remove filter association from an LLM.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		llmID := args[0]
		filterID := args[1]
		
		// Get current filters
		resp, err := cli.GetClient().Get("/api/v1/llms/" + llmID + "/filters")
		if err != nil {
			return fmt.Errorf("failed to get current filters: %w", err)
		}
		
		// Extract current filter IDs and remove the specified one
		var newFilters []uint
		removeFilterID, err := strconv.ParseUint(filterID, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid filter ID: %s", filterID)
		}
		
		found := false
		if data, ok := resp.Data.([]interface{}); ok {
			for _, item := range data {
				if filterMap, ok := item.(map[string]interface{}); ok {
					if id, ok := filterMap["id"].(float64); ok {
						if uint(id) != uint(removeFilterID) {
							newFilters = append(newFilters, uint(id))
						} else {
							found = true
						}
					}
				}
			}
		}
		
		if !found {
			return fmt.Errorf("filter %s is not associated with LLM %s", filterID, llmID)
		}
		
		// Update filter associations
		reqData := map[string]interface{}{
			"filter_ids": newFilters,
		}
		
		updateResp, err := cli.GetClient().Put("/api/v1/llms/"+llmID+"/filters", reqData)
		if err != nil {
			return fmt.Errorf("failed to remove filter: %w", err)
		}
		
		cli.PrintSuccess(updateResp.Message)
		return nil
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
	llmCmd.AddCommand(llmFiltersCmd)
	llmCmd.AddCommand(llmAddFilterCmd)
	llmCmd.AddCommand(llmRemoveFilterCmd)

	// llm list flags
	llmListCmd.Flags().String("vendor", "", "filter by vendor (openai, anthropic, google, vertex, ollama)")
	llmListCmd.Flags().Bool("active", true, "filter by active status")
	llmListCmd.Flags().Int("page", 1, "page number")
	llmListCmd.Flags().Int("limit", 20, "items per page")
	llmListCmd.Flags().Bool("detailed", false, "show all columns (default: compact view)")

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