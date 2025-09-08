// cmd/mgw/cmd/token.go
package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/cli"
	"github.com/spf13/cobra"
)

// tokenCmd represents the token command
var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage API tokens",
	Long: `Manage API tokens for authentication with the microgateway.

API tokens are used to authenticate requests to the microgateway API and gateway endpoints.
Tokens can have different scopes and expiration times.`,
	Aliases: []string{"tokens"},
}

// tokenListCmd lists tokens
var tokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List API tokens",
	Long:  "List all API tokens, optionally filtered by application ID.",
	RunE: func(cmd *cobra.Command, args []string) error {
		appID, _ := cmd.Flags().GetString("app-id")

		params := make(map[string]string)
		if appID != "" {
			params["app_id"] = appID
		}

		resp, err := cli.GetClient().GetWithQuery("/api/v1/tokens", params)
		if err != nil {
			return fmt.Errorf("failed to list tokens: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// tokenCreateCmd creates a new token
var tokenCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new API token",
	Long:  "Create a new API token for an application with specified scopes and expiration.",
	RunE: func(cmd *cobra.Command, args []string) error {
		appID, _ := cmd.Flags().GetUint("app-id")
		name, _ := cmd.Flags().GetString("name")
		scopesStr, _ := cmd.Flags().GetString("scopes")
		expiresStr, _ := cmd.Flags().GetString("expires")

		if appID == 0 {
			return fmt.Errorf("app-id is required")
		}
		if name == "" {
			return fmt.Errorf("name is required")
		}

		req := cli.GenerateTokenRequest{
			AppID: appID,
			Name:  name,
		}

		// Parse scopes
		if scopesStr != "" {
			req.Scopes = strings.Split(scopesStr, ",")
			for i := range req.Scopes {
				req.Scopes[i] = strings.TrimSpace(req.Scopes[i])
			}
		}

		// Parse expiration
		if expiresStr != "" {
			duration, err := time.ParseDuration(expiresStr)
			if err != nil {
				return fmt.Errorf("invalid expires duration (use format like '24h', '7d', '30m'): %w", err)
			}
			req.ExpiresIn = duration
		}

		resp, err := cli.GetClient().Post("/api/v1/tokens", req)
		if err != nil {
			return fmt.Errorf("failed to create token: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		cli.PrintWarning("Save the token - it won't be shown again!")
		
		return cli.PrintOutput(resp.Data)
	},
}

// tokenRevokeCmd revokes a token
var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke <token>",
	Short: "Revoke an API token",
	Long:  "Revoke an API token, immediately invalidating it for all future requests.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		token := args[0]
		
		resp, err := cli.GetClient().Delete("/api/v1/tokens/" + token)
		if err != nil {
			return fmt.Errorf("failed to revoke token: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return nil
	},
}

// tokenInfoCmd gets token information
var tokenInfoCmd = &cobra.Command{
	Use:   "info <token>",
	Short: "Get API token information",
	Long:  "Retrieve information about an API token including its scopes and expiration.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		token := args[0]
		
		resp, err := cli.GetClient().Get("/api/v1/tokens/" + token)
		if err != nil {
			return fmt.Errorf("failed to get token info: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// tokenValidateCmd validates a token
var tokenValidateCmd = &cobra.Command{
	Use:   "validate <token>",
	Short: "Validate an API token",
	Long:  "Check if an API token is valid and retrieve its authentication details.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		token := args[0]
		
		req := map[string]string{"token": token}
		resp, err := cli.GetClient().Post("/api/v1/auth/validate", req)
		if err != nil {
			return fmt.Errorf("failed to validate token: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

func init() {
	// Add subcommands
	tokenCmd.AddCommand(tokenListCmd)
	tokenCmd.AddCommand(tokenCreateCmd)
	tokenCmd.AddCommand(tokenRevokeCmd)
	tokenCmd.AddCommand(tokenInfoCmd)
	tokenCmd.AddCommand(tokenValidateCmd)

	// token list flags
	tokenListCmd.Flags().String("app-id", "", "filter by application ID")

	// token create flags
	tokenCreateCmd.Flags().Uint("app-id", 0, "application ID (required)")
	tokenCreateCmd.Flags().String("name", "", "token name (required)")
	tokenCreateCmd.Flags().String("scopes", "", "comma-separated list of scopes (e.g., 'admin,read')")
	tokenCreateCmd.Flags().String("expires", "", "expiration duration (e.g., '24h', '7d', '30m')")
	tokenCreateCmd.MarkFlagRequired("app-id")
	tokenCreateCmd.MarkFlagRequired("name")
}