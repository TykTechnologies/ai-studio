package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

// MessageExtractor converts vendor-specific request formats to langchaingo MessageContent
type MessageExtractor interface {
	// ExtractMessages parses the request body and returns normalized messages
	ExtractMessages(r *http.Request, body []byte) ([]llms.MessageContent, error)

	// VendorName returns the vendor this extractor handles
	VendorName() string
}

// MessageExtractorRegistry manages extractors for different vendors
type MessageExtractorRegistry struct {
	extractors map[string]MessageExtractor
}

// NewMessageExtractorRegistry creates a new registry for message extractors
func NewMessageExtractorRegistry() *MessageExtractorRegistry {
	return &MessageExtractorRegistry{
		extractors: make(map[string]MessageExtractor),
	}
}

// Register adds a message extractor to the registry
func (r *MessageExtractorRegistry) Register(extractor MessageExtractor) {
	r.extractors[strings.ToLower(extractor.VendorName())] = extractor
}

// Extract uses the appropriate extractor to parse messages from the request
func (r *MessageExtractorRegistry) Extract(vendor string, req *http.Request, body []byte) ([]llms.MessageContent, error) {
	extractor, ok := r.extractors[strings.ToLower(vendor)]
	if !ok {
		return nil, fmt.Errorf("no message extractor for vendor: %s", vendor)
	}
	return extractor.ExtractMessages(req, body)
}

// OpenAIMessageExtractor extracts messages from OpenAI-format requests
type OpenAIMessageExtractor struct{}

// VendorName returns "openai"
func (e *OpenAIMessageExtractor) VendorName() string {
	return "openai"
}

// ExtractMessages wraps the existing ChatCompletionRequest.GetMessages() method
func (e *OpenAIMessageExtractor) ExtractMessages(r *http.Request, body []byte) ([]llms.MessageContent, error) {
	req := &ChatCompletionRequest{}
	if err := json.Unmarshal(body, req); err != nil {
		return nil, fmt.Errorf("invalid OpenAI request: %w", err)
	}
	// Reuse existing GetMessages() from translator_models.go
	return req.GetMessages(), nil
}

// AnthropicMessageExtractor extracts messages from Anthropic-format requests
type AnthropicMessageExtractor struct{}

// VendorName returns "anthropic"
func (e *AnthropicMessageExtractor) VendorName() string {
	return "anthropic"
}

// ExtractMessages parses Anthropic's format (separate system field + messages array)
func (e *AnthropicMessageExtractor) ExtractMessages(r *http.Request, body []byte) ([]llms.MessageContent, error) {
	var req struct {
		System   string `json:"system"`
		Messages []struct {
			Role    string      `json:"role"`
			Content interface{} `json:"content"` // string or array of content blocks
		} `json:"messages"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid Anthropic request: %w", err)
	}

	messages := []llms.MessageContent{}

	// Add system message first if present
	if req.System != "" {
		messages = append(messages, llms.MessageContent{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextPart(req.System)},
		})
	}

	// Convert messages array
	for _, msg := range req.Messages {
		content := e.extractContent(msg.Content)
		role := e.convertRole(msg.Role)

		messages = append(messages, llms.MessageContent{
			Role:  role,
			Parts: []llms.ContentPart{llms.TextPart(content)},
		})
	}

	return messages, nil
}

// extractContent handles both string and array content formats
func (e *AnthropicMessageExtractor) extractContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		text := ""
		for _, block := range v {
			if blockMap, ok := block.(map[string]interface{}); ok {
				if blockType, ok := blockMap["type"].(string); ok && blockType == "text" {
					if t, ok := blockMap["text"].(string); ok {
						text += t
					}
				}
			}
		}
		return text
	default:
		return ""
	}
}

// convertRole maps Anthropic roles to langchaingo types
func (e *AnthropicMessageExtractor) convertRole(role string) llms.ChatMessageType {
	switch role {
	case "user":
		return llms.ChatMessageTypeHuman
	case "assistant":
		return llms.ChatMessageTypeAI
	default:
		return llms.ChatMessageTypeHuman
	}
}

// GoogleAIMessageExtractor extracts messages from Google AI/Vertex format requests
type GoogleAIMessageExtractor struct{}

// VendorName returns "google_ai"
func (e *GoogleAIMessageExtractor) VendorName() string {
	return "google_ai"
}

// ExtractMessages parses Google AI's contents/parts structure
func (e *GoogleAIMessageExtractor) ExtractMessages(r *http.Request, body []byte) ([]llms.MessageContent, error) {
	var req struct {
		SystemInstruction struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"systemInstruction"`
		Contents []struct {
			Role  string `json:"role"` // "user" or "model"
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid Google AI request: %w", err)
	}

	messages := []llms.MessageContent{}

	// Add system instruction if present
	if len(req.SystemInstruction.Parts) > 0 {
		systemText := ""
		for _, part := range req.SystemInstruction.Parts {
			systemText += part.Text
		}
		if systemText != "" {
			messages = append(messages, llms.MessageContent{
				Role:  llms.ChatMessageTypeSystem,
				Parts: []llms.ContentPart{llms.TextPart(systemText)},
			})
		}
	}

	// Convert contents
	for _, content := range req.Contents {
		text := ""
		for _, part := range content.Parts {
			text += part.Text
		}

		role := e.convertRole(content.Role)
		messages = append(messages, llms.MessageContent{
			Role:  role,
			Parts: []llms.ContentPart{llms.TextPart(text)},
		})
	}

	return messages, nil
}

// convertRole normalizes Google AI roles ("model" → ChatMessageTypeAI)
func (e *GoogleAIMessageExtractor) convertRole(role string) llms.ChatMessageType {
	if role == "model" {
		return llms.ChatMessageTypeAI
	}
	return llms.ChatMessageTypeHuman
}

// VertexMessageExtractor is an alias for GoogleAIMessageExtractor (same format)
type VertexMessageExtractor struct {
	GoogleAIMessageExtractor
}

// VendorName returns "vertex"
func (e *VertexMessageExtractor) VendorName() string {
	return "vertex"
}

// OllamaMessageExtractor uses OpenAI format
type OllamaMessageExtractor struct {
	OpenAIMessageExtractor
}

// VendorName returns "ollama"
func (e *OllamaMessageExtractor) VendorName() string {
	return "ollama"
}
