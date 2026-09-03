package vendorconformance

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// StreamFrame is one `data:` payload from an SSE response, in arrival order.
type StreamFrame struct {
	// Index is the frame's position in the stream, counting only data frames.
	Index int
	// Raw is the payload after "data: ", before JSON decoding.
	Raw []byte
	// Done is true for the literal "[DONE]" sentinel, which is not JSON.
	Done bool
	// Event is the SSE event name, if the server sent one. Anthropic's native
	// stream is event-typed; OpenAI's is not.
	Event string
}

// Stream is a fully-consumed SSE response plus everything the cross-chunk
// invariants need. Reassembly happens once, here, so each assertion reads a
// field rather than re-walking the frames.
type Stream struct {
	Frames []StreamFrame

	// Text is the concatenation of every delta.content fragment.
	Text string
	// ToolCalls holds reassembled tool calls, keyed by their streamed index.
	ToolCalls map[int]*StreamToolCall
	// FinishReason is the last non-empty finish_reason seen.
	FinishReason string
	// Usage is from the final usage-bearing chunk, if any.
	Usage    Usage
	HasUsage bool
	// Model is from the first chunk that carried one.
	Model string

	// SawDone reports whether the [DONE] sentinel arrived.
	SawDone bool
	// FramesAfterDone counts data frames that arrived after [DONE]. Any is a bug:
	// clients stop reading at the sentinel and would never see them.
	FramesAfterDone int
	// RoleFrames records the frame indices that carried delta.role.
	RoleFrames []int
}

// StreamToolCall is a tool call reassembled from its streamed fragments.
type StreamToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ParseStream reads an SSE body to completion. It returns an error only for
// malformed framing; schema and invariant problems are reported by Validate and
// AssertStreamInvariants so a caller sees all of them at once rather than the
// first.
func ParseStream(body io.Reader) (*Stream, error) {
	s := &Stream{ToolCalls: map[int]*StreamToolCall{}}

	scanner := bufio.NewScanner(body)
	// SSE frames carrying a large tool-call argument or a base64 image can far
	// exceed bufio's 64KB default, and the failure mode is a truncated line that
	// looks like malformed JSON from the vendor.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	var currentEvent string
	idx := 0

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			currentEvent = ""
			continue
		case strings.HasPrefix(line, ":"):
			continue // comment / keepalive
		case strings.HasPrefix(line, "event:"):
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		case !strings.HasPrefix(line, "data:"):
			continue // id:, retry:, or a field we do not use
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		frame := StreamFrame{Index: idx, Raw: []byte(payload), Event: currentEvent}
		idx++

		if payload == "[DONE]" {
			frame.Done = true
			s.SawDone = true
			s.Frames = append(s.Frames, frame)
			continue
		}
		if s.SawDone {
			s.FramesAfterDone++
		}
		s.Frames = append(s.Frames, frame)
		s.accumulate(frame)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading SSE stream: %w", err)
	}
	return s, nil
}

// chunk is the subset of an OpenAI streaming chunk the reassembler needs.
type chunk struct {
	Model   string `json:"model"`
	Choices []struct {
		FinishReason *string `json:"finish_reason"`
		Delta        struct {
			Role      *string `json:"role"`
			Content   *string `json:"content"`
			ToolCalls []struct {
				Index    int     `json:"index"`
				ID       *string `json:"id"`
				Function *struct {
					Name      *string `json:"name"`
					Arguments *string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		Prompt     int `json:"prompt_tokens"`
		Completion int `json:"completion_tokens"`
		Total      int `json:"total_tokens"`
	} `json:"usage"`
}

func (s *Stream) accumulate(f StreamFrame) {
	var c chunk
	if err := json.Unmarshal(f.Raw, &c); err != nil {
		// Not decodable as an OpenAI chunk. Validate reports the schema failure
		// with a far better message than anything we could add here.
		return
	}

	if s.Model == "" && c.Model != "" {
		s.Model = c.Model
	}
	if c.Usage != nil {
		s.HasUsage = true
		s.Usage = Usage{c.Usage.Prompt, c.Usage.Completion, c.Usage.Total}
	}

	for _, ch := range c.Choices {
		if ch.Delta.Role != nil && *ch.Delta.Role != "" {
			s.RoleFrames = append(s.RoleFrames, f.Index)
		}
		if ch.Delta.Content != nil {
			s.Text += *ch.Delta.Content
		}
		if ch.FinishReason != nil && *ch.FinishReason != "" {
			s.FinishReason = *ch.FinishReason
		}
		for _, tc := range ch.Delta.ToolCalls {
			call, ok := s.ToolCalls[tc.Index]
			if !ok {
				call = &StreamToolCall{}
				s.ToolCalls[tc.Index] = call
			}
			if tc.ID != nil && *tc.ID != "" {
				call.ID = *tc.ID
			}
			if tc.Function != nil {
				if tc.Function.Name != nil && *tc.Function.Name != "" {
					call.Name = *tc.Function.Name
				}
				if tc.Function.Arguments != nil {
					call.Arguments += *tc.Function.Arguments
				}
			}
		}
	}
}

// DataFrames returns the JSON frames, excluding the [DONE] sentinel.
func (s *Stream) DataFrames() []StreamFrame {
	var out []StreamFrame
	for _, f := range s.Frames {
		if !f.Done {
			out = append(out, f)
		}
	}
	return out
}

// Envelope reduces a finished stream to the same canonical shape a
// non-streaming response produces, so the two can be compared directly. A
// streaming and non-streaming call to the same model must agree on shape; when
// they do not, one of the two code paths is wrong.
func (s *Stream) Envelope() Envelope {
	env := Envelope{
		Role:         "assistant",
		FinishReason: s.FinishReason,
		Model:        s.Model,
		HasUsage:     s.HasUsage,
		Usage:        s.Usage,
		HasText:      strings.TrimSpace(s.Text) != "",
	}
	for _, idx := range sortedKeys(s.ToolCalls) {
		env.ToolCallNames = append(env.ToolCallNames, s.ToolCalls[idx].Name)
	}
	sort.Strings(env.ToolCallNames)
	env.ContentKind = classify(env.HasText, len(env.ToolCallNames) > 0)
	return env
}

// StreamInvariants checks the cross-chunk rules a per-chunk JSON Schema cannot
// express. It returns one message per violation; an empty slice means clean.
//
// expectToolCalls tells the check whether the scenario asked for tool use, since
// an empty text stream is a failure for a text scenario but expected for a
// tool-call one.
func (s *Stream) StreamInvariants(expectToolCalls bool) []string {
	var problems []string

	if len(s.DataFrames()) == 0 {
		problems = append(problems, "stream contained no data frames")
		return problems
	}

	if !s.SawDone {
		problems = append(problems,
			"stream never sent the [DONE] sentinel; OpenAI SDKs block or hang waiting for it")
	}
	if s.FramesAfterDone > 0 {
		problems = append(problems, fmt.Sprintf(
			"%d data frame(s) arrived after [DONE]; clients stop reading at the sentinel and never see them",
			s.FramesAfterDone))
	}

	switch {
	case len(s.RoleFrames) == 0:
		problems = append(problems,
			"no chunk carried delta.role; clients that build a message from the stream have no role to assign")
	case s.RoleFrames[0] != 0:
		problems = append(problems, fmt.Sprintf(
			"delta.role first appeared on frame %d, not the first frame", s.RoleFrames[0]))
	}
	if len(s.RoleFrames) > 1 {
		problems = append(problems, fmt.Sprintf(
			"delta.role repeated on %d frames (%v); it belongs on the first frame only",
			len(s.RoleFrames), s.RoleFrames))
	}

	if s.FinishReason == "" {
		problems = append(problems,
			"no chunk carried a finish_reason; the client cannot tell why the turn ended")
	}

	if expectToolCalls {
		if len(s.ToolCalls) == 0 {
			problems = append(problems, "expected tool calls but the stream produced none")
		}
		for _, idx := range sortedKeys(s.ToolCalls) {
			call := s.ToolCalls[idx]
			if call.Name == "" {
				problems = append(problems, fmt.Sprintf("tool call %d has no function name", idx))
			}
			if call.ID == "" {
				problems = append(problems, fmt.Sprintf(
					"tool call %d has no id; the client cannot correlate a tool result back to it", idx))
			}
			if !json.Valid([]byte(call.Arguments)) {
				problems = append(problems, fmt.Sprintf(
					"tool call %d arguments do not concatenate to valid JSON: %q", idx, truncate(call.Arguments, 200)))
			}
		}
	} else if strings.TrimSpace(s.Text) == "" {
		problems = append(problems, "stream produced no text content")
	}

	return problems
}

func sortedKeys(m map[int]*StreamToolCall) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
