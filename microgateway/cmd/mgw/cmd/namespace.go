// cmd/mgw/cmd/namespace.go
package cmd

import (
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/cli"
	"github.com/spf13/cobra"
)

// namespaceCmd represents the namespace command
var namespaceCmd = &cobra.Command{
	Use:   "namespace",
	Short: "Manage hub-and-spoke namespace operations",
	Long: `Control and monitor edge instances by namespace in hub-and-spoke deployments.

Namespace operations allow you to coordinate distributed configuration reloads,
monitor edge instance status, and manage namespace-specific deployments.

This command is only available when connected to a control instance (hub mode).`,
	Aliases: []string{"ns"},
}

// namespaceReloadCmd reloads configuration for all edges in a namespace
var namespaceReloadCmd = &cobra.Command{
	Use:   "reload <namespace|all>",
	Short: "Reload configuration for all edges in namespace",
	Long: `Trigger configuration reload for all edge instances in the specified namespace.

This command will:
1. Send reload requests to all connected edges in the namespace
2. Monitor reload progress in real-time
3. Report the final status of each edge instance

Use "all" to reload all edges across all namespaces.
Use specific namespace name to reload only edges in that namespace.

Examples:
  mgw namespace reload tenant-a    # Reload all edges in tenant-a namespace
  mgw namespace reload all         # Reload all edges across all namespaces
  mgw namespace reload ""          # Reload all edges in global namespace`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		namespace := args[0]
		timeout, _ := cmd.Flags().GetInt("timeout")
		watch, _ := cmd.Flags().GetBool("watch")
		
		// Initiate namespace reload
		req := map[string]interface{}{
			"target_namespace": namespace,
			"timeout_seconds":  timeout,
			"initiated_by":     "mgw-cli",
		}
		
		resp, err := cli.GetClient().Post("/api/v1/namespace/reload", req)
		if err != nil {
			return fmt.Errorf("failed to initiate namespace reload: %w", err)
		}
		
		if resp.Data == nil {
			return fmt.Errorf("invalid response from server")
		}
		
		// Extract operation ID from response
		operationID := ""
		if dataMap, ok := resp.Data.(map[string]interface{}); ok {
			if opID, exists := dataMap["operation_id"]; exists {
				operationID = fmt.Sprintf("%v", opID)
			}
		}
		
		if operationID == "" {
			return fmt.Errorf("missing operation_id in response")
		}
		
		cli.PrintSuccess(fmt.Sprintf("Reload operation initiated: %s", operationID))
		
		if watch {
			return watchReloadProgress(operationID, timeout)
		}
		
		fmt.Printf("\nTo monitor progress, run: mgw namespace status %s\n", operationID)
		return cli.PrintOutput(resp.Data)
	},
}

// namespaceStatusCmd shows status of namespace reload operations
var namespaceStatusCmd = &cobra.Command{
	Use:   "status [operation-id]",
	Short: "Show namespace reload operation status",
	Long: `Display the status of namespace reload operations.

Without operation-id: Shows all active reload operations
With operation-id: Shows detailed status of a specific operation including per-edge progress`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			// List all active operations
			resp, err := cli.GetClient().Get("/api/v1/namespace/reload/operations")
			if err != nil {
				return fmt.Errorf("failed to get reload operations: %w", err)
			}
			return cli.PrintOutput(resp.Data)
		}
		
		// Get specific operation status
		operationID := args[0]
		resp, err := cli.GetClient().Get("/api/v1/namespace/reload/" + operationID + "/status")
		if err != nil {
			return fmt.Errorf("failed to get operation status: %w", err)
		}
		
		return cli.PrintOutput(resp.Data)
	},
}

// edgeCmd represents edge-specific operations
var edgeCmd = &cobra.Command{
	Use:   "edge",
	Short: "Manage individual edge instances",
	Long:  "Control and monitor individual edge instances in hub-and-spoke deployments.",
}

// edgeReloadCmd reloads configuration for specific edge instances
var edgeReloadCmd = &cobra.Command{
	Use:   "reload <edge-id> [edge-id...]",
	Short: "Reload configuration for specific edge instances",
	Long: `Trigger configuration reload for specific edge instances.

This allows targeted reloads without affecting all edges in a namespace.

Examples:
  mgw edge reload edge-1                    # Reload single edge
  mgw edge reload edge-1 edge-2 edge-3     # Reload multiple edges`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		edgeIDs := args
		timeout, _ := cmd.Flags().GetInt("timeout")
		watch, _ := cmd.Flags().GetBool("watch")
		
		// Initiate edge reload
		req := map[string]interface{}{
			"target_edges":    edgeIDs,
			"timeout_seconds": timeout,
			"initiated_by":    "mgw-cli",
		}
		
		resp, err := cli.GetClient().Post("/api/v1/edge/reload", req)
		if err != nil {
			return fmt.Errorf("failed to initiate edge reload: %w", err)
		}
		
		// Extract operation ID from response
		operationID := ""
		if dataMap, ok := resp.Data.(map[string]interface{}); ok {
			if opID, exists := dataMap["operation_id"]; exists {
				operationID = fmt.Sprintf("%v", opID)
			}
		}
		
		if operationID == "" {
			return fmt.Errorf("missing operation_id in response")
		}
		
		cli.PrintSuccess(fmt.Sprintf("Edge reload operation initiated: %s", operationID))
		
		if watch {
			return watchReloadProgress(operationID, timeout)
		}
		
		fmt.Printf("\nTo monitor progress, run: mgw edge status %s\n", operationID)
		return cli.PrintOutput(resp.Data)
	},
}

// edgeStatusCmd shows status of edge instances
var edgeStatusCmd = &cobra.Command{
	Use:   "status [operation-id]",
	Short: "Show edge instance status",
	Long: `Display status of edge instances and reload operations.

Without operation-id: Shows all connected edge instances
With operation-id: Shows detailed status of a specific reload operation`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			// List all edge instances
			resp, err := cli.GetClient().Get("/api/v1/edge/status")
			if err != nil {
				return fmt.Errorf("failed to get edge status: %w", err)
			}
			return cli.PrintOutput(resp.Data)
		}
		
		// Get specific operation status
		operationID := args[0]
		resp, err := cli.GetClient().Get("/api/v1/edge/reload/" + operationID + "/status")
		if err != nil {
			return fmt.Errorf("failed to get operation status: %w", err)
		}
		
		return cli.PrintOutput(resp.Data)
	},
}

// watchReloadProgress monitors reload operation progress with real-time updates
func watchReloadProgress(operationID string, timeoutSeconds int) error {
	fmt.Printf("\n🔄 Monitoring reload operation: %s\n", operationID)
	fmt.Printf("Press Ctrl+C to stop monitoring\n\n")
	
	timeout := time.After(time.Duration(timeoutSeconds) * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	lastStatus := ""
	
	for {
		select {
		case <-timeout:
			fmt.Printf("\n⏰ Monitoring timeout reached\n")
			return nil
			
		case <-ticker.C:
			resp, err := cli.GetClient().Get("/api/v1/namespace/reload/" + operationID + "/status")
			if err != nil {
				fmt.Printf("❌ Error getting status: %v\n", err)
				continue
			}
			
			if resp.Data == nil {
				continue
			}
			
			// Print status table if changed
			currentStatus := fmt.Sprintf("%v", resp.Data)
			if currentStatus != lastStatus {
				printReloadStatusTable(resp.Data)
				lastStatus = currentStatus
				
				// Check if operation is complete
				if isReloadComplete(resp.Data) {
					fmt.Printf("\n✅ Reload operation completed!\n")
					return nil
				}
			}
		}
	}
}

// printReloadStatusTable prints a formatted table of edge reload status
func printReloadStatusTable(data interface{}) {
	fmt.Printf("\n📊 Reload Status:\n")
	
	// Handle different data structures
	switch v := data.(type) {
	case map[string]interface{}:
		// Pretty print operation status
		if operationID, ok := v["operation_id"].(string); ok {
			fmt.Printf("Operation ID: %s\n", operationID)
		}
		if status, ok := v["status"].(string); ok {
			fmt.Printf("Status: %s\n", status)
		}
		if message, ok := v["message"].(string); ok {
			fmt.Printf("Message: %s\n", message)
		}
		if edges, ok := v["edges"].([]interface{}); ok {
			fmt.Printf("\nEdge Status:\n")
			fmt.Printf("%-20s %-15s %-30s\n", "Edge ID", "Status", "Message")
			fmt.Printf("%-20s %-15s %-30s\n", "-------", "------", "-------")
			for _, edge := range edges {
				if edgeMap, ok := edge.(map[string]interface{}); ok {
					edgeID := getStringValue(edgeMap, "edge_id", "unknown")
					edgeStatus := getStringValue(edgeMap, "status", "unknown")
					edgeMessage := getStringValue(edgeMap, "message", "")
					fmt.Printf("%-20s %-15s %-30s\n", edgeID, edgeStatus, edgeMessage)
				}
			}
		}
	default:
		// Fallback to previous behavior for unknown structures
		fmt.Printf("%+v\n", data)
	}
}

// getStringValue safely extracts a string value from a map
func getStringValue(m map[string]interface{}, key, defaultValue string) string {
	if value, exists := m[key]; exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return defaultValue
}

// isReloadComplete checks if the reload operation is finished
func isReloadComplete(data interface{}) bool {
	// TODO: Check operation status field to determine if complete
	if dataMap, ok := data.(map[string]interface{}); ok {
		if status, exists := dataMap["status"]; exists {
			statusStr := fmt.Sprintf("%v", status)
			return statusStr == "completed" || statusStr == "failed" || statusStr == "timed_out"
		}
	}
	return false
}

func init() {
	// Add namespace subcommands
	namespaceCmd.AddCommand(namespaceReloadCmd)
	namespaceCmd.AddCommand(namespaceStatusCmd)
	
	// Add edge subcommands
	edgeCmd.AddCommand(edgeReloadCmd)
	edgeCmd.AddCommand(edgeStatusCmd)

	// namespace reload flags
	namespaceReloadCmd.Flags().Int("timeout", 300, "operation timeout in seconds")
	namespaceReloadCmd.Flags().Bool("watch", false, "watch reload progress in real-time")

	// edge reload flags
	edgeReloadCmd.Flags().Int("timeout", 300, "operation timeout in seconds")
	edgeReloadCmd.Flags().Bool("watch", false, "watch reload progress in real-time")
}