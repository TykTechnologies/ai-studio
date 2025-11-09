// plugins/examples/response_modifier/main.go
package main

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

//go:embed manifest.json
var manifestBytes []byte

const (
	PluginName    = "response-modifier"
	PluginVersion = "1.0.0"
)

// ResponseModifierPlugin modifies LLM responses
type ResponseModifierPlugin struct {
	plugin_sdk.BasePlugin
	restModifier   string
	streamModifier string
}

// NewResponseModifierPlugin creates a new response modifier plugin
func NewResponseModifierPlugin() *ResponseModifierPlugin {
	return &ResponseModifierPlugin{
		BasePlugin:     plugin_sdk.NewBasePlugin(PluginName, PluginVersion, "Response Modifier"),
		restModifier:   "REST RESPONSE MOD",
		streamModifier: "STREAM RESPONSE MOD",
	}
}

// Initialize implements plugin_sdk.Plugin
func (p *ResponseModifierPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	if restMod, ok := config["rest_modifier"]; ok && restMod != "" {
		p.restModifier = restMod
	}

	if streamMod, ok := config["stream_modifier"]; ok && streamMod != "" {
		p.streamModifier = streamMod
	}

	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *ResponseModifierPlugin) Shutdown(ctx plugin_sdk.Context) error {
	return nil
}

// GetManifest implements plugin_sdk.ManifestProvider
func (p *ResponseModifierPlugin) GetManifest() ([]byte, error) {
	return manifestBytes, nil
}

// GetConfigSchema implements plugin_sdk.ConfigProvider
func (p *ResponseModifierPlugin) GetConfigSchema() ([]byte, error) {
	schema := map[string]interface{}{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"type":        "object",
		"title":       "Response Modifier Plugin Configuration",
		"description": "Configuration for the response modifier plugin",
		"properties": map[string]interface{}{
			"rest_modifier": map[string]interface{}{
				"type":        "string",
				"title":       "REST Response Modifier",
				"description": "Text to append to REST (non-streaming) responses",
				"default":     "REST RESPONSE MOD",
			},
			"stream_modifier": map[string]interface{}{
				"type":        "string",
				"title":       "Stream Response Modifier",
				"description": "Text to append to streaming response chunks",
				"default":     "STREAM RESPONSE MOD",
			},
		},
	}
	return json.Marshal(schema)
}

// OnBeforeWriteHeaders implements plugin_sdk.ResponseHandler
func (p *ResponseModifierPlugin) OnBeforeWriteHeaders(ctx plugin_sdk.Context, req *pb.HeadersRequest) (*pb.HeadersResponse, error) {
	// Add custom response headers
	modifiedHeaders := make(map[string]string)
	
	// Copy original headers
	for key, value := range req.Headers {
		modifiedHeaders[key] = value
	}
	
	// Add plugin-specific headers
	modifiedHeaders["X-Plugin-Modified"] = "response-modifier"
	modifiedHeaders["X-Modification-Type"] = "headers"

	return &pb.HeadersResponse{
		Modified: true,
		Headers:  modifiedHeaders,
	}, nil
}

// OnBeforeWrite implements plugin_sdk.ResponseHandler
func (p *ResponseModifierPlugin) OnBeforeWrite(ctx plugin_sdk.Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
	// Choose modifier based on stream chunk vs complete response
	modifier := p.restModifier
	if req.IsStreamChunk {
		modifier = p.streamModifier
	}

	// Parse the JSON response/chunk
	var responseBody map[string]interface{}
	if err := json.Unmarshal(req.Body, &responseBody); err != nil {
		// If we can't parse JSON, don't modify
		return &pb.ResponseWriteResponse{
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
							return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
						}
						
						return &pb.ResponseWriteResponse{
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
					return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
				}
				
				return &pb.ResponseWriteResponse{
					Modified: true,
					Body:     modifiedData,
					Headers:  req.Headers,
				}, nil
			}
		}
	}

	// No modification needed
	return &pb.ResponseWriteResponse{
		Modified: false,
		Body:     req.Body,
		Headers:  req.Headers,
	}, nil
}

func main() {
	plugin := NewResponseModifierPlugin()
	plugin_sdk.Serve(plugin)
}