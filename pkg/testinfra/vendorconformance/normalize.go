package vendorconformance

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Envelope is the wire-format-neutral shape of an assistant turn.
//
// It deliberately carries no content text. LLMs are non-deterministic, so
// comparing text across surfaces would be flaky theatre; comparing *shape* is
// what actually catches translation bugs. A translator that drops usage on the
// universal path but not the shim path fails an Envelope comparison even though
// both responses are individually schema-valid.
type Envelope struct {
	Role          string   `json:"role"`
	ContentKind   string   `json:"content_kind"` // "text" | "tool_calls" | "mixed" | "empty"
	ToolCallNames []string `json:"tool_call_names,omitempty"`
	FinishReason  string   `json:"finish_reason"`
	Model         string   `json:"model"`
	HasUsage      bool     `json:"has_usage"`
	Usage         Usage    `json:"usage"`
	// HasText records whether any non-empty text was produced, without
	// recording what it was.
	HasText bool `json:"has_text"`
}

// Usage is the token triple, normalized across vendors' different names.
type Usage struct {
	Prompt     int `json:"prompt_tokens"`
	Completion int `json:"completion_tokens"`
	Total      int `json:"total_tokens"`
}

// Consistent reports whether the counters add up. Vendors that report a total
// which is not the sum (because it folds in reasoning or cache tokens) are the
// reason this is a reported condition rather than a schema constraint.
func (u Usage) Consistent() bool {
	return u.Prompt+u.Completion == u.Total
}

func (u Usage) String() string {
	return fmt.Sprintf("prompt=%d completion=%d total=%d", u.Prompt, u.Completion, u.Total)
}

// content kinds
const (
	ContentText      = "text"
	ContentToolCalls = "tool_calls"
	ContentMixed     = "mixed"
	ContentEmpty     = "empty"
)

// Normalize converts a raw response in the given wire format into an Envelope.
func Normalize(f Format, payload []byte) (Envelope, error) {
	switch f {
	case FormatOpenAICompletion:
		return normalizeOpenAI(payload)
	case FormatAnthropicMessage:
		return normalizeAnthropic(payload)
	case FormatGeminiGenerate:
		return normalizeGemini(payload)
	default:
		return Envelope{}, fmt.Errorf("cannot normalize format %q", f)
	}
}

type openAIResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role string `json:"role"`
			// Content is deliberately absent: it is a string for most vendors
			// but an array of parts for some OpenAI-compatible providers, and a
			// decode error here would mask the schema violation the validator
			// already reports. It is read from `raw` instead.
			ToolCalls []struct {
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		Prompt     int `json:"prompt_tokens"`
		Completion int `json:"completion_tokens"`
		Total      int `json:"total_tokens"`
	} `json:"usage"`
}

func normalizeOpenAI(payload []byte) (Envelope, error) {
	// Content is decoded separately: it is a string for text responses but an
	// array of parts for some OpenAI-compatible providers, and a hard decode
	// error here would mask the schema violation the validator already reports.
	var raw struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	_ = json.Unmarshal(payload, &raw)

	var r openAIResponse
	if err := json.Unmarshal(payload, &r); err != nil {
		return Envelope{}, fmt.Errorf("decoding OpenAI response: %w", err)
	}
	if len(r.Choices) == 0 {
		return Envelope{}, fmt.Errorf("OpenAI response has no choices")
	}

	choice := r.Choices[0]
	env := Envelope{
		Role:         choice.Message.Role,
		FinishReason: choice.FinishReason,
		Model:        r.Model,
	}
	for _, tc := range choice.Message.ToolCalls {
		env.ToolCallNames = append(env.ToolCallNames, tc.Function.Name)
	}
	if len(raw.Choices) > 0 {
		env.HasText = rawTextNonEmpty(raw.Choices[0].Message.Content)
	}
	if r.Usage != nil {
		env.HasUsage = true
		env.Usage = Usage{r.Usage.Prompt, r.Usage.Completion, r.Usage.Total}
	}
	env.ContentKind = classify(env.HasText, len(env.ToolCallNames) > 0)
	sort.Strings(env.ToolCallNames)
	return env, nil
}

type anthropicResponse struct {
	Model      string `json:"model"`
	Role       string `json:"role"`
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
		Name string `json:"name"`
	} `json:"content"`
	Usage *struct {
		Input  int `json:"input_tokens"`
		Output int `json:"output_tokens"`
	} `json:"usage"`
}

func normalizeAnthropic(payload []byte) (Envelope, error) {
	var r anthropicResponse
	if err := json.Unmarshal(payload, &r); err != nil {
		return Envelope{}, fmt.Errorf("decoding Anthropic response: %w", err)
	}

	env := Envelope{
		Role:  r.Role,
		Model: r.Model,
		// Mapped to OpenAI vocabulary so envelopes are comparable across
		// surfaces. This mapping is itself part of the contract under test.
		FinishReason: anthropicStopReason(r.StopReason),
	}
	for _, block := range r.Content {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				env.HasText = true
			}
		case "tool_use":
			env.ToolCallNames = append(env.ToolCallNames, block.Name)
		}
	}
	if r.Usage != nil {
		env.HasUsage = true
		env.Usage = Usage{
			Prompt:     r.Usage.Input,
			Completion: r.Usage.Output,
			Total:      r.Usage.Input + r.Usage.Output,
		}
	}
	env.ContentKind = classify(env.HasText, len(env.ToolCallNames) > 0)
	sort.Strings(env.ToolCallNames)
	return env, nil
}

// anthropicStopReason maps Anthropic stop reasons onto the OpenAI finish_reason
// vocabulary. This is exactly what proxy/translator.go must do, so a divergence
// between this mapping and the shim's output is a finding, not a test bug.
func anthropicStopReason(s string) string {
	switch s {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "refusal":
		return "content_filter"
	case "":
		return ""
	default:
		return s
	}
}

type geminiResponse struct {
	ModelVersion string `json:"modelVersion"`
	Candidates   []struct {
		FinishReason string `json:"finishReason"`
		Content      struct {
			Role  string `json:"role"`
			Parts []struct {
				Text         string `json:"text"`
				FunctionCall *struct {
					Name string `json:"name"`
				} `json:"functionCall"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata *struct {
		Prompt     int `json:"promptTokenCount"`
		Candidates int `json:"candidatesTokenCount"`
		Total      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func normalizeGemini(payload []byte) (Envelope, error) {
	var r geminiResponse
	if err := json.Unmarshal(payload, &r); err != nil {
		return Envelope{}, fmt.Errorf("decoding Gemini response: %w", err)
	}
	if len(r.Candidates) == 0 {
		return Envelope{}, fmt.Errorf("Gemini response has no candidates")
	}

	c := r.Candidates[0]
	env := Envelope{
		Role:         normalizeRole(c.Content.Role),
		Model:        r.ModelVersion,
		FinishReason: geminiFinishReason(c.FinishReason),
	}
	for _, p := range c.Content.Parts {
		if strings.TrimSpace(p.Text) != "" {
			env.HasText = true
		}
		if p.FunctionCall != nil {
			env.ToolCallNames = append(env.ToolCallNames, p.FunctionCall.Name)
		}
	}
	if r.UsageMetadata != nil {
		env.HasUsage = true
		env.Usage = Usage{
			Prompt:     r.UsageMetadata.Prompt,
			Completion: r.UsageMetadata.Candidates,
			Total:      r.UsageMetadata.Total,
		}
	}
	env.ContentKind = classify(env.HasText, len(env.ToolCallNames) > 0)
	sort.Strings(env.ToolCallNames)
	return env, nil
}

func geminiFinishReason(s string) string {
	switch s {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "PROHIBITED_CONTENT", "BLOCKLIST", "SPII", "IMAGE_SAFETY":
		return "content_filter"
	case "":
		return ""
	default:
		return strings.ToLower(s)
	}
}

// normalizeRole maps a vendor's name for the assistant onto OpenAI's.
func normalizeRole(role string) string {
	if role == "model" {
		return "assistant"
	}
	if role == "" {
		return "assistant"
	}
	return role
}

func classify(hasText, hasToolCalls bool) string {
	switch {
	case hasText && hasToolCalls:
		return ContentMixed
	case hasToolCalls:
		return ContentToolCalls
	case hasText:
		return ContentText
	default:
		return ContentEmpty
	}
}

func rawTextNonEmpty(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s) != ""
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		for _, p := range parts {
			if strings.TrimSpace(p.Text) != "" {
				return true
			}
		}
	}
	return false
}

// Diff compares two envelopes on the fields that must agree across surfaces and
// returns a human-readable list of mismatches. Model is compared loosely: a
// vendor may echo back a fully-qualified version of the requested ID.
func (e Envelope) Diff(other Envelope, label, otherLabel string) []string {
	var out []string
	add := func(field, a, b string) {
		out = append(out, fmt.Sprintf("%s: %s=%s %s=%s", field, label, a, otherLabel, b))
	}

	if e.Role != other.Role {
		add("role", e.Role, other.Role)
	}
	if e.ContentKind != other.ContentKind {
		add("content_kind", e.ContentKind, other.ContentKind)
	}
	if e.FinishReason != other.FinishReason {
		add("finish_reason", e.FinishReason, other.FinishReason)
	}
	if e.HasUsage != other.HasUsage {
		add("has_usage", fmt.Sprint(e.HasUsage), fmt.Sprint(other.HasUsage))
	}
	if strings.Join(e.ToolCallNames, ",") != strings.Join(other.ToolCallNames, ",") {
		add("tool_call_names", strings.Join(e.ToolCallNames, ","), strings.Join(other.ToolCallNames, ","))
	}
	if !modelsAgree(e.Model, other.Model) {
		add("model", e.Model, other.Model)
	}
	return out
}

// modelsAgree accepts one model string being a prefix or suffix of the other,
// which covers vendors that qualify the requested ID with a version or region.
func modelsAgree(a, b string) bool {
	if a == b {
		return true
	}
	if a == "" || b == "" {
		return false
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}
