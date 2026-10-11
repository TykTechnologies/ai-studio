package proxy

// Request filters over decision-model bodies. Decision requests carry no
// messages array, so filters and guardrails see them through these formats:
// each question's instructions become a system message, in question order,
// and the evidence (OpenAI "input", System One "state") becomes the final
// user message. Scopes such as last_user therefore scan the evidence, which
// is the untrusted part of a decision request. Filters that rewrite messages
// have their edits written back into the same fields.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/decisions"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// Filter formats for decision bodies, keyed like vendor formats in the
// extractor and reconstructor registries.
const (
	requestFilterFormatOpenAIDecisions = "openai_decisions"
	requestFilterFormatSystemOne       = "systemone"
)

// decisionFilterFormat is the filter format of a request to a decision
// endpoint, or "" when the path is not one.
func decisionFilterFormat(path string) string {
	switch f, _ := decisions.FormatForPath(path); f {
	case decisions.FormatOpenAI:
		return requestFilterFormatOpenAIDecisions
	case decisions.FormatSystemOne:
		return requestFilterFormatSystemOne
	}
	return ""
}

// requestKindForPath is the request kind of an upstream path.
func requestKindForPath(path string) models.RequestKind {
	if decisions.IsDecisionPath(path) {
		return models.DecisionRequest
	}
	return models.GenerativeRequest
}

// decisionMessages is the normalised message view of a decision request.
func decisionMessages(req *decisions.Request) ([]llms.MessageContent, error) {
	evidence, _, err := decisions.InputText(req.Input)
	if err != nil {
		return nil, err
	}
	out := make([]llms.MessageContent, 0, len(req.Questions)+1)
	for _, q := range req.Questions {
		out = append(out, llms.MessageContent{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextPart(q.Instructions)},
		})
	}
	out = append(out, llms.MessageContent{
		Role:  llms.ChatMessageTypeHuman,
		Parts: []llms.ContentPart{llms.TextPart(evidence)},
	})
	return out, nil
}

// OpenAIDecisionsMessageExtractor reads OpenAI Decisions requests.
type OpenAIDecisionsMessageExtractor struct{}

func (e *OpenAIDecisionsMessageExtractor) VendorName() string {
	return requestFilterFormatOpenAIDecisions
}

func (e *OpenAIDecisionsMessageExtractor) ExtractMessages(r *http.Request, body []byte) ([]llms.MessageContent, error) {
	req, err := decisions.ParseRequest(body, decisions.FormatOpenAI)
	if err != nil {
		return nil, fmt.Errorf("invalid decision request: %w", err)
	}
	return decisionMessages(req)
}

// SystemOneMessageExtractor reads System One requests.
type SystemOneMessageExtractor struct{}

func (e *SystemOneMessageExtractor) VendorName() string {
	return requestFilterFormatSystemOne
}

func (e *SystemOneMessageExtractor) ExtractMessages(r *http.Request, body []byte) ([]llms.MessageContent, error) {
	req, err := decisions.ParseRequest(body, decisions.FormatSystemOne)
	if err != nil {
		return nil, fmt.Errorf("invalid decision request: %w", err)
	}
	return decisionMessages(req)
}

// splitDecisionMessages separates filter output messages into the question
// instructions (system messages, in order) and the evidence (the last user
// message). ok is false when the output carries no user message.
func splitDecisionMessages(messages []map[string]interface{}) (instructions []string, evidence string, ok bool) {
	for _, m := range messages {
		role, _ := m["role"].(string)
		content, isText := m["content"].(string)
		if !isText {
			continue
		}
		switch role {
		case "system":
			instructions = append(instructions, content)
		case "user", "human":
			evidence, ok = content, true
		}
	}
	return instructions, evidence, ok
}

// OpenAIDecisionsMessageReconstructor writes filter edits back into an
// OpenAI Decisions request.
type OpenAIDecisionsMessageReconstructor struct{}

func (r *OpenAIDecisionsMessageReconstructor) VendorName() string {
	return requestFilterFormatOpenAIDecisions
}

func (r *OpenAIDecisionsMessageReconstructor) Reconstruct(messages []map[string]interface{}, originalBody []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(originalBody, &fields); err != nil {
		return nil, fmt.Errorf("failed to parse original request: %w", err)
	}
	instructions, evidence, ok := splitDecisionMessages(messages)
	if ok {
		input, err := rewriteInputText(fields["input"], evidence)
		if err != nil {
			return nil, err
		}
		fields["input"] = input
	}
	if raw, has := fields["questions"]; has {
		var questions []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &questions); err == nil && len(questions) == len(instructions) {
			for i := range questions {
				questions[i]["instructions"], _ = json.Marshal(instructions[i])
			}
			fields["questions"], _ = json.Marshal(questions)
		}
	}
	return json.Marshal(fields)
}

// rewriteInputText replaces the text of an OpenAI Decisions input. A string
// input is replaced outright; a message input keeps its image parts and
// carries the new text in its first text part.
func rewriteInputText(input json.RawMessage, text string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return json.Marshal(text)
	}
	if current, _, err := decisions.InputText(trimmed); err == nil && current == text {
		return input, nil
	}
	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &msgs); err != nil {
		return nil, fmt.Errorf("failed to parse input messages: %w", err)
	}
	placed := false
	for i, m := range msgs {
		content := bytes.TrimSpace(m["content"])
		if len(content) > 0 && content[0] == '"' {
			if placed {
				msgs[i]["content"], _ = json.Marshal("")
			} else {
				msgs[i]["content"], _ = json.Marshal(text)
				placed = true
			}
			continue
		}
		var parts []map[string]json.RawMessage
		if json.Unmarshal(content, &parts) != nil {
			continue
		}
		kept := parts[:0]
		for _, part := range parts {
			var typ string
			_ = json.Unmarshal(part["type"], &typ)
			if typ != "input_text" && typ != "text" {
				kept = append(kept, part)
				continue
			}
			if placed {
				continue
			}
			part["text"], _ = json.Marshal(text)
			kept = append(kept, part)
			placed = true
		}
		msgs[i]["content"], _ = json.Marshal(kept)
	}
	if !placed && len(msgs) > 0 {
		textPart := map[string]string{"type": "input_text", "text": text}
		var parts []any
		_ = json.Unmarshal(msgs[0]["content"], &parts)
		msgs[0]["content"], _ = json.Marshal(append([]any{textPart}, parts...))
	}
	return json.Marshal(msgs)
}

// SystemOneMessageReconstructor writes filter edits back into a System One
// request. Edited state is written as a string: a redacted object or array
// cannot keep its structure.
type SystemOneMessageReconstructor struct{}

func (r *SystemOneMessageReconstructor) VendorName() string {
	return requestFilterFormatSystemOne
}

func (r *SystemOneMessageReconstructor) Reconstruct(messages []map[string]interface{}, originalBody []byte) ([]byte, error) {
	var so decisions.SystemOneRequest
	if err := json.Unmarshal(originalBody, &so); err != nil {
		return nil, fmt.Errorf("failed to parse original request: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(originalBody, &fields); err != nil {
		return nil, fmt.Errorf("failed to parse original request: %w", err)
	}
	instructions, evidence, ok := splitDecisionMessages(messages)
	if ok && evidence != decisions.StateText(so.State) {
		so.State, _ = json.Marshal(evidence)
	}
	if len(instructions) == len(so.Questions) {
		for i := range so.Questions {
			// Untouched instructions keep their original JSON shape.
			if instructions[i] != decisions.StateText(so.Questions[i].Question.Instructions) {
				so.Questions[i].Question.Instructions, _ = json.Marshal(instructions[i])
			}
		}
	}
	rebuilt, err := json.Marshal(so)
	if err != nil {
		return nil, err
	}
	// Keep any top-level fields the System One type does not model.
	var out map[string]json.RawMessage
	if err := json.Unmarshal(rebuilt, &out); err != nil {
		return nil, err
	}
	for k, v := range fields {
		if _, known := out[k]; !known {
			out[k] = v
		}
	}
	return json.Marshal(out)
}
