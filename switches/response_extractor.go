package switches

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/responses"
)

// ExtractResponseText extracts the assistant/model message content from a vendor-specific response body
// This leverages the existing vendor parsers for consistency
// Returns the response text that would be sent to the user
func ExtractResponseText(vendor models.Vendor, responseBody []byte) (string, error) {
	switch vendor {
	case models.OPENAI, models.OLLAMA:
		var resp responses.OpenAIResponse
		if err := json.Unmarshal(responseBody, &resp); err != nil {
			return "", fmt.Errorf("failed to parse OpenAI response: %w", err)
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("no choices in OpenAI response")
		}
		return resp.Choices[0].Message.Content, nil

	case models.ANTHROPIC:
		var resp responses.AnthropicResponse
		if err := json.Unmarshal(responseBody, &resp); err != nil {
			return "", fmt.Errorf("failed to parse Anthropic response: %w", err)
		}
		// Concatenate all text content blocks
		var fullText string
		for _, content := range resp.Content {
			if content.GetType() == "text" {
				// Type assert to TextContent to access Text field
				if textContent, ok := content.(*responses.TextContent); ok {
					fullText += textContent.Text
				}
			}
		}
		if fullText == "" {
			return "", fmt.Errorf("no text content in Anthropic response")
		}
		return fullText, nil

	case models.GOOGLEAI, models.VERTEX:
		var resp responses.GoogleAIChatResponse
		if err := json.Unmarshal(responseBody, &resp); err != nil {
			return "", fmt.Errorf("failed to parse Google AI response: %w", err)
		}
		if len(resp.Candidates) == 0 {
			return "", fmt.Errorf("no candidates in Google AI response")
		}
		// Concatenate all text parts from first candidate
		var fullText string
		for _, part := range resp.Candidates[0].Content.Parts {
			fullText += part.Text
		}
		if fullText == "" {
			return "", fmt.Errorf("no text content in Google AI response")
		}
		return fullText, nil

	default:
		// For unknown vendors, try OpenAI format as it's most common
		var resp responses.OpenAIResponse
		if err := json.Unmarshal(responseBody, &resp); err != nil {
			return "", fmt.Errorf("unsupported vendor for response extraction: %s", vendor)
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("no choices in response")
		}
		return resp.Choices[0].Message.Content, nil
	}
}

// ExtractStreamingChunkText extracts text from a streaming chunk
// Returns concatenated text from ALL SSE events in the chunk
// A single network read may contain multiple SSE events
func ExtractStreamingChunkText(vendor models.Vendor, chunk []byte) string {
	// Extract all SSE data payloads from the chunk
	dataPayloads := extractAllSSEDataPayloads(chunk)
	if len(dataPayloads) == 0 {
		return ""
	}

	// Extract text from each payload and concatenate
	var fullText strings.Builder
	for _, payload := range dataPayloads {
		var text string
		switch vendor {
		case models.OPENAI, models.OLLAMA:
			text = extractOpenAIChunkText(payload)
		case models.ANTHROPIC:
			text = extractAnthropicChunkText(payload)
		case models.GOOGLEAI, models.VERTEX:
			text = extractGoogleAIChunkText(payload)
		default:
			text = extractOpenAIChunkText(payload)
		}
		if text != "" {
			fullText.WriteString(text)
		}
	}

	return fullText.String()
}

// extractAllSSEDataPayloads extracts all "data: {...}" payloads from an SSE chunk
// A single chunk may contain multiple SSE events
func extractAllSSEDataPayloads(chunk []byte) [][]byte {
	chunkStr := string(chunk)
	lines := strings.Split(chunkStr, "\n")

	var payloads [][]byte
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			jsonData := strings.TrimPrefix(line, "data: ")
			jsonData = strings.TrimSpace(jsonData)

			// Skip [DONE] markers and empty data
			if jsonData != "" && jsonData != "[DONE]" {
				payloads = append(payloads, []byte(jsonData))
			}
		}
	}

	return payloads
}

// stripSSEFraming removes SSE framing from chunks
// Handles both simple format ("data: {...}") and event format ("event: ...\ndata: {...}")
func stripSSEFraming(chunk []byte) []byte {
	chunkStr := string(chunk)

	// Split by newlines to handle multi-line SSE events
	lines := strings.Split(chunkStr, "\n")

	// Find the line that starts with "data: "
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			jsonData := strings.TrimPrefix(line, "data: ")
			jsonData = strings.TrimSpace(jsonData)

			// Skip [DONE] markers and empty chunks
			if jsonData == "" || jsonData == "[DONE]" {
				return []byte{}
			}

			return []byte(jsonData)
		}
	}

	// No "data: " prefix found - might be raw JSON
	// Try to use as-is if it looks like JSON
	trimmed := strings.TrimSpace(chunkStr)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return []byte(trimmed)
	}

	return []byte{}
}

// extractOpenAIChunkText extracts text from OpenAI SSE chunk
func extractOpenAIChunkText(chunk []byte) string {
	var chunkData struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(chunk, &chunkData); err != nil {
		return ""
	}

	if len(chunkData.Choices) == 0 {
		return ""
	}

	return chunkData.Choices[0].Delta.Content
}

// extractAnthropicChunkText extracts text from Anthropic SSE chunk
func extractAnthropicChunkText(chunk []byte) string {
	var chunkData struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	}

	if err := json.Unmarshal(chunk, &chunkData); err != nil {
		return ""
	}

	// Anthropic sends different chunk types, we want content_block_delta
	if chunkData.Type == "content_block_delta" && chunkData.Delta.Type == "text_delta" {
		return chunkData.Delta.Text
	}

	return ""
}

// extractGoogleAIChunkText extracts text from Google AI SSE chunk
func extractGoogleAIChunkText(chunk []byte) string {
	var chunkData struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(chunk, &chunkData); err != nil {
		return ""
	}

	if len(chunkData.Candidates) == 0 {
		return ""
	}

	// Concatenate all text parts
	var fullText string
	for _, part := range chunkData.Candidates[0].Content.Parts {
		fullText += part.Text
	}

	return fullText
}
