package probe

import (
	"encoding/json"
	"strings"
)

// BodySpec describes the request body a cell sends.
type BodySpec struct {
	Format      Format `yaml:"format" json:"format"`
	Stream      bool   `yaml:"stream" json:"stream"`
	Model       string `yaml:"model" json:"model"`
	MaxTokens   int    `yaml:"max_tokens" json:"max_tokens"`
	PromptBytes int    `yaml:"prompt_bytes" json:"prompt_bytes"`
	// Prompt overrides the generated prompt. Real-upstream cells use a prompt
	// whose answer reliably runs to max_tokens, so output length (and with it
	// total time) is the same in both arms.
	Prompt string `yaml:"prompt" json:"prompt,omitempty"`
	// Extra is merged into the top level of the body last. A null value
	// deletes a key, e.g. {max_tokens: null, max_completion_tokens: 512,
	// reasoning_effort: none} for OpenAI reasoning models.
	Extra map[string]any `yaml:"extra" json:"extra,omitempty"`
}

// DefaultCountingPrompt produces long, predictable output: the reply runs to
// max_tokens on every vendor, so output length is the same in both arms.
const DefaultCountingPrompt = "Count from 1 to 2000 in words, separated by commas. Output only the numbers."

// Build renders the body once; the same bytes are sent for every request.
func (b BodySpec) Build() Request {
	maxTokens := b.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 64
	}
	prompt := b.Prompt
	if prompt == "" {
		prompt = filler(b.PromptBytes)
	}
	messages := []map[string]string{{"role": "user", "content": prompt}}
	// No sampling parameters: current models reject temperature, and output
	// length, not content, is what must match between arms.
	body := map[string]any{
		"model":      b.Model,
		"messages":   messages,
		"max_tokens": maxTokens,
	}
	if b.Stream {
		body["stream"] = true
		if b.Format != FormatAnthropic {
			// OpenAI only reports usage on a stream when asked; the gateway
			// needs it for analytics and budgets, as a real client would.
			body["stream_options"] = map[string]bool{"include_usage": true}
		}
	}
	for k, v := range b.Extra {
		if v == nil {
			delete(body, k)
		} else {
			body[k] = v
		}
	}
	raw, _ := json.Marshal(body)
	return Request{Format: b.Format, Stream: b.Stream, Body: raw}
}

// filler returns a prompt of about n bytes (minimum a short instruction).
func filler(n int) string {
	const head = "Reply with OK. Context follows. "
	if n <= len(head) {
		return head
	}
	const word = "lorem ipsum dolor sit amet consectetur "
	var sb strings.Builder
	sb.Grow(n)
	sb.WriteString(head)
	for sb.Len() < n {
		sb.WriteString(word)
	}
	return sb.String()[:n]
}
