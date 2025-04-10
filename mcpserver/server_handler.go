package mcpserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/ThinkInAIXYZ/go-mcp/protocol"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
	"gorm.io/gorm"
)

// ToolHandler handles tool operations for MCP servers
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

// HandleToolRequest handles a tool request from the MCP protocol
func (h *ToolHandler) HandleToolRequest(req *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
	log.Printf("Handling tool request for %s", req.Name)

	// Parse the tool name to get the original tool and operation
	// Format is typically "Tool Name_operationName"
	// We need to handle space in tool names, so find the last underscore
	reqName := req.Name
	lastUnderscoreIndex := strings.LastIndex(reqName, "_")
	if lastUnderscoreIndex == -1 {
		// No underscore found, assume the whole string is the tool name
		return &protocol.CallToolResult{
			IsError: true,
			Content: []protocol.Content{
				&protocol.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Invalid tool name format (no operation): %s", reqName),
				},
			},
		}, nil
	}

	// Extract tool name and operation
	toolName := reqName[:lastUnderscoreIndex]
	opName := reqName[lastUnderscoreIndex+1:]

	log.Printf("Parsed tool name: '%s', operation: '%s'", toolName, opName)

	// Find the tool
	tool, ok := h.toolMap[toolName]
	if !ok {
		// Return error response with proper protocol.TextContent
		return &protocol.CallToolResult{
			IsError: true,
			Content: []protocol.Content{
				&protocol.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Tool not found: %s", toolName),
				},
			},
		}, nil
	}

	// Use the universal client to execute the operation
	log.Printf("Executing operation %s on tool %s", opName, toolName)

	// Create a universal client for the tool
	opts := []universalclient.ClientOption{}

	// Add authentication if specified
	if tool.AuthSchemaName != "" && tool.AuthKey != "" {
		opts = append(opts, universalclient.WithAuth(tool.AuthSchemaName, tool.AuthKey))
	}

	// Attempt to decode the OAS spec as base64 (it may be stored encoded)
	specBytes := []byte(tool.OASSpec)
	decodedSpec, err := base64.StdEncoding.DecodeString(tool.OASSpec)
	if err == nil {
		// Successfully decoded as base64
		log.Printf("Successfully decoded base64 OAS spec for tool %s", toolName)
		specBytes = decodedSpec
	}

	// Create client with potentially decoded spec
	uc, err := universalclient.NewClient(specBytes, "", opts...)
	if err != nil {
		// Return error response with proper protocol.TextContent
		return &protocol.CallToolResult{
			IsError: true,
			Content: []protocol.Content{
				&protocol.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to create client: %v", err),
				},
			},
		}, nil
	}

	// Parse the arguments into the required format
	var args map[string]interface{}
	if err := json.Unmarshal(req.RawArguments, &args); err != nil {
		// Return error response with proper protocol.TextContent
		return &protocol.CallToolResult{
			IsError: true,
			Content: []protocol.Content{
				&protocol.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to parse arguments: %v", err),
				},
			},
		}, nil
	}

	// Execute the operation
	opResult, err := uc.CallOperation(opName, nil, args, nil)
	if err != nil {
		// Return error response with proper protocol.TextContent
		return &protocol.CallToolResult{
			IsError: true,
			Content: []protocol.Content{
				&protocol.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Operation failed: %v", err),
				},
			},
		}, nil
	}

	// Convert the result to a string
	resultBytes, err := json.MarshalIndent(opResult, "", "  ")
	if err != nil {
		// Return error response with proper protocol.TextContent
		return &protocol.CallToolResult{
			IsError: true,
			Content: []protocol.Content{
				&protocol.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to format result: %v", err),
				},
			},
		}, nil
	}

	// Create success result with proper protocol.TextContent
	return &protocol.CallToolResult{
		Content: []protocol.Content{
			&protocol.TextContent{
				Type: "text",
				Text: string(resultBytes),
			},
		},
	}, nil
}
