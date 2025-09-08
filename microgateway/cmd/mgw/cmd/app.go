// cmd/mgw/cmd/app.go
package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/microgateway/internal/cli"
	"github.com/spf13/cobra"
)

// appCmd represents the app command
var appCmd = &cobra.Command{
	Use:   "app",
	Short: "Manage application configurations",
	Long: `Manage application configurations in the microgateway.

Applications represent client applications that use the microgateway to access LLM providers.
Each app has its own credentials, budget limits, and LLM access permissions.`,
}

// appListCmd lists all apps
var appListCmd = &cobra.Command{
	Use:   "list",
	Short: "List application configurations",
	Long:  "List all application configurations with optional filtering by active status.",
	RunE: func(cmd *cobra.Command, args []string) error {
		active, _ := cmd.Flags().GetBool("active")
		page, _ := cmd.Flags().GetInt("page")
		limit, _ := cmd.Flags().GetInt("limit")

		params := map[string]string{
			"active": strconv.FormatBool(active),
			"page":   strconv.Itoa(page),
			"limit":  strconv.Itoa(limit),
		}

		resp, err := cli.GetClient().GetWithQuery("/api/v1/apps", params)
		if err != nil {
			return fmt.Errorf("failed to list apps: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// appCreateCmd creates a new app
var appCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new application",
	Long:  "Create a new application configuration with the specified parameters.",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")
		email, _ := cmd.Flags().GetString("email")
		budget, _ := cmd.Flags().GetFloat64("budget")
		resetDay, _ := cmd.Flags().GetInt("reset-day")
		rateLimit, _ := cmd.Flags().GetInt("rate-limit")
		allowedIPs, _ := cmd.Flags().GetString("allowed-ips")
		llmIDs, _ := cmd.Flags().GetString("llm-ids")

		if name == "" {
			return fmt.Errorf("name is required")
		}
		if email == "" {
			return fmt.Errorf("email is required")
		}

		req := cli.CreateAppRequest{
			Name:           name,
			Description:    description,
			OwnerEmail:     email,
			MonthlyBudget:  budget,
			BudgetResetDay: resetDay,
			RateLimitRPM:   rateLimit,
		}

		// Parse allowed IPs
		if allowedIPs != "" {
			req.AllowedIPs = strings.Split(allowedIPs, ",")
		}

		// Parse LLM IDs
		if llmIDs != "" {
			idStrings := strings.Split(llmIDs, ",")
			req.LLMIDs = make([]uint, len(idStrings))
			for i, idStr := range idStrings {
				id, err := strconv.ParseUint(strings.TrimSpace(idStr), 10, 32)
				if err != nil {
					return fmt.Errorf("invalid LLM ID '%s': %w", idStr, err)
				}
				req.LLMIDs[i] = uint(id)
			}
		}

		resp, err := cli.GetClient().Post("/api/v1/apps", req)
		if err != nil {
			return fmt.Errorf("failed to create app: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return cli.PrintOutput(resp.Data)
	},
}

// appGetCmd gets a specific app
var appGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get application configuration by ID",
	Long:  "Retrieve detailed information about a specific application configuration.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		
		resp, err := cli.GetClient().Get("/api/v1/apps/" + id)
		if err != nil {
			return fmt.Errorf("failed to get app: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// appUpdateCmd updates an app
var appUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update application configuration",
	Long:  "Update an existing application configuration with new parameters.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		
		req := cli.UpdateAppRequest{}
		
		if name, _ := cmd.Flags().GetString("name"); name != "" {
			req.Name = &name
		}
		if description, _ := cmd.Flags().GetString("description"); description != "" {
			req.Description = &description
		}
		if email, _ := cmd.Flags().GetString("email"); email != "" {
			req.OwnerEmail = &email
		}
		if cmd.Flags().Changed("active") {
			val, _ := cmd.Flags().GetBool("active")
			req.IsActive = &val
		}
		if cmd.Flags().Changed("budget") {
			val, _ := cmd.Flags().GetFloat64("budget")
			req.MonthlyBudget = &val
		}
		if cmd.Flags().Changed("reset-day") {
			val, _ := cmd.Flags().GetInt("reset-day")
			req.BudgetResetDay = &val
		}
		if cmd.Flags().Changed("rate-limit") {
			val, _ := cmd.Flags().GetInt("rate-limit")
			req.RateLimitRPM = &val
		}
		if allowedIPs, _ := cmd.Flags().GetString("allowed-ips"); allowedIPs != "" {
			req.AllowedIPs = strings.Split(allowedIPs, ",")
		}

		resp, err := cli.GetClient().Put("/api/v1/apps/"+id, req)
		if err != nil {
			return fmt.Errorf("failed to update app: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return cli.PrintOutput(resp.Data)
	},
}

// appDeleteCmd deletes an app
var appDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete application configuration",
	Long:  "Delete (soft delete) an application configuration. This will disable the app but preserve historical data.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		
		resp, err := cli.GetClient().Delete("/api/v1/apps/" + id)
		if err != nil {
			return fmt.Errorf("failed to delete app: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return nil
	},
}

// appLLMsCmd manages LLM associations for an app
var appLLMsCmd = &cobra.Command{
	Use:   "llms <id>",
	Short: "Manage LLM associations for an app",
	Long:  "Get or set LLM associations for an application. Use --set to update associations.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		setLLMs, _ := cmd.Flags().GetString("set")

		if setLLMs != "" {
			// Update LLM associations
			idStrings := strings.Split(setLLMs, ",")
			llmIDs := make([]uint, len(idStrings))
			for i, idStr := range idStrings {
				llmID, err := strconv.ParseUint(strings.TrimSpace(idStr), 10, 32)
				if err != nil {
					return fmt.Errorf("invalid LLM ID '%s': %w", idStr, err)
				}
				llmIDs[i] = uint(llmID)
			}

			req := cli.UpdateAppLLMsRequest{LLMIDs: llmIDs}
			resp, err := cli.GetClient().Put("/api/v1/apps/"+id+"/llms", req)
			if err != nil {
				return fmt.Errorf("failed to update app LLMs: %w", err)
			}

			cli.PrintSuccess(resp.Message)
			return nil
		} else {
			// Get current LLM associations
			resp, err := cli.GetClient().Get("/api/v1/apps/" + id + "/llms")
			if err != nil {
				return fmt.Errorf("failed to get app LLMs: %w", err)
			}

			return cli.PrintOutput(resp.Data)
		}
	},
}

func init() {
	// Add subcommands
	appCmd.AddCommand(appListCmd)
	appCmd.AddCommand(appCreateCmd)
	appCmd.AddCommand(appGetCmd)
	appCmd.AddCommand(appUpdateCmd)
	appCmd.AddCommand(appDeleteCmd)
	appCmd.AddCommand(appLLMsCmd)

	// app list flags
	appListCmd.Flags().Bool("active", true, "filter by active status")
	appListCmd.Flags().Int("page", 1, "page number")
	appListCmd.Flags().Int("limit", 20, "items per page")

	// app create flags
	appCreateCmd.Flags().String("name", "", "application name (required)")
	appCreateCmd.Flags().String("description", "", "application description")
	appCreateCmd.Flags().String("email", "", "owner email (required)")
	appCreateCmd.Flags().Float64("budget", 0, "monthly budget limit")
	appCreateCmd.Flags().Int("reset-day", 1, "budget reset day of month (1-28)")
	appCreateCmd.Flags().Int("rate-limit", 0, "requests per minute limit")
	appCreateCmd.Flags().String("allowed-ips", "", "comma-separated list of allowed IP addresses")
	appCreateCmd.Flags().String("llm-ids", "", "comma-separated list of LLM IDs to associate")
	appCreateCmd.MarkFlagRequired("name")
	appCreateCmd.MarkFlagRequired("email")

	// app update flags
	appUpdateCmd.Flags().String("name", "", "application name")
	appUpdateCmd.Flags().String("description", "", "application description")
	appUpdateCmd.Flags().String("email", "", "owner email")
	appUpdateCmd.Flags().Bool("active", true, "whether app is active")
	appUpdateCmd.Flags().Float64("budget", 0, "monthly budget limit")
	appUpdateCmd.Flags().Int("reset-day", 0, "budget reset day of month (1-28)")
	appUpdateCmd.Flags().Int("rate-limit", 0, "requests per minute limit")
	appUpdateCmd.Flags().String("allowed-ips", "", "comma-separated list of allowed IP addresses")

	// app llms flags
	appLLMsCmd.Flags().String("set", "", "comma-separated list of LLM IDs to associate with app")
}