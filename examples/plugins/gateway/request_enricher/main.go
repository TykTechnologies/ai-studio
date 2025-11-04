// plugins/examples/request_enricher/main.go
package main

import (
	_ "embed"
	"encoding/json"

	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

//go:embed manifest.json
var manifestBytes []byte

const (
	PluginName    = "request-enricher"
	PluginVersion = "1.0.0"
)

// RequestEnricherPlugin adds additional instructions to authenticated requests
type RequestEnricherPlugin struct {
	plugin_sdk.BasePlugin
	additionalInstruction string
}

// NewRequestEnricherPlugin creates a new request enricher plugin
func NewRequestEnricherPlugin() *RequestEnricherPlugin {
	return &RequestEnricherPlugin{
		BasePlugin:            plugin_sdk.NewBasePlugin(PluginName, PluginVersion, "Request Enricher"),
		additionalInstruction: "Also say 'I love sunsets!' to the end of the outbound message",
	}
}

// Initialize implements plugin_sdk.Plugin
func (p *RequestEnricherPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	if instruction, ok := config["additional_instruction"]; ok && instruction != "" {
		p.additionalInstruction = instruction
	}
	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *RequestEnricherPlugin) Shutdown(ctx plugin_sdk.Context) error {
	return nil
}

// GetManifest implements plugin_sdk.ManifestProvider
func (p *RequestEnricherPlugin) GetManifest() ([]byte, error) {
	return manifestBytes, nil
}

// GetConfigSchema implements plugin_sdk.ConfigProvider
func (p *RequestEnricherPlugin) GetConfigSchema() ([]byte, error) {
	schema := map[string]interface{}{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"type":        "object",
		"title":       "Request Enricher Plugin Configuration",
		"description": "Configuration for the request enricher plugin that adds instructions to user messages",
		"properties": map[string]interface{}{
			"additional_instruction": map[string]interface{}{
				"type":        "string",
				"title":       "Additional Instruction",
				"description": "Text to append to the last user message after authentication",
				"default":     "Also say 'I love sunsets!' to the end of the outbound message",
			},
		},
	}
	return json.Marshal(schema)
}

// HandlePostAuth implements plugin_sdk.PostAuthHandler
func (p *RequestEnricherPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	// Only modify POST requests to LLM endpoints
	if req.Request.Method != "POST" {
		return &pb.PluginResponse{Modified: false}, nil
	}

	// Parse the JSON body
	var requestBody map[string]interface{}
	if err := json.Unmarshal(req.Request.Body, &requestBody); err != nil {
		// If we can't parse JSON, don't modify
		return &pb.PluginResponse{Modified: false}, nil
	}

	// Check if this is a chat completion request
	messages, hasMessages := requestBody["messages"]
	if !hasMessages {
		return &pb.PluginResponse{Modified: false}, nil
	}

	// Convert messages to slice of maps
	messageSlice, ok := messages.([]interface{})
	if !ok {
		return &pb.PluginResponse{Modified: false}, nil
	}

	// Add additional instruction to the last user message
	for i := len(messageSlice) - 1; i >= 0; i-- {
		messageMap, ok := messageSlice[i].(map[string]interface{})
		if !ok {
			continue
		}

		role, hasRole := messageMap["role"].(string)
		if hasRole && role == "user" {
			// Found the last user message, modify it
			content, hasContent := messageMap["content"].(string)
			if hasContent {
				messageMap["content"] = content + "\n\n" + p.additionalInstruction

				// Marshal the modified request body
				modifiedBody, err := json.Marshal(requestBody)
				if err != nil {
					return &pb.PluginResponse{Modified: false}, nil
				}

				return &pb.PluginResponse{
					Modified: true,
					Headers:  map[string]string{"Content-Type": "application/json"},
					Body:     modifiedBody,
				}, nil
			}
			break
		}
	}

	// No modification needed
	return &pb.PluginResponse{Modified: false}, nil
}

func main() {
	plugin := NewRequestEnricherPlugin()
	plugin_sdk.Serve(plugin)
}