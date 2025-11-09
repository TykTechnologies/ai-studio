// plugins/examples/message_modifier/main.go
package main

import (
	"encoding/json"
	_ "embed"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

//go:embed manifest.json
var manifestBytes []byte

const (
	PluginName    = "message-modifier"
	PluginVersion = "1.0.0"
)

// MessageModifierPlugin modifies outbound LLM requests to add instructions
type MessageModifierPlugin struct {
	plugin_sdk.BasePlugin
	instruction string
}

// NewMessageModifierPlugin creates a new message modifier plugin
func NewMessageModifierPlugin() *MessageModifierPlugin {
	return &MessageModifierPlugin{
		BasePlugin:  plugin_sdk.NewBasePlugin(PluginName, PluginVersion, "Message Modifier"),
		instruction: "Say Moo! at the end of your response",
	}
}

// Initialize implements plugin_sdk.Plugin
func (p *MessageModifierPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	if instruction, ok := config["instruction"]; ok && instruction != "" {
		p.instruction = instruction
	}
	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *MessageModifierPlugin) Shutdown(ctx plugin_sdk.Context) error {
	return nil
}

// GetManifest implements ManifestProvider
func (p *MessageModifierPlugin) GetManifest() ([]byte, error) {
	return manifestBytes, nil
}

// GetConfigSchema implements ConfigSchemaProvider
func (p *MessageModifierPlugin) GetConfigSchema() ([]byte, error) {
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object",
		"title":   "Message Modifier Plugin Configuration",
		"description": "Configuration for the message modifier plugin that adds instructions to user messages",
		"properties": map[string]interface{}{
			"instruction": map[string]interface{}{
				"type":        "string",
				"title":       "Instruction Text",
				"description": "Text to append to the last user message in the conversation",
				"default":     "Say Moo! at the end of your response",
				"examples":    []string{
					"Say Moo! at the end of your response",
					"Please respond in a friendly and helpful tone",
					"Add a summary at the end of your response",
					"Include relevant examples in your answer",
				},
				"minLength": 1,
				"maxLength": 500,
			},
		},
		"required": []string{"instruction"},
		"additionalProperties": false,
	}

	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to generate config schema: %w", err)
	}

	return schemaBytes, nil
}

// HandlePreAuth implements plugin_sdk.PreAuthHandler
func (p *MessageModifierPlugin) HandlePreAuth(ctx plugin_sdk.Context, req *pb.PluginRequest) (*pb.PluginResponse, error) {
	// Only modify POST requests to LLM endpoints
	if req.Method != "POST" {
		return &pb.PluginResponse{Modified: false}, nil
	}

	// Parse the JSON body
	var requestBody map[string]interface{}
	if err := json.Unmarshal(req.Body, &requestBody); err != nil {
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

	// Add instruction to the last user message
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
				messageMap["content"] = content + "\n\n" + p.instruction

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
	plugin := NewMessageModifierPlugin()
	plugin_sdk.Serve(plugin)
}