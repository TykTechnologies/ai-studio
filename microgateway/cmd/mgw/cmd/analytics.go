// cmd/mgw/cmd/analytics.go
package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/cli"
	"github.com/spf13/cobra"
)

// analyticsCmd represents the analytics command
var analyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "View analytics and usage reports",
	Long: `View analytics data and usage reports for applications.

Analytics provide insights into API usage patterns, costs, performance,
and error rates across different LLM providers and applications.`,
	Aliases: []string{"stats", "reports"},
}

// analyticsEventsCmd gets analytics events
var analyticsEventsCmd = &cobra.Command{
	Use:   "events <app-id>",
	Short: "Get analytics events for an application",
	Long:  "Retrieve detailed analytics events for a specific application with pagination support.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := args[0]
		page, _ := cmd.Flags().GetInt("page")
		limit, _ := cmd.Flags().GetInt("limit")

		params := map[string]string{
			"app_id": appID,
			"page":   strconv.Itoa(page),
			"limit":  strconv.Itoa(limit),
		}

		resp, err := cli.GetClient().GetWithQuery("/api/v1/analytics/events", params)
		if err != nil {
			return fmt.Errorf("failed to get analytics events: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// analyticsSummaryCmd gets analytics summary
var analyticsSummaryCmd = &cobra.Command{
	Use:   "summary <app-id>",
	Short: "Get analytics summary for an application",
	Long:  "Retrieve aggregated analytics summary for a specific application over a time period.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := args[0]
		startStr, _ := cmd.Flags().GetString("start")
		endStr, _ := cmd.Flags().GetString("end")

		params := map[string]string{"app_id": appID}

		// Default to last 7 days if no time range specified
		if startStr == "" {
			startStr = time.Now().AddDate(0, 0, -7).Format(time.RFC3339)
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

		resp, err := cli.GetClient().GetWithQuery("/api/v1/analytics/summary", params)
		if err != nil {
			return fmt.Errorf("failed to get analytics summary: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// analyticsCostsCmd gets cost analysis
var analyticsCostsCmd = &cobra.Command{
	Use:   "costs <app-id>",
	Short: "Get cost analysis for an application",
	Long:  "Retrieve detailed cost analysis and breakdown for a specific application over a time period.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := args[0]
		startStr, _ := cmd.Flags().GetString("start")
		endStr, _ := cmd.Flags().GetString("end")

		params := map[string]string{"app_id": appID}

		// Default to last 30 days if no time range specified
		if startStr == "" {
			startStr = time.Now().AddDate(0, -1, 0).Format(time.RFC3339)
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

		resp, err := cli.GetClient().GetWithQuery("/api/v1/analytics/costs", params)
		if err != nil {
			return fmt.Errorf("failed to get cost analysis: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

func init() {
	// Add subcommands
	analyticsCmd.AddCommand(analyticsEventsCmd)
	analyticsCmd.AddCommand(analyticsSummaryCmd)
	analyticsCmd.AddCommand(analyticsCostsCmd)

	// analytics events flags
	analyticsEventsCmd.Flags().Int("page", 1, "page number")
	analyticsEventsCmd.Flags().Int("limit", 50, "items per page")

	// analytics summary flags
	analyticsSummaryCmd.Flags().String("start", "", "start time in RFC3339 format (default: 7 days ago)")
	analyticsSummaryCmd.Flags().String("end", "", "end time in RFC3339 format (default: now)")

	// analytics costs flags
	analyticsCostsCmd.Flags().String("start", "", "start time in RFC3339 format (default: 30 days ago)")
	analyticsCostsCmd.Flags().String("end", "", "end time in RFC3339 format (default: now)")
}