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
	"time"
)

// Direction says which side of the LLM call the text comes from.
type Direction string

const (
	DirectionRequest  Direction = "request"
	DirectionResponse Direction = "response"
)

// Segment is one piece of text a provider classifies: a message, a response
// buffer, a tool call envelope. Index is the position in Input.Segments and is
// how findings and rewrites refer back to it.
type Segment struct {
	Index int    `json:"index"`
	Role  string `json:"role"` // system, user, assistant, tool
	Text  string `json:"text"`
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
	// runner evaluates more often when streaming.
	Local bool
}

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
