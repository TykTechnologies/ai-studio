// plugins/examples/message_modifier/main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

// MessageModifierPlugin modifies outbound LLM requests to add instructions
type MessageModifierPlugin struct {
	instruction string
}

// Initialize implements BasePlugin
func (p *MessageModifierPlugin) Initialize(config map[string]interface{}) error {
	if instruction, ok := config["instruction"]; ok {
		p.instruction = instruction.(string)
	} else {
		p.instruction = "Say Moo! at the end of your response"
	}
	return nil
}

// GetHookType implements BasePlugin
func (p *MessageModifierPlugin) GetHookType() sdk.HookType {
	return sdk.HookTypePreAuth
}

// GetName implements BasePlugin
func (p *MessageModifierPlugin) GetName() string {
	return "message-modifier"
}

// GetVersion implements BasePlugin
func (p *MessageModifierPlugin) GetVersion() string {
	return "1.0.0"
}

// Shutdown implements BasePlugin
func (p *MessageModifierPlugin) Shutdown() error {
	return nil
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

// ProcessRequest implements PreAuthPlugin
func (p *MessageModifierPlugin) ProcessRequest(ctx context.Context, req *sdk.PluginRequest, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
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
	plugin := &MessageModifierPlugin{}
	sdk.ServePlugin(plugin)
}