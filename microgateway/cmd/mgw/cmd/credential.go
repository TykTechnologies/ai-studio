// cmd/mgw/cmd/credential.go
package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/cli"
	"github.com/spf13/cobra"
)

// credentialCmd represents the credential command
var credentialCmd = &cobra.Command{
	Use:   "credential",
	Short: "Manage application credentials",
	Long: `Manage credentials for applications in the microgateway.

Credentials provide secure access for applications to use the gateway.
Each credential consists of a key ID and secret pair that apps use for authentication.`,
	Aliases: []string{"cred", "credentials"},
}

// credentialListCmd lists credentials for an app
var credentialListCmd = &cobra.Command{
	Use:   "list <app-id>",
	Short: "List credentials for an application",
	Long:  "List all credentials associated with a specific application.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := args[0]
		
		resp, err := cli.GetClient().Get("/api/v1/apps/" + appID + "/credentials")
		if err != nil {
			return fmt.Errorf("failed to list credentials: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// credentialCreateCmd creates a new credential
var credentialCreateCmd = &cobra.Command{
	Use:   "create <app-id>",
	Short: "Create a new credential for an application",
	Long: `Create a new credential for an application.

This generates a new key ID and secret pair that the application can use
for authentication. The secret is only shown once during creation.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := args[0]
		name, _ := cmd.Flags().GetString("name")
		expiresStr, _ := cmd.Flags().GetString("expires")

		req := cli.CreateCredentialRequest{
			Name: name,
		}

		// Parse expiration date if provided
		if expiresStr != "" {
			expiresAt, err := time.Parse(time.RFC3339, expiresStr)
			if err != nil {
				return fmt.Errorf("invalid expires date format (use RFC3339: 2006-01-02T15:04:05Z): %w", err)
			}
			req.ExpiresAt = &expiresAt
		}

		resp, err := cli.GetClient().Post("/api/v1/apps/"+appID+"/credentials", req)
		if err != nil {
			return fmt.Errorf("failed to create credential: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		if resp.Message != "" && strings.Contains(resp.Message, "Save the secret") {
			cli.PrintWarning("This is the only time the secret will be displayed!")
		}
		
		return cli.PrintOutput(resp.Data)
	},
}

// credentialDeleteCmd deletes a credential
var credentialDeleteCmd = &cobra.Command{
	Use:   "delete <app-id> <credential-id>",
	Short: "Delete a credential",
	Long:  "Delete a credential from an application. This will immediately invalidate the credential.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := args[0]
		credID := args[1]
		
		resp, err := cli.GetClient().Delete("/api/v1/apps/" + appID + "/credentials/" + credID)
		if err != nil {
			return fmt.Errorf("failed to delete credential: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return nil
	},
}

func init() {
	// Add subcommands
	credentialCmd.AddCommand(credentialListCmd)
	credentialCmd.AddCommand(credentialCreateCmd)
	credentialCmd.AddCommand(credentialDeleteCmd)

	// credential create flags
	credentialCreateCmd.Flags().String("name", "", "credential name")
	credentialCreateCmd.Flags().String("expires", "", "expiration date in RFC3339 format (e.g., 2024-12-31T23:59:59Z)")
}