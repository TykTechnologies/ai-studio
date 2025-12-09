package proxy

import (
	"encoding/json"
	"fmt"
)

// MessageReconstructor rebuilds vendor-specific request bodies from normalized messages
type MessageReconstructor interface {
	// Reconstruct takes normalized messages and the original request body,
	// returning a new request body with modified messages
	Reconstruct(messages []map[string]interface{}, originalBody []byte) ([]byte, error)

	// VendorName returns the vendor this reconstructor handles
	VendorName() string
}

// MessageReconstructorRegistry manages reconstructors for different vendors
type MessageReconstructorRegistry struct {
	reconstructors map[string]MessageReconstructor
}

// NewMessageReconstructorRegistry creates a new registry
func NewMessageReconstructorRegistry() *MessageReconstructorRegistry {
	return &MessageReconstructorRegistry{
		reconstructors: make(map[string]MessageReconstructor),
	}
}

// Register adds a reconstructor to the registry
func (r *MessageReconstructorRegistry) Register(reconstructor MessageReconstructor) {
	r.reconstructors[reconstructor.VendorName()] = reconstructor
}

// Reconstruct uses the appropriate reconstructor for the vendor
func (r *MessageReconstructorRegistry) Reconstruct(vendor string, messages []map[string]interface{}, originalBody []byte) ([]byte, error) {
	reconstructor, ok := r.reconstructors[vendor]
	if !ok {
		return nil, fmt.Errorf("no message reconstructor for vendor: %s", vendor)
	}
	return reconstructor.Reconstruct(messages, originalBody)
}

// OpenAIMessageReconstructor rebuilds OpenAI-format requests
type OpenAIMessageReconstructor struct{}

func (r *OpenAIMessageReconstructor) VendorName() string {
	return "openai"
}

func (r *OpenAIMessageReconstructor) Reconstruct(messages []map[string]interface{}, originalBody []byte) ([]byte, error) {
	// Parse original request to preserve non-message fields
	var original map[string]interface{}
	if err := json.Unmarshal(originalBody, &original); err != nil {
		return nil, fmt.Errorf("failed to parse original request: %w", err)
	}

	// Convert normalized messages to OpenAI format
	oaiMessages := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		role, ok := msg["role"].(string)
		if !ok {
			continue
		}
		content, ok := msg["content"].(string)
		if !ok {
			continue
		}

		oaiMessages = append(oaiMessages, map[string]interface{}{
			"role":    role,
			"content": content,
		})
	}

	// Replace messages in original request
	original["messages"] = oaiMessages

	return json.Marshal(original)
}

// AnthropicMessageReconstructor rebuilds Anthropic-format requests
type AnthropicMessageReconstructor struct{}

func (r *AnthropicMessageReconstructor) VendorName() string {
	return "anthropic"
}

func (r *AnthropicMessageReconstructor) Reconstruct(messages []map[string]interface{}, originalBody []byte) ([]byte, error) {
	// Parse original request
	var original map[string]interface{}
	if err := json.Unmarshal(originalBody, &original); err != nil {
		return nil, fmt.Errorf("failed to parse original request: %w", err)
	}

	// Separate system messages from user/assistant messages
	var systemText string
	anthropicMessages := make([]map[string]interface{}, 0)

	for _, msg := range messages {
		role, ok := msg["role"].(string)
		if !ok {
			continue
		}
		content, ok := msg["content"].(string)
		if !ok {
			continue
		}

		if role == "system" {
			// Accumulate system messages
			if systemText != "" {
				systemText += "\n"
			}
			systemText += content
		} else {
			// Convert role names
			anthropicRole := role
			if role == "assistant" {
				anthropicRole = "assistant"
			} else if role == "user" {
				anthropicRole = "user"
			}

			anthropicMessages = append(anthropicMessages, map[string]interface{}{
				"role":    anthropicRole,
				"content": content,
			})
		}
	}

	// Update original request
	if systemText != "" {
		original["system"] = systemText
	} else {
		delete(original, "system")
	}
	original["messages"] = anthropicMessages

	return json.Marshal(original)
}

// GoogleAIMessageReconstructor rebuilds Google AI/Vertex-format requests
type GoogleAIMessageReconstructor struct{}

func (r *GoogleAIMessageReconstructor) VendorName() string {
	return "google_ai"
}

func (r *GoogleAIMessageReconstructor) Reconstruct(messages []map[string]interface{}, originalBody []byte) ([]byte, error) {
	// Parse original request
	var original map[string]interface{}
	if err := json.Unmarshal(originalBody, &original); err != nil {
		return nil, fmt.Errorf("failed to parse original request: %w", err)
	}

	// Separate system from user/model messages
	var systemText string
	contents := make([]map[string]interface{}, 0)

	for _, msg := range messages {
		role, ok := msg["role"].(string)
		if !ok {
			continue
		}
		content, ok := msg["content"].(string)
		if !ok {
			continue
		}

		if role == "system" {
			if systemText != "" {
				systemText += "\n"
			}
			systemText += content
		} else {
			// Convert role names
			googleRole := "user"
			if role == "assistant" {
				googleRole = "model"
			}

			contents = append(contents, map[string]interface{}{
				"role": googleRole,
				"parts": []map[string]interface{}{
					{"text": content},
				},
			})
		}
	}

	// Update original request
	if systemText != "" {
		original["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": systemText},
			},
		}
	} else {
		delete(original, "systemInstruction")
	}
	original["contents"] = contents

	return json.Marshal(original)
}

// VertexMessageReconstructor is an alias for GoogleAIMessageReconstructor
type VertexMessageReconstructor struct {
	GoogleAIMessageReconstructor
}

func (r *VertexMessageReconstructor) VendorName() string {
	return "vertex"
}

// OllamaMessageReconstructor uses OpenAI format
type OllamaMessageReconstructor struct {
	OpenAIMessageReconstructor
}

func (r *OllamaMessageReconstructor) VendorName() string {
	return "ollama"
}
