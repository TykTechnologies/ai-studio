package proxy

// Request filters over OpenAI's prompt-and-input bodies: /v1/embeddings and
// /v1/moderations ("input") and /v1/completions ("prompt", "suffix"), and
// batch lines with those urls. They carry no messages array, so the Chat
// Completions extractor found nothing and the filters screened an empty
// prompt. Here each text is one user message, in body order: a string, each
// string of an array, or each text part of a moderation's multimodal input.
// A filter's rewrite goes back into the same position.
//
// Token arrays (integers) cannot be screened, so this extractor fails
// closed: with request filters attached, a body whose input it cannot read
// as text is refused.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/pkg/requestformat"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// requestFilterFormatOpenAIInput is the filter format of an embeddings,
// completions or moderations body.
const requestFilterFormatOpenAIInput = requestformat.OpenAIInput

// openAIInputFields are the prompt fields of these bodies, in the order
// their texts are listed.
var openAIInputFields = []string{"prompt", "suffix", "input"}

// openAIInputSlot is one prompt text and how to write a rewrite back.
type openAIInputSlot struct {
	role llms.ChatMessageType
	text string
	set  func(string)
}

// openAIInputSlots decodes body and lists its prompt texts.
func openAIInputSlots(body []byte) (map[string]any, []openAIInputSlot, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil || obj == nil {
		return nil, nil, errors.New("not a JSON object")
	}
	var slots []openAIInputSlot
	for _, field := range openAIInputFields {
		switch v := obj[field].(type) {
		case nil:
		case string:
			slots = append(slots, openAIInputSlot{role: llms.ChatMessageTypeHuman, text: v, set: func(s string) { obj[field] = s }})
		case []any:
			for i, el := range v {
				switch e := el.(type) {
				case string:
					slots = append(slots, openAIInputSlot{role: llms.ChatMessageTypeHuman, text: e, set: func(s string) { v[i] = s }})
				case map[string]any:
					// A moderation's multimodal input: text parts are read,
					// image parts are not text.
					switch e["type"] {
					case "text":
						t, ok := e["text"].(string)
						if !ok {
							return nil, nil, fmt.Errorf("%s[%d].text must be a string", field, i)
						}
						slots = append(slots, openAIInputSlot{role: llms.ChatMessageTypeHuman, text: t, set: func(s string) { e["text"] = s }})
					case "image_url":
					default:
						return nil, nil, fmt.Errorf("%s[%d] has an unsupported type", field, i)
					}
				default:
					return nil, nil, fmt.Errorf("%s[%d] is not text (token arrays cannot be screened)", field, i)
				}
			}
		default:
			return nil, nil, fmt.Errorf("%s is not text (token arrays cannot be screened)", field)
		}
	}
	return obj, slots, nil
}

// OpenAIInputMessageExtractor reads embeddings, completions and moderations
// requests.
type OpenAIInputMessageExtractor struct{}

func (e *OpenAIInputMessageExtractor) VendorName() string { return requestFilterFormatOpenAIInput }

// failClosed marks the extractor's errors as refusing the request.
func (e *OpenAIInputMessageExtractor) failClosed() bool { return true }

func (e *OpenAIInputMessageExtractor) ExtractMessages(_ *http.Request, body []byte) ([]llms.MessageContent, error) {
	_, slots, err := openAIInputSlots(body)
	if err != nil {
		return nil, fmt.Errorf("invalid OpenAI input request: %w", err)
	}
	out := make([]llms.MessageContent, len(slots))
	for i, s := range slots {
		out[i] = llms.MessageContent{Role: s.role, Parts: []llms.ContentPart{llms.TextPart(s.text)}}
	}
	return out, nil
}

// OpenAIInputMessageReconstructor writes filter edits back into the same
// positions; output that adds or drops messages fails the request.
type OpenAIInputMessageReconstructor struct{}

func (r *OpenAIInputMessageReconstructor) VendorName() string { return requestFilterFormatOpenAIInput }

func (r *OpenAIInputMessageReconstructor) Reconstruct(messages []map[string]interface{}, originalBody []byte) ([]byte, error) {
	obj, slots, err := openAIInputSlots(originalBody)
	if err != nil {
		return nil, fmt.Errorf("invalid OpenAI input request: %w", err)
	}
	if len(messages) != len(slots) {
		return nil, fmt.Errorf("a filter returned %d messages for a request with %d inputs; only edits in place can be written back", len(messages), len(slots))
	}
	for i, m := range messages {
		content, ok := m["content"].(string)
		if !ok {
			return nil, fmt.Errorf("message %d has no text content", i)
		}
		if content != slots[i].text {
			slots[i].set(content)
		}
	}
	return json.Marshal(obj)
}
