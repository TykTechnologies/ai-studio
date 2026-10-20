package extract

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// Formats, as the proxy names them (the request filter format: the vendor or
// the bridge's wire format).
const (
	FormatOpenAI          = "openai"
	FormatOpenAIResponses = "openai_responses"
	FormatOllama          = "ollama"
	FormatAnthropic       = "anthropic"
	FormatGoogleAI        = "google_ai"
	FormatVertex          = "vertex"
	FormatBedrock         = "bedrock"
)

var (
	// ErrUnsupportedFormat: no adapter for the format.
	ErrUnsupportedFormat = errors.New("extract: unsupported format")
	// ErrUnrecognisedBody: the body is not JSON shaped like the format.
	ErrUnrecognisedBody = errors.New("extract: unrecognised body")
)

// ToolCall is one tool call in an LLM response. Arguments appear only as a
// fingerprint: nothing raw is kept.
type ToolCall struct {
	ID   string
	Name string
	// ArgsFP is nil when no Fingerprinter is installed or the arguments are
	// not valid JSON (a truncated or malformed model output).
	ArgsFP agenttrace.Fingerprint
	// Server marks a provider-hosted tool (web search, a remote MCP call the
	// provider makes): it becomes a span event on the LLM span, not a tool
	// span, since the gateway cannot time it (§4.4 R2).
	Server bool
}

// Response is what correlation needs from one LLM response body.
type Response struct {
	ID            string
	FinishReasons []string
	// ToolCalls are client tool calls, in body order; server tool calls are
	// in ServerToolCalls.
	ToolCalls       []ToolCall
	ServerToolCalls []ToolCall
	HasUsage        bool
}

// Apply copies the response extracts into x.
func (r Response) Apply(x *agenttrace.Extracts) {
	x.ResponseID = r.ID
	x.FinishReasons = append(x.FinishReasons, r.FinishReasons...)
	for _, c := range r.ToolCalls {
		x.ToolCalls = append(x.ToolCalls, agenttrace.ToolCallRequest{ID: c.ID, Name: c.Name, ArgsFP: c.ArgsFP})
	}
}

// ParseResponse extracts correlation material from a buffered (or
// reassembled) response body. fp may be nil: ids and names are still
// extracted, fingerprints are not.
func ParseResponse(format string, body []byte, fp agenttrace.Fingerprinter) (Response, error) {
	var r Response
	var err error
	switch strings.ToLower(format) {
	case FormatOpenAI, FormatOllama:
		err = parseOpenAIResponse(body, fp, &r)
	case FormatOpenAIResponses:
		err = parseResponsesResponse(body, fp, &r)
	case FormatAnthropic:
		err = parseAnthropicResponse(body, fp, &r)
	case FormatGoogleAI, FormatVertex:
		err = parseGoogleResponse(body, fp, &r)
	case FormatBedrock:
		err = parseBedrockResponse(body, fp, &r)
	default:
		return r, fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
	}
	return r, err
}

func unmarshal(body []byte, v any) error {
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%w: %v", ErrUnrecognisedBody, err)
	}
	return nil
}

func call(fp agenttrace.Fingerprinter, id, name string, args json.RawMessage, server bool) ToolCall {
	c := ToolCall{ID: id, Name: name, Server: server}
	if f, err := ArgsFingerprint(fp, args); err == nil {
		c.ArgsFP = f
	}
	return c
}

func (r *Response) add(c ToolCall) {
	if c.Server {
		r.ServerToolCalls = append(r.ServerToolCalls, c)
		return
	}
	r.ToolCalls = append(r.ToolCalls, c)
}

func (r *Response) finish(reason string) {
	if reason != "" {
		r.FinishReasons = append(r.FinishReasons, reason)
	}
}

type openAIFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type openAIMessage struct {
	ToolCalls []struct {
		ID       string         `json:"id"`
		Function openAIFunction `json:"function"`
	} `json:"tool_calls"`
	FunctionCall *openAIFunction `json:"function_call"`
}

func (m *openAIMessage) collect(fp agenttrace.Fingerprinter, r *Response) {
	if m == nil {
		return
	}
	for _, tc := range m.ToolCalls {
		r.add(call(fp, tc.ID, tc.Function.Name, tc.Function.Arguments, false))
	}
	if fc := m.FunctionCall; fc != nil {
		r.add(call(fp, "", fc.Name, fc.Arguments, false))
	}
}

// parseOpenAIResponse reads Chat Completions (choices[]) and Ollama
// (message, done_reason).
func parseOpenAIResponse(body []byte, fp agenttrace.Fingerprinter, r *Response) error {
	var b struct {
		ID      string `json:"id"`
		Choices []struct {
			FinishReason string         `json:"finish_reason"`
			Message      *openAIMessage `json:"message"`
		} `json:"choices"`
		Message    *openAIMessage  `json:"message"`
		DoneReason string          `json:"done_reason"`
		Usage      json.RawMessage `json:"usage"`
		EvalCount  *int            `json:"eval_count"`
		Output     json.RawMessage `json:"output"`
	}
	if err := unmarshal(body, &b); err != nil {
		return err
	}
	if b.Choices == nil && len(b.Output) > 0 && b.Output[0] == '[' {
		// A Responses API body under the vendor's name.
		return parseResponsesResponse(body, fp, r)
	}
	r.ID = b.ID
	for _, c := range b.Choices {
		c.Message.collect(fp, r)
		r.finish(c.FinishReason)
	}
	b.Message.collect(fp, r)
	r.finish(b.DoneReason)
	r.HasUsage = present(b.Usage) || b.EvalCount != nil
	return nil
}

// Responses API output item types the provider executes itself.
var responsesServerItems = map[string]bool{
	"web_search_call":       true,
	"file_search_call":      true,
	"code_interpreter_call": true,
	"image_generation_call": true,
	"mcp_call":              true,
	"mcp_list_tools":        true,
	"mcp_approval_request":  true,
}

func parseResponsesResponse(body []byte, fp agenttrace.Fingerprinter, r *Response) error {
	var b struct {
		ID                string `json:"id"`
		Status            string `json:"status"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Output []struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			CallID    string          `json:"call_id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Input     json.RawMessage `json:"input"`
			Action    json.RawMessage `json:"action"`
		} `json:"output"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := unmarshal(body, &b); err != nil {
		return err
	}
	r.ID = b.ID
	for _, it := range b.Output {
		id := it.CallID
		if id == "" {
			id = it.ID
		}
		switch it.Type {
		case "function_call":
			r.add(call(fp, id, it.Name, it.Arguments, false))
		case "custom_tool_call":
			r.add(call(fp, id, it.Name, it.Input, false))
		case "computer_call", "local_shell_call":
			r.add(call(fp, id, it.Type, it.Action, false))
		default:
			if responsesServerItems[it.Type] {
				name := it.Name
				if name == "" {
					name = strings.TrimSuffix(it.Type, "_call")
				}
				r.add(call(fp, id, name, it.Arguments, true))
			}
		}
	}
	r.finish(b.Status)
	if b.IncompleteDetails != nil {
		r.finish(b.IncompleteDetails.Reason)
	}
	r.HasUsage = present(b.Usage)
	return nil
}

type anthropicBlock struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func collectAnthropic(blocks []anthropicBlock, fp agenttrace.Fingerprinter, r *Response) {
	for _, bl := range blocks {
		switch bl.Type {
		case "tool_use":
			r.add(call(fp, bl.ID, bl.Name, bl.Input, false))
		case "server_tool_use", "mcp_tool_use":
			r.add(call(fp, bl.ID, bl.Name, bl.Input, true))
		}
	}
}

func parseAnthropicResponse(body []byte, fp agenttrace.Fingerprinter, r *Response) error {
	var b struct {
		ID         string           `json:"id"`
		StopReason string           `json:"stop_reason"`
		Content    []anthropicBlock `json:"content"`
		Usage      json.RawMessage  `json:"usage"`
	}
	if err := unmarshal(body, &b); err != nil {
		return err
	}
	r.ID = b.ID
	collectAnthropic(b.Content, fp, r)
	r.finish(b.StopReason)
	r.HasUsage = present(b.Usage)
	return nil
}

func parseGoogleResponse(body []byte, fp agenttrace.Fingerprinter, r *Response) error {
	type functionCall struct {
		ID   string          `json:"id"`
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	}
	var b struct {
		ResponseID string `json:"responseId"`
		Candidates []struct {
			FinishReason string `json:"finishReason"`
			Content      struct {
				Parts []struct {
					FunctionCall      *functionCall `json:"functionCall"`
					FunctionCallSnake *functionCall `json:"function_call"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata json.RawMessage `json:"usageMetadata"`
	}
	if err := unmarshal(body, &b); err != nil {
		return err
	}
	r.ID = b.ResponseID
	for _, c := range b.Candidates {
		for _, p := range c.Content.Parts {
			fc := p.FunctionCall
			if fc == nil {
				fc = p.FunctionCallSnake
			}
			if fc != nil {
				r.add(call(fp, fc.ID, fc.Name, fc.Args, false))
			}
		}
		r.finish(c.FinishReason)
	}
	r.HasUsage = present(b.UsageMetadata)
	return nil
}

// parseBedrockResponse reads a Converse response, or an Anthropic body from
// InvokeModel.
func parseBedrockResponse(body []byte, fp agenttrace.Fingerprinter, r *Response) error {
	var b struct {
		ID         string           `json:"id"`
		StopReason string           `json:"stop_reason"`
		Content    []anthropicBlock `json:"content"`
		Converse   string           `json:"stopReason"`
		Output     struct {
			Message struct {
				Content []struct {
					ToolUse *struct {
						ToolUseID string          `json:"toolUseId"`
						Name      string          `json:"name"`
						Input     json.RawMessage `json:"input"`
					} `json:"toolUse"`
				} `json:"content"`
			} `json:"message"`
		} `json:"output"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := unmarshal(body, &b); err != nil {
		return err
	}
	if b.Content != nil {
		r.ID = b.ID
		collectAnthropic(b.Content, fp, r)
		r.finish(b.StopReason)
	} else {
		for _, c := range b.Output.Message.Content {
			if tu := c.ToolUse; tu != nil {
				r.add(call(fp, tu.ToolUseID, tu.Name, tu.Input, false))
			}
		}
		r.finish(b.Converse)
	}
	r.HasUsage = present(b.Usage)
	return nil
}

func present(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}
