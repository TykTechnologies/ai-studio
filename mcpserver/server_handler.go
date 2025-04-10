package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/MegaGrindStone/go-mcp"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
	"gorm.io/gorm"
)

// ToolHandler implements the mcp.ToolServer interface
type ToolHandler struct {
	db          *gorm.DB
	mcpServerID uint
	tools       []models.Tool
	toolMap     map[string]models.Tool
}

// NewToolHandler creates a new MCP server handler
func NewToolHandler(db *gorm.DB, mcpServerID uint, tools []models.Tool) *ToolHandler {
	toolMap := make(map[string]models.Tool)
	for _, tool := range tools {
		toolMap[tool.Name] = tool
	}

	return &ToolHandler{
		db:          db,
		mcpServerID: mcpServerID,
		tools:       tools,
		toolMap:     toolMap,
	}
}

// ListTools returns a list of available tools
func (h *ToolHandler) ListTools(ctx context.Context, _ mcp.ListToolsParams, _ mcp.ProgressReporter) (mcp.ListToolsResult, error) {
	result := mcp.ListToolsResult{
		Tools: []mcp.Tool{},
	}

	for _, tool := range h.tools {
		// Get operations from the model
		operations := tool.GetOperations()

		// Skip tools with no operations
		if len(operations) == 0 {
			continue
		}

		for _, op := range operations {
			// Just log info, we'll use a simplified schema
			log.Printf("Adding tool operation: %s_%s", tool.Name, op)

			// For now, we'll use a simplified schema since we don't have GetOperationSpecBytes
			schemaMap := map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"parameters": map[string]interface{}{
						"type": "object",
					},
					"body": map[string]interface{}{
						"type": "object",
					},
				},
			}

			// Convert schema map to JSON
			schemaBytes, err := json.Marshal(schemaMap)
			if err != nil {
				log.Printf("Failed to marshal schema for operation %s: %v", op, err)
				continue
			}

			// Add the tool
			mcpTool := mcp.Tool{
				Name:        fmt.Sprintf("%s_%s", tool.Name, op),
				Description: fmt.Sprintf("%s - %s", tool.Description, op),
				InputSchema: schemaBytes,
			}
			result.Tools = append(result.Tools, mcpTool)
		}
	}

	return result, nil
}

// CallTool executes a tool operation
func (h *ToolHandler) CallTool(ctx context.Context, params mcp.CallToolParams, _ mcp.ProgressReporter) (mcp.CallToolResult, error) {
	// Parse the tool name to get the original tool and operation
	var toolName, opName string
	fmt.Sscanf(params.Name, "%s_%s", &toolName, &opName)

	// Find the tool
	tool, ok := h.toolMap[toolName]
	if !ok {
		return mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				{
					Type: mcp.ContentTypeText,
					Text: fmt.Sprintf("Tool not found: %s", toolName),
				},
			},
		}, nil
	}

	// Create a universal client for the tool
	opts := []universalclient.ClientOption{}

	// Add authentication if specified
	if tool.AuthSchemaName != "" && tool.AuthKey != "" {
		opts = append(opts, universalclient.WithAuth(tool.AuthSchemaName, tool.AuthKey))
	}

	uc, err := universalclient.NewClient([]byte(tool.OASSpec), "", opts...)
	if err != nil {
		return mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				{
					Type: mcp.ContentTypeText,
					Text: fmt.Sprintf("Failed to create client: %v", err),
				},
			},
		}, nil
	}

	// Parse the arguments into the required format
	var args map[string]interface{}
	if err := json.Unmarshal(params.Arguments, &args); err != nil {
		return mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				{
					Type: mcp.ContentTypeText,
					Text: fmt.Sprintf("Failed to parse arguments: %v", err),
				},
			},
		}, nil
	}

	// Execute the operation
	result, err := uc.CallOperation(opName, nil, args, nil)
	if err != nil {
		return mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				{
					Type: mcp.ContentTypeText,
					Text: fmt.Sprintf("Operation failed: %v", err),
				},
			},
		}, nil
	}

	// Convert the result to a string
	resultBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				{
					Type: mcp.ContentTypeText,
					Text: fmt.Sprintf("Failed to format result: %v", err),
				},
			},
		}, nil
	}

	// Return the result
	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: string(resultBytes),
			},
		},
	}, nil
}
