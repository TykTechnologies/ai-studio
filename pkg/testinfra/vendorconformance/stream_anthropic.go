package vendorconformance

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Anthropic's native stream is event-typed rather than a sequence of uniform
// chunks, so none of the OpenAI cross-chunk rules apply to it: there is no
// delta.role, no [DONE] sentinel, and no per-chunk finish_reason. Its contract
// is a state machine over event types, and that is what is checked here.
//
// This matters for the sdk surface specifically. Validating a native Anthropic
// stream against the OpenAI chunk schema produces a wall of violations for a
// response that is perfectly correct — which is worse than no check at all,
// because it trains people to ignore the suite.

// AnthropicStream is a fully-consumed native Anthropic SSE response.
type AnthropicStream struct {
	Events []AnthropicEvent

	Text         string
	ToolCalls    map[int]*StreamToolCall
	Model        string
	StopReason   string
	Usage        Usage
	HasUsage     bool
	SawStart     bool
	SawStop      bool
	EventsByType map[string]int
}

// AnthropicEvent is one decoded stream event, retained for schema validation.
type AnthropicEvent struct {
	Index int
	Type  string
	Raw   []byte
}

type anthropicEventPayload struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message *struct {
		Model string `json:"model"`
		Usage *struct {
			Input  int `json:"input_tokens"`
			Output int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage *struct {
		Input  int `json:"input_tokens"`
		Output int `json:"output_tokens"`
	} `json:"usage"`
}

// AsAnthropicStream reinterprets a parsed SSE body as a native Anthropic stream.
// ParseStream already did the framing; only the payload semantics differ.
func AsAnthropicStream(s *Stream) *AnthropicStream {
	out := &AnthropicStream{
		ToolCalls:    map[int]*StreamToolCall{},
		EventsByType: map[string]int{},
	}
	if s == nil {
		return out
	}

	for _, frame := range s.DataFrames() {
		var p anthropicEventPayload
		if err := json.Unmarshal(frame.Raw, &p); err != nil {
			continue
		}
		// The SSE `event:` line and the payload's own `type` should agree; the
		// payload wins because it is what a client parsing only data frames sees.
		kind := p.Type
		if kind == "" {
			kind = frame.Event
		}
		out.Events = append(out.Events, AnthropicEvent{Index: frame.Index, Type: kind, Raw: frame.Raw})
		out.EventsByType[kind]++

		switch kind {
		case "message_start":
			out.SawStart = true
			if p.Message != nil {
				out.Model = p.Message.Model
				if p.Message.Usage != nil {
					out.HasUsage = true
					out.Usage.Prompt = p.Message.Usage.Input
					out.Usage.Completion = p.Message.Usage.Output
				}
			}
		case "content_block_start":
			if p.ContentBlock != nil && p.ContentBlock.Type == "tool_use" {
				out.ToolCalls[p.Index] = &StreamToolCall{
					ID:   p.ContentBlock.ID,
					Name: p.ContentBlock.Name,
				}
			}
		case "content_block_delta":
			if p.Delta == nil {
				continue
			}
			if p.Delta.Text != "" {
				out.Text += p.Delta.Text
			}
			if p.Delta.PartialJSON != "" {
				call, ok := out.ToolCalls[p.Index]
				if !ok {
					call = &StreamToolCall{}
					out.ToolCalls[p.Index] = call
				}
				call.Arguments += p.Delta.PartialJSON
			}
		case "message_delta":
			if p.Delta != nil && p.Delta.StopReason != "" {
				out.StopReason = p.Delta.StopReason
			}
			// output_tokens is only final here; message_start reports it as 0.
			if p.Usage != nil {
				out.HasUsage = true
				out.Usage.Completion = p.Usage.Output
				if p.Usage.Input > 0 {
					out.Usage.Prompt = p.Usage.Input
				}
			}
		case "message_stop":
			out.SawStop = true
		}
	}

	out.Usage.Total = out.Usage.Prompt + out.Usage.Completion
	return out
}

// Envelope reduces the stream to the shape a buffered response would produce,
// so streaming and non-streaming can be compared on the same terms.
func (a *AnthropicStream) Envelope() Envelope {
	env := Envelope{
		Role:         "assistant",
		Model:        a.Model,
		FinishReason: anthropicStopReason(a.StopReason),
		HasUsage:     a.HasUsage,
		Usage:        a.Usage,
		HasText:      strings.TrimSpace(a.Text) != "",
	}
	idxs := make([]int, 0, len(a.ToolCalls))
	for i := range a.ToolCalls {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	for _, i := range idxs {
		env.ToolCallNames = append(env.ToolCallNames, a.ToolCalls[i].Name)
	}
	sort.Strings(env.ToolCallNames)
	env.ContentKind = classify(env.HasText, len(env.ToolCallNames) > 0)
	return env
}

// StreamInvariants checks the Anthropic event state machine.
func (a *AnthropicStream) StreamInvariants(expectToolCalls bool) []string {
	var problems []string

	if len(a.Events) == 0 {
		return []string{"stream contained no events"}
	}

	if !a.SawStart {
		problems = append(problems,
			"no message_start event; clients build the message envelope from it and have nothing to attach deltas to")
	} else if a.Events[0].Type != "message_start" {
		problems = append(problems, fmt.Sprintf(
			"first event was %q, not message_start", a.Events[0].Type))
	}

	if !a.SawStop {
		problems = append(problems,
			"no message_stop event; the client cannot tell the turn ended")
	} else if last := a.Events[len(a.Events)-1]; last.Type != "message_stop" {
		problems = append(problems, fmt.Sprintf(
			"last event was %q, not message_stop", last.Type))
	}

	if a.StopReason == "" {
		problems = append(problems,
			"no message_delta carried a stop_reason; the client cannot tell why the turn ended")
	}

	// Every started block must be stopped, or a client accumulating deltas per
	// index never learns a block is complete.
	if starts, stops := a.EventsByType["content_block_start"], a.EventsByType["content_block_stop"]; starts != stops {
		problems = append(problems, fmt.Sprintf(
			"%d content_block_start events but %d content_block_stop", starts, stops))
	}

	if expectToolCalls {
		if len(a.ToolCalls) == 0 {
			problems = append(problems, "expected tool calls but the stream produced none")
		}
		idxs := make([]int, 0, len(a.ToolCalls))
		for i := range a.ToolCalls {
			idxs = append(idxs, i)
		}
		sort.Ints(idxs)
		for _, i := range idxs {
			call := a.ToolCalls[i]
			if call.Name == "" {
				problems = append(problems, fmt.Sprintf("tool call at index %d has no name", i))
			}
			if call.ID == "" {
				problems = append(problems, fmt.Sprintf(
					"tool call at index %d has no id; a tool result cannot be correlated back to it", i))
			}
			if !json.Valid([]byte(call.Arguments)) {
				problems = append(problems, fmt.Sprintf(
					"tool call at index %d: input_json_delta fragments do not concatenate to valid JSON: %q",
					i, truncate(call.Arguments, 200)))
			}
		}
	} else if strings.TrimSpace(a.Text) == "" {
		problems = append(problems, "stream produced no text content")
	}

	return problems
}
