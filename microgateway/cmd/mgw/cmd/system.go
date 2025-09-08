// cmd/mgw/cmd/system.go
package cmd

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/microgateway/internal/cli"
	"github.com/spf13/cobra"
)

// systemCmd represents the system command
var systemCmd = &cobra.Command{
	Use:   "system",
	Short: "System health and information commands",
	Long: `System commands for checking microgateway health, readiness, and configuration.

These commands help monitor the microgateway service status and retrieve
system-level information for monitoring and debugging purposes.`,
	Aliases: []string{"sys"},
}

// systemHealthCmd checks system health
var systemHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check microgateway health status",
	Long:  "Check the basic health status of the microgateway service.",
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := cli.GetClient().Get("/health")
		if err != nil {
			cli.PrintError(fmt.Errorf("health check failed: %w", err))
			return nil // Don't return error to avoid double error display
		}

		cli.PrintSuccess("Microgateway is healthy")
		return cli.PrintOutput(resp.Data)
	},
}

// systemReadyCmd checks system readiness
var systemReadyCmd = &cobra.Command{
	Use:   "ready",
	Short: "Check microgateway readiness status",
	Long:  "Check if the microgateway service is ready to accept requests (includes dependency checks).",
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := cli.GetClient().Get("/ready")
		if err != nil {
			cli.PrintError(fmt.Errorf("readiness check failed: %w", err))
			return nil // Don't return error to avoid double error display
		}

		cli.PrintSuccess("Microgateway is ready")
		return cli.PrintOutput(resp.Data)
	},
}

// systemMetricsCmd gets system metrics
var systemMetricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Get system metrics",
	Long:  "Retrieve Prometheus-format metrics from the microgateway service.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// For metrics, we want raw text output not JSON parsing
		client := cli.GetClient()
		u := client.BaseURL + "/metrics"
		
		resp, err := client.HTTPClient.Get(u)
		if err != nil {
			return fmt.Errorf("failed to get metrics: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
		}

		// Read and print raw response
		body := make([]byte, 4096)
		n, err := resp.Body.Read(body)
		if err != nil && err.Error() != "EOF" {
			return fmt.Errorf("failed to read metrics: %w", err)
		}

		fmt.Print(string(body[:n]))
		return nil
	},
}

// systemConfigCmd shows CLI configuration
var systemConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show CLI configuration",
	Long:  "Display current CLI configuration including API URL, authentication, and output format.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := cli.GetClient()
		
		config := map[string]interface{}{
			"api_url": client.BaseURL,
			"format":  cli.GetFormat(),
			"verbose": cli.IsVerbose(),
			"token_set": client.Token != "",
		}

		return cli.PrintOutput(config)
	},
}

// systemVersionCmd gets microgateway version
var systemVersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Get microgateway version information",
	Long:  "Retrieve version information from the running microgateway service.",
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := cli.GetClient().Get("/")
		if err != nil {
			return fmt.Errorf("failed to get version info: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

func init() {
	// Add subcommands
	systemCmd.AddCommand(systemHealthCmd)
	systemCmd.AddCommand(systemReadyCmd)
	systemCmd.AddCommand(systemMetricsCmd)
	systemCmd.AddCommand(systemConfigCmd)
	systemCmd.AddCommand(systemVersionCmd)
}