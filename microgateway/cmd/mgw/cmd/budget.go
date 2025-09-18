// cmd/mgw/cmd/budget.go
package cmd

import (
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/cli"
	"github.com/spf13/cobra"
)

// budgetCmd represents the budget command
var budgetCmd = &cobra.Command{
	Use:   "budget",
	Short: "Manage budget configurations and usage",
	Long: `Manage budget configurations and monitor usage across applications.

Budget management allows you to set spending limits, monitor usage,
and track costs across different LLM providers and applications.`,
	Aliases: []string{"budgets"},
}

// budgetListCmd lists budget information
var budgetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List budget information for all applications",
	Long:  "List budget information and usage summary for all applications (admin only).",
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := cli.GetClient().Get("/api/v1/budgets")
		if err != nil {
			return fmt.Errorf("failed to list budgets: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// budgetUsageCmd gets budget usage for an app
var budgetUsageCmd = &cobra.Command{
	Use:   "usage <app-id>",
	Short: "Get budget usage for an application",
	Long:  "Retrieve current budget usage and status for a specific application.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := args[0]
		llmID, _ := cmd.Flags().GetString("llm-id")

		params := make(map[string]string)
		if llmID != "" {
			params["llm_id"] = llmID
		}

		endpoint := "/api/v1/budgets/" + appID + "/usage"
		resp, err := cli.GetClient().GetWithQuery(endpoint, params)
		if err != nil {
			return fmt.Errorf("failed to get budget usage: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// budgetUpdateCmd updates budget settings
var budgetUpdateCmd = &cobra.Command{
	Use:   "update <app-id>",
	Short: "Update budget settings for an application",
	Long:  "Update monthly budget limits and reset day for an application.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := args[0]
		budget, _ := cmd.Flags().GetFloat64("budget")
		resetDay, _ := cmd.Flags().GetInt("reset-day")

		if budget <= 0 {
			return fmt.Errorf("budget must be greater than 0")
		}

		req := cli.UpdateBudgetRequest{
			MonthlyBudget:  budget,
			BudgetResetDay: resetDay,
		}

		resp, err := cli.GetClient().Put("/api/v1/budgets/"+appID, req)
		if err != nil {
			return fmt.Errorf("failed to update budget: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return nil
	},
}

// budgetHistoryCmd gets budget usage history
var budgetHistoryCmd = &cobra.Command{
	Use:   "history <app-id>",
	Short: "Get budget usage history for an application",
	Long:  "Retrieve historical budget usage data for a specific application over a time period.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := args[0]
		startStr, _ := cmd.Flags().GetString("start")
		endStr, _ := cmd.Flags().GetString("end")
		llmID, _ := cmd.Flags().GetString("llm-id")

		params := make(map[string]string)
		
		// Default to last 30 days if no start time specified
		if startStr == "" {
			startStr = time.Now().AddDate(0, 0, -30).Format(time.RFC3339)
		}
		if endStr == "" {
			endStr = time.Now().Format(time.RFC3339)
		}
		
		// Validate time format
		if _, err := time.Parse(time.RFC3339, startStr); err != nil {
			return fmt.Errorf("invalid start time format (use RFC3339: 2006-01-02T15:04:05Z): %w", err)
		}
		if _, err := time.Parse(time.RFC3339, endStr); err != nil {
			return fmt.Errorf("invalid end time format (use RFC3339: 2006-01-02T15:04:05Z): %w", err)
		}

		params["start_time"] = startStr
		params["end_time"] = endStr
		if llmID != "" {
			params["llm_id"] = llmID
		}

		endpoint := "/api/v1/budgets/" + appID + "/history"
		resp, err := cli.GetClient().GetWithQuery(endpoint, params)
		if err != nil {
			return fmt.Errorf("failed to get budget history: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

func init() {
	// Add subcommands
	budgetCmd.AddCommand(budgetListCmd)
	budgetCmd.AddCommand(budgetUsageCmd)
	budgetCmd.AddCommand(budgetUpdateCmd)
	budgetCmd.AddCommand(budgetHistoryCmd)

	// budget usage flags
	budgetUsageCmd.Flags().String("llm-id", "", "filter by specific LLM ID")

	// budget update flags
	budgetUpdateCmd.Flags().Float64("budget", 0, "monthly budget limit (required)")
	budgetUpdateCmd.Flags().Int("reset-day", 1, "budget reset day of month (1-28)")
	budgetUpdateCmd.MarkFlagRequired("budget")

	// budget history flags
	budgetHistoryCmd.Flags().String("start", "", "start time in RFC3339 format (default: 30 days ago)")
	budgetHistoryCmd.Flags().String("end", "", "end time in RFC3339 format (default: now)")
	budgetHistoryCmd.Flags().String("llm-id", "", "filter by specific LLM ID")
}