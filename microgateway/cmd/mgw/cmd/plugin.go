// cmd/mgw/cmd/plugin.go
package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/TykTechnologies/midsommar/microgateway/internal/cli"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// pluginCmd represents the plugin command
var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage plugin configurations",
	Long: `Manage plugin configurations in the microgateway.

This command allows you to create, list, update, and delete plugin configurations,
as well as manage plugin associations with LLMs.`,
}

// pluginListCmd lists all plugins
var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List plugin configurations",
	Long:  "List all plugin configurations with optional filtering by hook type and active status.",
	RunE: func(cmd *cobra.Command, args []string) error {
		hookType, _ := cmd.Flags().GetString("hook-type")
		active, _ := cmd.Flags().GetBool("active")
		page, _ := cmd.Flags().GetInt("page")
		limit, _ := cmd.Flags().GetInt("limit")
		detailed, _ := cmd.Flags().GetBool("detailed")

		params := make(map[string]string)
		if hookType != "" {
			params["hook_type"] = hookType
		}
		if active {
			params["active"] = "true"
		} else {
			params["active"] = "false"
		}
		params["page"] = strconv.Itoa(page)
		params["limit"] = strconv.Itoa(limit)

		resp, err := cli.GetClient().GetWithQuery("/api/v1/plugins", params)
		if err != nil {
			return fmt.Errorf("failed to list plugins: %w", err)
		}

		// Show detailed output if requested
		if detailed {
			return cli.PrintOutput(resp.Data)
		}

		// Show compact table by default
		return cli.PrintOutput(resp.Data)
	},
}

// pluginCreateCmd creates a new plugin
var pluginCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new plugin configuration",
	Long:  "Create a new plugin configuration with the specified parameters.",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		slug, _ := cmd.Flags().GetString("slug")
		description, _ := cmd.Flags().GetString("description")
		command, _ := cmd.Flags().GetString("command")
		checksum, _ := cmd.Flags().GetString("checksum")
		hookType, _ := cmd.Flags().GetString("hook-type")
		active, _ := cmd.Flags().GetBool("active")
		configFile, _ := cmd.Flags().GetString("config-file")

		if name == "" {
			return fmt.Errorf("name is required")
		}
		if slug == "" {
			return fmt.Errorf("slug is required")
		}
		if command == "" {
			return fmt.Errorf("command is required")
		}
		if hookType == "" {
			return fmt.Errorf("hook-type is required")
		}

		// Validate hook type
		validHookTypes := []string{"pre_auth", "auth", "post_auth", "on_response"}
		isValid := false
		for _, validType := range validHookTypes {
			if hookType == validType {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid hook-type: %s. Valid types: pre_auth, auth, post_auth, on_response", hookType)
		}

		req := map[string]interface{}{
			"name":        name,
			"slug":        slug,
			"description": description,
			"command":     command,
			"checksum":    checksum,
			"hook_type":   hookType,
			"is_active":   active,
		}

		// Parse config file if provided
		if configFile != "" {
			configData, err := parseConfigFile(configFile)
			if err != nil {
				return fmt.Errorf("failed to parse config file: %w", err)
			}
			// Merge config file data into request
			if req["config"] == nil {
				req["config"] = make(map[string]interface{})
			}
			for key, value := range configData {
				req["config"].(map[string]interface{})[key] = value
			}
			fmt.Printf("✓ Loaded configuration from %s\n", configFile)
		}

		resp, err := cli.GetClient().Post("/api/v1/plugins", req)
		if err != nil {
			return fmt.Errorf("failed to create plugin: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return cli.PrintOutput(resp.Data)
	},
}

// pluginGetCmd gets a specific plugin
var pluginGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get plugin configuration by ID",
	Long:  "Retrieve detailed information about a specific plugin configuration.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		
		resp, err := cli.GetClient().Get("/api/v1/plugins/" + id)
		if err != nil {
			return fmt.Errorf("failed to get plugin: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// pluginUpdateCmd updates a plugin
var pluginUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update plugin configuration",
	Long:  "Update an existing plugin configuration with new parameters.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		
		req := make(map[string]interface{})
		
		if name, _ := cmd.Flags().GetString("name"); name != "" {
			req["name"] = name
		}
		if description, _ := cmd.Flags().GetString("description"); description != "" {
			req["description"] = description
		}
		if command, _ := cmd.Flags().GetString("command"); command != "" {
			req["command"] = command
		}
		if checksum, _ := cmd.Flags().GetString("checksum"); checksum != "" {
			req["checksum"] = checksum
		}
		if hookType, _ := cmd.Flags().GetString("hook-type"); hookType != "" {
			// Validate hook type
			validHookTypes := []string{"pre_auth", "auth", "post_auth", "on_response"}
			isValid := false
			for _, validType := range validHookTypes {
				if hookType == validType {
					isValid = true
					break
				}
			}
			if !isValid {
				return fmt.Errorf("invalid hook-type: %s. Valid types: pre_auth, auth, post_auth, on_response", hookType)
			}
			req["hook_type"] = hookType
		}
		if cmd.Flags().Changed("active") {
			val, _ := cmd.Flags().GetBool("active")
			req["is_active"] = val
		}

		if len(req) == 0 {
			return fmt.Errorf("no fields specified to update")
		}

		resp, err := cli.GetClient().Put("/api/v1/plugins/"+id, req)
		if err != nil {
			return fmt.Errorf("failed to update plugin: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return cli.PrintOutput(resp.Data)
	},
}

// pluginDeleteCmd deletes a plugin
var pluginDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete plugin configuration",
	Long:  "Delete (soft delete) a plugin configuration. This will disable the plugin but preserve historical data.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		
		resp, err := cli.GetClient().Delete("/api/v1/plugins/" + id)
		if err != nil {
			return fmt.Errorf("failed to delete plugin: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return nil
	},
}

// pluginTestCmd tests a plugin
var pluginTestCmd = &cobra.Command{
	Use:   "test <id>",
	Short: "Test plugin functionality",
	Long:  "Test a plugin by loading it and running basic checks.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		testFile, _ := cmd.Flags().GetString("test-file")
		
		req := make(map[string]interface{})
		if testFile != "" {
			req["test_file"] = testFile
		}
		
		resp, err := cli.GetClient().Post("/api/v1/plugins/"+id+"/test", req)
		if err != nil {
			return fmt.Errorf("failed to test plugin: %w", err)
		}

		cli.PrintSuccess(resp.Message)
		return cli.PrintOutput(resp.Data)
	},
}

func init() {
	// Add subcommands
	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginCreateCmd)
	pluginCmd.AddCommand(pluginGetCmd)
	pluginCmd.AddCommand(pluginUpdateCmd)
	pluginCmd.AddCommand(pluginDeleteCmd)
	pluginCmd.AddCommand(pluginTestCmd)

	// plugin list flags
	pluginListCmd.Flags().String("hook-type", "", "filter by hook type (pre_auth, auth, post_auth, on_response)")
	pluginListCmd.Flags().Bool("active", true, "filter by active status")
	pluginListCmd.Flags().Int("page", 1, "page number")
	pluginListCmd.Flags().Int("limit", 20, "items per page")
	pluginListCmd.Flags().Bool("detailed", false, "show all columns (default: compact view)")

	// plugin create flags
	pluginCreateCmd.Flags().String("name", "", "Plugin name (required)")
	pluginCreateCmd.Flags().String("slug", "", "Plugin slug (required)")
	pluginCreateCmd.Flags().String("description", "", "Plugin description")
	pluginCreateCmd.Flags().String("command", "", "Plugin executable command (required)")
	pluginCreateCmd.Flags().String("checksum", "", "Plugin file checksum (SHA256)")
	pluginCreateCmd.Flags().String("hook-type", "", "Plugin hook type: pre_auth, auth, post_auth, on_response (required)")
	pluginCreateCmd.Flags().Bool("active", true, "Whether plugin is active")
	pluginCreateCmd.Flags().String("config-file", "", "YAML config file for plugin")
	pluginCreateCmd.MarkFlagRequired("name")
	pluginCreateCmd.MarkFlagRequired("slug")
	pluginCreateCmd.MarkFlagRequired("command")
	pluginCreateCmd.MarkFlagRequired("hook-type")

	// plugin update flags
	pluginUpdateCmd.Flags().String("name", "", "Plugin name")
	pluginUpdateCmd.Flags().String("description", "", "Plugin description")
	pluginUpdateCmd.Flags().String("command", "", "Plugin executable command")
	pluginUpdateCmd.Flags().String("checksum", "", "Plugin file checksum (SHA256)")
	pluginUpdateCmd.Flags().String("hook-type", "", "Plugin hook type: pre_auth, auth, post_auth, on_response")
	pluginUpdateCmd.Flags().Bool("active", true, "Whether plugin is active")

	// plugin test flags
	pluginTestCmd.Flags().String("test-file", "", "JSON file containing test data")
}

// parseConfigFile parses a YAML or JSON config file
func parseConfigFile(filePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	var config map[string]interface{}
	
	// Try to parse as YAML first (supports both YAML and JSON)
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file as YAML: %w", err)
	}
	
	return config, nil
}