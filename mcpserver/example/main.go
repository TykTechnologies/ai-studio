package main

import (
	"context"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/mcpserver"
)

// SimpleServer implements the ServerHandler interface
type SimpleServer struct {
	// Add any state you need
	resources map[string]mcpserver.Resource
	prompts   map[string]mcpserver.Prompt
	tools     map[string]mcpserver.Tool
}

func NewSimpleServer() *SimpleServer {
	return &SimpleServer{
		resources: make(map[string]mcpserver.Resource),
		prompts:   make(map[string]mcpserver.Prompt),
		tools:     make(map[string]mcpserver.Tool),
	}
}

// Resource methods
func (s *SimpleServer) ListResources(ctx context.Context, cursor string) ([]mcpserver.Resource, string, error) {
	// Simple implementation just returns all resources
	resources := make([]mcpserver.Resource, 0, len(s.resources))
	for _, r := range s.resources {
		resources = append(resources, r)
	}
	return resources, "", nil // No cursor implementation for simplicity
}

func (s *SimpleServer) ReadResource(ctx context.Context, uri string) ([]interface{}, error) {
	resource, exists := s.resources[uri]
	if !exists {
		return nil, fmt.Errorf("resource not found: %s", uri)
	}

	// Example returning a text resource
	content := mcpserver.TextResourceContents{
		URI:      uri,
		Text:     fmt.Sprintf("Content for %s", resource.Name),
		MimeType: "text/plain",
	}

	return []interface{}{content}, nil
}

func (s *SimpleServer) SubscribeResource(ctx context.Context, uri string, callback func(string)) error {
	// Simple implementation just stores the callback
	return nil
}

func (s *SimpleServer) UnsubscribeResource(ctx context.Context, uri string) error {
	return nil
}

// Prompt methods
func (s *SimpleServer) ListPrompts(ctx context.Context, cursor string) ([]mcpserver.Prompt, string, error) {
	prompts := make([]mcpserver.Prompt, 0, len(s.prompts))
	for _, p := range s.prompts {
		prompts = append(prompts, p)
	}
	return prompts, "", nil
}

func (s *SimpleServer) GetPrompt(ctx context.Context, name string, args map[string]string) (*mcpserver.GetPromptResult, error) {
	prompt, exists := s.prompts[name]
	if !exists {
		return nil, fmt.Errorf("prompt not found: %s", name)
	}

	// Example response
	return &mcpserver.GetPromptResult{
		Messages: []mcpserver.PromptMessage{
			{
				Role: mcpserver.RoleAssistant,
				Content: mcpserver.TextContent{
					Type: "text",
					Text: fmt.Sprintf("This is prompt %s", prompt.Name),
				},
			},
		},
		Description: prompt.Description,
	}, nil
}

// Tool methods
func (s *SimpleServer) ListTools(ctx context.Context, cursor string) ([]mcpserver.Tool, string, error) {
	tools := make([]mcpserver.Tool, 0, len(s.tools))
	for _, t := range s.tools {
		tools = append(tools, t)
	}
	return tools, "", nil
}

func (s *SimpleServer) CallTool(ctx context.Context, name string, args map[string]interface{}) (*mcpserver.CallToolResult, error) {
	tool, exists := s.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	// Example response
	return &mcpserver.CallToolResult{
		Content: []interface{}{
			mcpserver.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Result from tool %s", tool.Name),
			},
		},
		IsError: false,
	}, nil
}

// Completion method
func (s *SimpleServer) Complete(ctx context.Context, req mcpserver.CompleteRequest) (*mcpserver.CompleteResult, error) {
	return &mcpserver.CompleteResult{
		Completion: struct {
			Values  []string `json:"values"`
			HasMore bool     `json:"hasMore,omitempty"`
			Total   int      `json:"total,omitempty"`
		}{
			Values: []string{"completion1", "completion2"},
			Total:  2,
		},
	}, nil
}

// Sampling method
func (s *SimpleServer) CreateMessage(ctx context.Context, req mcpserver.CreateMessageRequest) (*mcpserver.CreateMessageResult, error) {
	return &mcpserver.CreateMessageResult{
		Content: mcpserver.TextContent{
			Type: "text",
			Text: "Generated response",
		},
		Model: "example-model",
		Role:  mcpserver.RoleAssistant,
	}, nil
}

// Example usage:
func main() {
	// Create the handler implementation
	handler := NewSimpleServer()

	// Add some example resources
	handler.resources["file:///example.txt"] = mcpserver.Resource{
		Name: "Example Resource",
		URI:  "file:///example.txt",
	}

	// Create the MCP server
	server := mcpserver.NewServer(mcpserver.ServerConfig{
		Implementation: mcpserver.Implementation{
			Name:    "SimpleServer",
			Version: "1.0.0",
		},
		Capabilities: mcpserver.ServerCapabilities{
			Resources: &struct {
				ListChanged bool `json:"listChanged"`
				Subscribe   bool `json:"subscribe"`
			}{
				ListChanged: true,
				Subscribe:   true,
			},
		},
		Handler: handler,
		NotificationHandler: func(ctx context.Context, notification interface{}) error {
			// Handle notifications (e.g., send to client)
			// fmt.Printf("Notification: %+v\n", notification)
			return nil
		},
	})

	// Example request handling
	ctx := context.Background()
	request := []byte(`{
        "jsonrpc": "2.0",
        "id": 1,
        "method": "resources/list",
        "params": {}
    }`)

	response, err := server.HandleRequest(ctx, request)
	if err != nil {
		panic(err)
	}

	// fmt.Printf("Response: %s\n", response)
}
