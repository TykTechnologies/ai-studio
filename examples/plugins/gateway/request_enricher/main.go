// plugins/examples/request_enricher/main.go
package main

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

//go:embed manifest.json
var manifestBytes []byte

// RequestEnricherPlugin adds additional instructions to authenticated requests
type RequestEnricherPlugin struct {
	additionalInstruction string
}

// Initialize implements BasePlugin
func (p *RequestEnricherPlugin) Initialize(config map[string]interface{}) error {
	if instruction, ok := config["additional_instruction"]; ok {
		p.additionalInstruction = instruction.(string)
	} else {
		p.additionalInstruction = "Also say 'I love sunsets!' to the end of the outbound message"
	}
	return nil
}

// GetHookType implements BasePlugin
func (p *RequestEnricherPlugin) GetHookType() sdk.HookType {
	return sdk.HookTypePostAuth
}

// GetName implements BasePlugin
func (p *RequestEnricherPlugin) GetName() string {
	return "request-enricher"
}

// GetVersion implements BasePlugin
func (p *RequestEnricherPlugin) GetVersion() string {
	return "1.0.0"
}

// Shutdown implements BasePlugin
func (p *RequestEnricherPlugin) Shutdown() error {
	return nil
}

// GetManifest implements ManifestProvider
func (p *RequestEnricherPlugin) GetManifest() ([]byte, error) {
	return manifestBytes, nil
}

// ProcessRequest implements PostAuthPlugin
func (p *RequestEnricherPlugin) ProcessRequest(ctx context.Context, req *sdk.EnrichedRequest, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
	// Only modify POST requests to LLM endpoints
	if req.Method != "POST" {
		return &sdk.PluginResponse{Modified: false}, nil
	}

	// Parse the JSON body
	var requestBody map[string]interface{}
	if err := json.Unmarshal(req.Body, &requestBody); err != nil {
		// If we can't parse JSON, don't modify
		return &sdk.PluginResponse{Modified: false}, nil
	}

	// Check if this is a chat completion request
	messages, hasMessages := requestBody["messages"]
	if !hasMessages {
		return &sdk.PluginResponse{Modified: false}, nil
	}

	// Convert messages to slice of maps
	messageSlice, ok := messages.([]interface{})
	if !ok {
		return &sdk.PluginResponse{Modified: false}, nil
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
					return &sdk.PluginResponse{Modified: false}, nil
				}

				return &sdk.PluginResponse{
					Modified: true,
					Headers:  map[string]string{"Content-Type": "application/json"},
					Body:     modifiedBody,
				}, nil
			}
			break
		}
	}

	// No modification needed
	return &sdk.PluginResponse{Modified: false}, nil
}

func main() {
	plugin := &RequestEnricherPlugin{}
	sdk.ServePlugin(plugin)
}