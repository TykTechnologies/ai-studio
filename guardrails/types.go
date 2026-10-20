// Package guardrails is the provider framework behind guardrail filters: a
// filter kind that classifies text with a typed provider (the built-in
// pattern library, or an external classifier or NER service) instead of a
// script, and blocks, redacts or logs on the verdict.
//
// The package owns the contract every provider implements and the
// configuration an administrator writes. Providers register themselves with
// RegisterFactory; the Enterprise build registers the real ones, the
// Community build registers none and every guardrail filter passes through,
// exactly as script filters do. The mapping from a filter's input (request
// messages, a response buffer, a tool call) onto Segments, and from a Verdict
// back onto a block/redact/log decision, lives in the scripting package next
// to the script runner so both kinds share one execution chain.
package guardrails

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Direction says which side of the LLM call the text comes from.
type Direction string

const (
	DirectionRequest  Direction = "request"
	DirectionResponse Direction = "response"
)

// Segment roles.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Segment kinds. A tool result is the output of a tool the client ran and
// sent back to the model: in OpenAI a role "tool" message, in Anthropic a
// tool_result block inside a user message, in Google a functionResponse
// part. It is the main channel for indirect prompt injection.
//
// A tool call is the model asking for a tool to run: its arguments (OpenAI
// tool_calls[].function.arguments, Anthropic tool_use input, Google
// functionCall args, Bedrock toolUse input, Responses function_call
// arguments), in a request's history or in a response. It is where an
// injection that reached the model turns into an action, such as data sent
// to an address the attacker chose. Tool calls are segments only on filters
// that scan them (Config.ToolCalls).
//
// A tool definition is a tool the request offers the model: its name,
// description and parameter schema (OpenAI tools and functions, Anthropic
// tools, Google functionDeclarations, Bedrock toolConfig, Responses tools).
// Definitions often come from MCP servers or OpenAPI specs a third party
// wrote, and a description can carry instructions the model follows (tool
// poisoning). They are segments only on filters that scan them
// (Config.ToolDefinitions).
const (
	KindText           = "text"
	KindToolResult     = "tool_result"
	KindToolCall       = "tool_call"
	KindToolDefinition = "tool_definition"
)

// Segment is one piece of text a provider classifies: a message, a response
// buffer, a tool call envelope, one block of a request. Index is the position
// in Input.Segments and is how findings and rewrites refer back to it.
type Segment struct {
	Index int    `json:"index"`
	Role  string `json:"role"` // system, user, assistant, tool
	Text  string `json:"text"`
	// Kind is KindText, KindToolResult or KindToolCall; empty means text.
	Kind string `json:"kind,omitempty"`
	// ToolName and ToolCallID identify the call a tool result answers, or a
	// tool call itself, when the request names them.
	ToolName   string `json:"tool_name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	// Context is what the model was asked to do when it made a tool call:
	// set on tool call segments only (never on tool results, whose calls
	// stay free of the user's text), and sent only to providers that ask
	// for it (Capabilities.WantsGoal).
	Context *SegmentContext `json:"context,omitempty"`
}

// SegmentContext is the goal behind a tool call: the latest user text before
// the call and, when the filter asks for it, the system prompt, each cut to
// MaxContextChars.
type SegmentContext struct {
	UserGoal string `json:"user_goal,omitempty"`
	System   string `json:"system,omitempty"`
}

// MaxContextChars bounds each text of a SegmentContext, in characters. A
// longer text keeps its first third and its last two thirds, as the
// classifiers the context is for truncate their input.
const MaxContextChars = 4096

// IsToolResult reports text a tool produced, as opposed to the client's
// own user or system text.
func (s Segment) IsToolResult() bool {
	return s.Kind == KindToolResult || s.Role == RoleTool
}

// IsToolCall reports a tool call's arguments.
func (s Segment) IsToolCall() bool {
	return s.Kind == KindToolCall
}

// IsToolDefinition reports a tool definition.
func (s Segment) IsToolDefinition() bool {
	return s.Kind == KindToolDefinition
}

// Untrusted reports text the client did not write itself: a tool result, a
// tool call the model made (whose arguments can carry what a tool result
// told it), or a tool definition (often written by a third party). Untrusted
// text never shares a provider call with the client's own text, so text
// built to make its call fail cannot cost the user's prompt.
func (s Segment) Untrusted() bool {
	return s.IsToolResult() || s.IsToolCall() || s.IsToolDefinition()
}

// ForProvider is what a provider with caps is sent of s, at index with
// text (s's text, or a piece of it): its role and text, and for a tool
// result or tool call its tool metadata when the provider handles them
// (Capabilities.ToolSegments). A provider that does not is sent a tool
// result as user text and a tool call as assistant text, roles every
// provider accepts; its findings still come back onto the segment by index.
// A tool call's context goes only to a provider that asks for it
// (Capabilities.WantsGoal).
func (s Segment) ForProvider(index int, text string, caps Capabilities) Segment {
	switch {
	case s.IsToolDefinition():
		// Untrusted metadata: plain user text to a provider that does not
		// know definitions, as for tool results.
		if !caps.ToolSegments {
			return Segment{Index: index, Role: RoleUser, Text: text}
		}
		return Segment{Index: index, Role: RoleSystem, Text: text, Kind: KindToolDefinition, ToolName: s.ToolName}
	case s.IsToolCall():
		if !caps.ToolSegments {
			return Segment{Index: index, Role: RoleAssistant, Text: text}
		}
		out := Segment{Index: index, Role: RoleAssistant, Text: text, Kind: KindToolCall, ToolName: s.ToolName, ToolCallID: s.ToolCallID}
		if caps.WantsGoal && s.Context != nil {
			c := *s.Context
			out.Context = &c
		}
		return out
	case !s.IsToolResult():
		return Segment{Index: index, Role: s.Role, Text: text}
	case !caps.ToolSegments:
		return Segment{Index: index, Role: RoleUser, Text: text}
	}
	return Segment{Index: index, Role: RoleTool, Text: text, Kind: KindToolResult, ToolName: s.ToolName, ToolCallID: s.ToolCallID}
}

// ClipContext cuts text to MaxContextChars, keeping its first third and its
// last two thirds around a marker line.
func ClipContext(text string) string {
	runes := []rune(text)
	if len(runes) <= MaxContextChars {
		return text
	}
	const marker = "\n[…]\n"
	keep := MaxContextChars - len([]rune(marker))
	head := keep / 3
	return string(runes[:head]) + marker + string(runes[len(runes)-(keep-head):])
}

// RedactionSpec asks a provider that can rewrite text to do so.
type RedactionSpec struct {
	// Style is one of RedactionPlaceholder, RedactionMask, RedactionHash.
	Style string
	// Placeholder is the template for RedactionPlaceholder. {{type}} expands
	// to the detector id's last element in upper case (AWS_ACCESS_KEY,
	// EMAIL), {{category}} to the category (SECRETS, PII).
	Placeholder string
}

// Input is what a provider classifies.
type Input struct {
	Segments  []Segment
	Direction Direction
	// Redact is set when the filter's action is redact and the provider
	// reports Capabilities.Redacts; the provider then fills Verdict.Rewritten.
	Redact *RedactionSpec
	// Meta carries request context a provider may forward (request_id, llm_id,
	// app_id, vendor, model). Never contains credentials.
	Meta map[string]any
}

// Span is a byte range inside a segment's Text.
type Span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Finding is one detection. It never carries the matched text: for a secret
// that would put the credential into the compliance log the guardrail exists
// to keep it out of.
type Finding struct {
	// Detector is the provider-scoped id, e.g. "secrets.aws_access_key",
	// "prompt_attack", "pii/email", "Hate".
	Detector string `json:"detector"`
	// Category groups detectors: secrets, pii, injection, moderation, leak.
	Category string `json:"category"`
	// Label is the human-readable name of the detector.
	Label string `json:"label"`
	// Score is the provider's confidence or severity normalised to 0..1 where
	// the provider gives one; 1 for boolean detectors.
	Score float64 `json:"score"`
	// Severity is info, warning or critical, as the compliance event will
	// record it.
	Severity string `json:"severity"`
	// SegmentIndex is the segment the finding is in.
	SegmentIndex int `json:"segment_index"`
	// Span locates the finding inside the segment when the provider knows it.
	Span *Span `json:"span,omitempty"`
}

// Verdict is a provider's answer.
type Verdict struct {
	Flagged  bool      `json:"flagged"`
	Findings []Finding `json:"findings"`
	// Rewritten maps a segment index to its redacted text. Only segments that
	// changed are present. Nil when the provider cannot rewrite or was not
	// asked to.
	Rewritten map[int]string `json:"rewritten,omitempty"`
	// Latency is the provider call's wall time, for metrics.
	Latency time.Duration `json:"-"`
}

// Capabilities describes what a provider can do so the runner can shape the
// call.
type Capabilities struct {
	// Redacts reports whether the provider can return rewritten segments.
	Redacts bool
	// MaxChars is the largest text one call accepts; 0 means unbounded. The
	// runner splits longer segments.
	MaxChars int
	// Local reports an in-process provider with no network cost, which the
	// runner evaluates more often when streaming, scans in full at any size
	// and runs outside the remote budgets (Budget).
	Local bool
	// MaxCallChars is the most text, in characters over all segments, one
	// call accepts; 0 means unbounded. The runner packs pieces into calls
	// that stay within it, so a provider that pays per segment and would
	// rather work in parallel sets it to MaxChars.
	MaxCallChars int
	// Contextual reports a provider whose verdict on one segment depends on
	// the other segments sent with it (a conversation-aware classifier).
	// The runner then never answers a segment from the verdict cache, since
	// that would send the provider the rest of the request without it.
	Contextual bool
	// Version changes when the provider's verdict for unchanged input and
	// config may change (a new model, a new pattern set). It is part of the
	// verdict cache key.
	Version string
	// SegmentPerCall reports a provider that makes a request of its own for
	// every segment it is sent. The runner then sends each segment in calls
	// of its own, so the requests run in parallel within the time budget,
	// each under the call timeout, rather than one after another under one.
	// A run of several such calls is never small, so a failure there leaves
	// its segments unscanned rather than to the fail mode.
	SegmentPerCall bool
	// ToolSegments reports a provider that handles tool results as such:
	// it is sent segments with role tool and their Kind, ToolName and
	// ToolCallID. A provider without it is sent a tool result as role user
	// with no tool metadata, a role every provider accepts, and its
	// findings are mapped back onto the tool result. The same holds for tool
	// calls: with it they arrive as role assistant with Kind tool_call and
	// the tool's name and call id, without it as plain assistant text.
	ToolSegments bool
	// WantsGoal reports a provider that judges a tool call against what the
	// model was asked to do: it is sent each tool call segment's Context
	// (the user's goal, and the system prompt where the filter includes
	// it). Only meaningful with ToolSegments. Other providers never receive
	// the user's text alongside a tool call.
	WantsGoal bool
}

// DefaultCallChars is the call size the runner uses for a remote provider
// that declares no MaxCallChars, so a large request becomes several calls.
const DefaultCallChars = 32 << 10

// Provider classifies text.
type Provider interface {
	Classify(ctx context.Context, in Input) (Verdict, error)
	Capabilities() Capabilities
}

// ErrNoProviders is returned by NewProvider when no factory is registered at
// all, which is the Community Edition: guardrail filters are accepted and
// stored but never enforced, exactly like script filters.
var ErrNoProviders = errors.New("guardrail providers are an Enterprise feature")

// ErrUnknownProvider is returned by NewProvider for a provider name no
// registered factory claims.
var ErrUnknownProvider = errors.New("unknown guardrail provider")

// ErrInvalidConfig wraps every validation failure from ParseConfig and
// Normalize, so a caller can tell a rejected configuration (the user's
// mistake) from anything else with errors.Is rather than by reading the
// message. Its text is the prefix the messages have always carried.
var ErrInvalidConfig = errors.New("guardrail config")

// ErrInputRejected marks a provider error caused by the text it was sent
// (too long, an encoding its validator refuses) rather than by the
// provider's availability. Whoever wrote the text can cause it at will, so
// the runner treats the text as unscanned and never applies the fail mode
// to it: under fail open that would let the text through unscreened.
// Providers wrap such errors with it, or return a StatusError, which
// matches it for 400, 413 and 422.
var ErrInputRejected = errors.New("guardrail provider rejected the input")

// ErrProviderPanic is a provider call that panicked. The runner recovers it
// in the call's goroutine so the process survives. Like ErrInputRejected it
// may have been provoked by the text, so on the request side the call's text
// is treated as unscanned, never let through by the fail mode.
var ErrProviderPanic = errors.New("guardrail provider panicked")

// ErrProviderSuspended is a call the runner did not make because the
// filter's provider has repeatedly failed to answer within its timeout (see
// the call breaker in scripting). It is an operational failure, like a
// timeout.
var ErrProviderSuspended = errors.New("guardrail provider calls suspended after repeated hangs")

// StatusError is a non-2xx HTTP answer from a provider. It carries the
// status only: a provider's error body may echo the text it was sent, which
// must not reach logs or compliance events.
type StatusError struct {
	Status int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("provider returned HTTP %d", e.Status)
}

// Is makes a 400, 413 or 422 match ErrInputRejected.
func (e *StatusError) Is(target error) bool {
	if target != ErrInputRejected {
		return false
	}
	switch e.Status {
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return true
	}
	return false
}

// ErrorClass names the kind of a provider error for logs, metrics and
// compliance events, which never carry the error text itself:
// input_rejected, panic, timeout, canceled, http_<status> or error.
func ErrorClass(err error) string {
	var se *StatusError
	switch {
	case errors.Is(err, ErrInputRejected):
		return "input_rejected"
	case errors.Is(err, ErrProviderPanic):
		return "panic"
	case errors.Is(err, ErrProviderSuspended):
		return "suspended"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.As(err, &se):
		return fmt.Sprintf("http_%d", se.Status)
	}
	return "error"
}
