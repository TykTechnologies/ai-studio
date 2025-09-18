// cmd/mgw/cmd/pricing.go
package cmd

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/microgateway/internal/cli"
	"github.com/spf13/cobra"
)

// pricingCmd represents the pricing command
var pricingCmd = &cobra.Command{
	Use:   "pricing",
	Short: "Manage model pricing configurations",
	Long: `Manage pricing configurations for LLM models.

Model pricing is used for accurate cost calculation in analytics and budget tracking.
Different models from the same vendor can have different pricing structures.`,
	Aliases: []string{"price", "prices"},
}

// pricingListCmd lists all model prices
var pricingListCmd = &cobra.Command{
	Use:   "list",
	Short: "List model pricing configurations",
	Long:  "List all model pricing configurations, optionally filtered by vendor.",
	RunE: func(cmd *cobra.Command, args []string) error {
		vendor, _ := cmd.Flags().GetString("vendor")

		params := make(map[string]string)
		if vendor != "" {
			params["vendor"] = vendor
		}

		resp, err := cli.GetClient().GetWithQuery("/api/v1/pricing", params)
		if err != nil {
			return fmt.Errorf("failed to list model prices: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// pricingCreateCmd creates a new model price
var pricingCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new model pricing configuration",
	Long:  "Create pricing configuration for a specific model and vendor combination.",
	RunE: func(cmd *cobra.Command, args []string) error {
		vendor, _ := cmd.Flags().GetString("vendor")
		model, _ := cmd.Flags().GetString("model")
		cpit, _ := cmd.Flags().GetFloat64("input-price")
		cpt, _ := cmd.Flags().GetFloat64("output-price") 
		cacheWritePT, _ := cmd.Flags().GetFloat64("cache-write-price")
		cacheReadPT, _ := cmd.Flags().GetFloat64("cache-read-price")
		currency, _ := cmd.Flags().GetString("currency")

		if vendor == "" {
			return fmt.Errorf("vendor is required")
		}
		if model == "" {
			return fmt.Errorf("model is required")
		}
		if cpit <= 0 {
			return fmt.Errorf("input-price must be greater than 0")
		}
		if cpt <= 0 {
			return fmt.Errorf("output-price must be greater than 0")
		}

		// Convert from per-million pricing to per-token pricing (AI Gateway format)
		req := cli.CreateModelPriceRequest{
			Vendor:       vendor,
			ModelName:    model,
			CPIT:         cpit / 1000000,         // Convert MTok to per-token
			CPT:          cpt / 1000000,          // Convert MTok to per-token  
			CacheWritePT: cacheWritePT / 1000000, // Convert MTok to per-token
			CacheReadPT:  cacheReadPT / 1000000,  // Convert MTok to per-token
			Currency:     currency,
		}

		resp, err := cli.GetClient().Post("/api/v1/pricing", req)
		if err != nil {
			return fmt.Errorf("failed to create model price: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return cli.PrintOutput(resp.Data)
	},
}

// pricingGetCmd gets a specific model price
var pricingGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get model pricing configuration by ID",
	Long:  "Retrieve detailed information about a specific model pricing configuration.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		
		resp, err := cli.GetClient().Get("/api/v1/pricing/" + id)
		if err != nil {
			return fmt.Errorf("failed to get model price: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// pricingUpdateCmd updates a model price
var pricingUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update model pricing configuration",
	Long:  "Update an existing model pricing configuration with new rates.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		
		req := cli.UpdateModelPriceRequest{}
		
		if cmd.Flags().Changed("input-price") {
			val, _ := cmd.Flags().GetFloat64("input-price")
			perToken := val / 1000000  // Convert MTok to per-token
			req.CPIT = &perToken
		}
		if cmd.Flags().Changed("output-price") {
			val, _ := cmd.Flags().GetFloat64("output-price")
			perToken := val / 1000000  // Convert MTok to per-token
			req.CPT = &perToken
		}
		if cmd.Flags().Changed("cache-write-price") {
			val, _ := cmd.Flags().GetFloat64("cache-write-price")
			perToken := val / 1000000  // Convert MTok to per-token
			req.CacheWritePT = &perToken
		}
		if cmd.Flags().Changed("cache-read-price") {
			val, _ := cmd.Flags().GetFloat64("cache-read-price")
			perToken := val / 1000000  // Convert MTok to per-token
			req.CacheReadPT = &perToken
		}
		if cmd.Flags().Changed("currency") {
			val, _ := cmd.Flags().GetString("currency")
			req.Currency = &val
		}

		resp, err := cli.GetClient().Put("/api/v1/pricing/"+id, req)
		if err != nil {
			return fmt.Errorf("failed to update model price: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return cli.PrintOutput(resp.Data)
	},
}

// pricingDeleteCmd deletes a model price
var pricingDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete model pricing configuration",
	Long:  "Delete a model pricing configuration. This will affect cost calculations for future requests.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		
		resp, err := cli.GetClient().Delete("/api/v1/pricing/" + id)
		if err != nil {
			return fmt.Errorf("failed to delete model price: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return nil
	},
}

func init() {
	// Add subcommands
	pricingCmd.AddCommand(pricingListCmd)
	pricingCmd.AddCommand(pricingCreateCmd)
	pricingCmd.AddCommand(pricingGetCmd)
	pricingCmd.AddCommand(pricingUpdateCmd)
	pricingCmd.AddCommand(pricingDeleteCmd)

	// pricing list flags
	pricingListCmd.Flags().String("vendor", "", "filter by vendor (anthropic, openai, google, etc.)")

	// pricing create flags
	pricingCreateCmd.Flags().String("vendor", "", "vendor name (required)")
	pricingCreateCmd.Flags().String("model", "", "model name (required)")
	pricingCreateCmd.Flags().Float64("input-price", 0, "cost per million input tokens (e.g., 3.0 for $3.00/MTok)")
	pricingCreateCmd.Flags().Float64("output-price", 0, "cost per million output tokens (e.g., 15.0 for $15.00/MTok)")
	pricingCreateCmd.Flags().Float64("cache-write-price", 0, "cost per million cache write tokens")
	pricingCreateCmd.Flags().Float64("cache-read-price", 0, "cost per million cache read tokens")
	pricingCreateCmd.Flags().String("currency", "USD", "currency code")
	pricingCreateCmd.MarkFlagRequired("vendor")
	pricingCreateCmd.MarkFlagRequired("model")
	pricingCreateCmd.MarkFlagRequired("input-price")
	pricingCreateCmd.MarkFlagRequired("output-price")

	// pricing update flags
	pricingUpdateCmd.Flags().Float64("input-price", 0, "cost per million input tokens")
	pricingUpdateCmd.Flags().Float64("output-price", 0, "cost per million output tokens")
	pricingUpdateCmd.Flags().Float64("cache-write-price", 0, "cost per million cache write tokens")
	pricingUpdateCmd.Flags().Float64("cache-read-price", 0, "cost per million cache read tokens")
	pricingUpdateCmd.Flags().String("currency", "", "currency code")
}