// plugins/examples/response_modifier/main.go
package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

// ResponseModifierPlugin modifies LLM responses
type ResponseModifierPlugin struct {
	restModifier   string
	streamModifier string
}

// Initialize implements BasePlugin
func (p *ResponseModifierPlugin) Initialize(config map[string]interface{}) error {
	if restMod, ok := config["rest_modifier"]; ok {
		p.restModifier = restMod.(string)
	} else {
		p.restModifier = "REST RESPONSE MOD"
	}

	if streamMod, ok := config["stream_modifier"]; ok {
		p.streamModifier = streamMod.(string)
	} else {
		p.streamModifier = "STREAM RESPONSE MOD"
	}

	return nil
}

// GetHookType implements BasePlugin
func (p *ResponseModifierPlugin) GetHookType() sdk.HookType {
	return sdk.HookTypeOnResponse
}

// GetName implements BasePlugin
func (p *ResponseModifierPlugin) GetName() string {
	return "response-modifier"
}

// GetVersion implements BasePlugin
func (p *ResponseModifierPlugin) GetVersion() string {
	return "1.0.0"
}

// Shutdown implements BasePlugin
func (p *ResponseModifierPlugin) Shutdown() error {
	return nil
}

// OnBeforeWriteHeaders implements ResponsePlugin (new clean interface)
func (p *ResponseModifierPlugin) OnBeforeWriteHeaders(ctx context.Context, req *sdk.HeadersRequest, pluginCtx *sdk.PluginContext) (*sdk.HeadersResponse, error) {
	// Add custom response headers
	modifiedHeaders := make(map[string]string)
	
	// Copy original headers
	for key, value := range req.Headers {
		modifiedHeaders[key] = value
	}
	
	// Add plugin-specific headers
	modifiedHeaders["X-Plugin-Modified"] = "response-modifier"
	modifiedHeaders["X-Modification-Type"] = "headers"

	return &sdk.HeadersResponse{
		Modified: true,
		Headers:  modifiedHeaders,
	}, nil
}

// OnBeforeWrite implements ResponsePlugin (new clean interface)
func (p *ResponseModifierPlugin) OnBeforeWrite(ctx context.Context, req *sdk.ResponseWriteRequest, pluginCtx *sdk.PluginContext) (*sdk.ResponseWriteResponse, error) {
	// Choose modifier based on stream chunk vs complete response
	modifier := p.restModifier
	if req.IsStreamChunk {
		modifier = p.streamModifier
	}

	// Parse the JSON response/chunk
	var responseBody map[string]interface{}
	if err := json.Unmarshal(req.Body, &responseBody); err != nil {
		// If we can't parse JSON, don't modify
		return &sdk.ResponseWriteResponse{
			Modified: false,
			Body:     req.Body,
			Headers:  req.Headers,
		}, nil
	}

	// Handle Anthropic-style responses (which Claude uses)
	if content, hasContent := responseBody["content"].([]interface{}); hasContent && len(content) > 0 {
		// Modify the last text content item
		for i := len(content) - 1; i >= 0; i-- {
			if contentItem, ok := content[i].(map[string]interface{}); ok {
				if itemType, hasType := contentItem["type"].(string); hasType && itemType == "text" {
					if text, hasText := contentItem["text"].(string); hasText {
						contentItem["text"] = text + "\n\n" + modifier
						
						modifiedData, err := json.Marshal(responseBody)
						if err != nil {
							return &sdk.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
						}
						
						return &sdk.ResponseWriteResponse{
							Modified: true,
							Body:     modifiedData,
							Headers:  req.Headers,
						}, nil
					}
					break
				}
			}
		}
	}

	// Handle streaming chunks for Anthropic
	if req.IsStreamChunk {
		if delta, hasDelta := responseBody["delta"].(map[string]interface{}); hasDelta {
			if text, hasText := delta["text"].(string); hasText && strings.Contains(text, "\n") {
				// End of streaming response, add modifier
				delta["text"] = text + " " + modifier
				
				modifiedData, err := json.Marshal(responseBody)
				if err != nil {
					return &sdk.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
				}
				
				return &sdk.ResponseWriteResponse{
					Modified: true,
					Body:     modifiedData,
					Headers:  req.Headers,
				}, nil
			}
		}
	}

	// No modification needed
	return &sdk.ResponseWriteResponse{
		Modified: false,
		Body:     req.Body,
		Headers:  req.Headers,
	}, nil
}

func main() {
	plugin := &ResponseModifierPlugin{}
	sdk.ServePlugin(plugin)
}