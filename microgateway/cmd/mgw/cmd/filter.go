// cmd/mgw/cmd/filter.go
package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/microgateway/internal/cli"
	"github.com/spf13/cobra"
)

// filterCmd represents the filter command
var filterCmd = &cobra.Command{
	Use:   "filter",
	Short: "Manage filter configurations",
	Long: `Manage filter configurations in the microgateway.

Filters are Tengo scripts that can validate or modify requests before they reach LLM providers.
They can be used to block sensitive content, log requests, or enforce organizational policies.`,
}

// filterListCmd lists all filters
var filterListCmd = &cobra.Command{
	Use:   "list",
	Short: "List filter configurations",
	Long:  "List all filter configurations with optional filtering by active status.",
	RunE: func(cmd *cobra.Command, args []string) error {
		active, _ := cmd.Flags().GetBool("active")
		page, _ := cmd.Flags().GetInt("page")
		limit, _ := cmd.Flags().GetInt("limit")

		params := map[string]string{
			"active": strconv.FormatBool(active),
			"page":   strconv.Itoa(page),
			"limit":  strconv.Itoa(limit),
		}

		resp, err := cli.GetClient().GetWithQuery("/api/v1/filters", params)
		if err != nil {
			return fmt.Errorf("failed to list filters: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// filterCreateCmd creates a new filter
var filterCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new filter",
	Long: `Create a new filter with an interactive script editor.

This command will open your default editor ($EDITOR) to write the Tengo script.
The script should set a 'result' variable to true (allow) or false (block).

Example filter script:
  text := strings.lower(payload)
  if strings.contains(text, "@") && strings.contains(text, ".com") {
      result = false  // Block if email detected
  } else {
      result = true   // Allow otherwise
  }`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		description, _ := cmd.Flags().GetString("description")

		// Open editor to write script
		script, err := editScript("")
		if err != nil {
			return fmt.Errorf("failed to edit script: %w", err)
		}

		if script == "" {
			return fmt.Errorf("script cannot be empty")
		}

		// Create filter via API
		reqData := map[string]interface{}{
			"name":        name,
			"description": description,
			"script":      script,
			"is_active":   true,
		}

		resp, err := cli.GetClient().Post("/api/v1/filters", reqData)
		if err != nil {
			return fmt.Errorf("failed to create filter: %w", err)
		}

		fmt.Printf("Filter '%s' created successfully\n", name)
		return cli.PrintOutput(resp.Data)
	},
}

// filterEditCmd edits an existing filter
var filterEditCmd = &cobra.Command{
	Use:   "edit <filter-id>",
	Short: "Edit an existing filter",
	Long:  "Edit an existing filter script using your default editor.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filterID := args[0]

		// Get current filter
		resp, err := cli.GetClient().Get("/api/v1/filters/" + filterID)
		if err != nil {
			return fmt.Errorf("failed to get filter: %w", err)
		}

		filterData := resp.Data.(map[string]interface{})
		currentScript := filterData["script"].(string)

		// Edit script
		newScript, err := editScript(currentScript)
		if err != nil {
			return fmt.Errorf("failed to edit script: %w", err)
		}

		if newScript == currentScript {
			fmt.Println("No changes made")
			return nil
		}

		// Update filter
		reqData := map[string]interface{}{
			"script": newScript,
		}

		_, err = cli.GetClient().Put("/api/v1/filters/"+filterID, reqData)
		if err != nil {
			return fmt.Errorf("failed to update filter: %w", err)
		}

		fmt.Printf("Filter %s updated successfully\n", filterID)
		return nil
	},
}

// filterShowCmd shows filter details
var filterShowCmd = &cobra.Command{
	Use:   "show <filter-id>",
	Short: "Show filter details",
	Long:  "Display detailed information about a specific filter including its script.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filterID := args[0]

		resp, err := cli.GetClient().Get("/api/v1/filters/" + filterID)
		if err != nil {
			return fmt.Errorf("failed to get filter: %w", err)
		}

		return cli.PrintOutput(resp.Data)
	},
}

// filterDeleteCmd deletes a filter
var filterDeleteCmd = &cobra.Command{
	Use:   "delete <filter-id>",
	Short: "Delete a filter",
	Long:  "Delete a filter configuration. This will also remove its associations with LLMs.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filterID := args[0]

		_, err := cli.GetClient().Delete("/api/v1/filters/" + filterID)
		if err != nil {
			return fmt.Errorf("failed to delete filter: %w", err)
		}

		fmt.Printf("Filter %s deleted successfully\n", filterID)
		return nil
	},
}

// filterTestCmd tests a filter with sample payload
var filterTestCmd = &cobra.Command{
	Use:   "test <filter-id>",
	Short: "Test a filter with sample payload",
	Long:  "Test a filter script execution with a sample payload from a file or stdin.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement filter testing
		// This would send test payload to a special test endpoint
		return fmt.Errorf("filter testing not implemented yet")
	},
}

// editScript opens the user's editor to edit a Tengo script
func editScript(initialContent string) (string, error) {
	// Get editor from environment
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano" // fallback
	}

	// Create temporary file with .tengo extension for syntax highlighting
	tmpFile, err := ioutil.TempFile("", "filter-*.tengo")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write initial content if provided
	if initialContent != "" {
		if _, err := tmpFile.WriteString(initialContent); err != nil {
			return "", fmt.Errorf("failed to write initial content: %w", err)
		}
	} else {
		// Write template script
		template := `// Filter script template
// The 'payload' variable contains the request data as a string
// Set 'result' to true to allow the request, false to block it

text := import("text")

// Example: Block requests containing email addresses
if text.re_match("[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}", payload) {
    fmt.println("Filter: Email detected, blocking request")
    result = false
} else {
    result = true
}
`
		if _, err := tmpFile.WriteString(template); err != nil {
			return "", fmt.Errorf("failed to write template: %w", err)
		}
	}
	tmpFile.Close()

	// Open editor
	cmd := exec.Command(editor, tmpFile.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor failed: %w", err)
	}

	// Read the edited content
	content, err := ioutil.ReadFile(tmpFile.Name())
	if err != nil {
		return "", fmt.Errorf("failed to read edited content: %w", err)
	}

	return strings.TrimSpace(string(content)), nil
}

func init() {
	rootCmd.AddCommand(filterCmd)

	// Add subcommands
	filterCmd.AddCommand(filterListCmd)
	filterCmd.AddCommand(filterCreateCmd)
	filterCmd.AddCommand(filterEditCmd)
	filterCmd.AddCommand(filterShowCmd)
	filterCmd.AddCommand(filterDeleteCmd)
	filterCmd.AddCommand(filterTestCmd)

	// List command flags
	filterListCmd.Flags().BoolP("active", "a", true, "Filter by active status")
	filterListCmd.Flags().IntP("page", "p", 1, "Page number")
	filterListCmd.Flags().IntP("limit", "l", 20, "Number of results per page")

	// Create command flags
	filterCreateCmd.Flags().StringP("description", "d", "", "Filter description")

	// Test command flags
	filterTestCmd.Flags().StringP("payload", "", "", "Path to file containing test payload")
	filterTestCmd.Flags().StringP("data", "", "", "Test payload data as string")
}