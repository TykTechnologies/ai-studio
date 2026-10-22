package scripting

import (
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// Reason codes a guardrail block answers with (ScriptOutput.ReasonCode). They
// are stable: clients and runbooks branch on them.
const (
	// ReasonDetected: the provider flagged the text.
	ReasonDetected = "guardrail_detected"
	// ReasonTooLargeToScan: a message or tool result is longer than the
	// filter scans (budget.max_scan_chars).
	ReasonTooLargeToScan = "guardrail_too_large_to_scan"
	// ReasonScanBudgetExceeded: the request had more text than the filter
	// scans per request (budget.max_pieces) or in its time (budget.time_ms).
	ReasonScanBudgetExceeded = "guardrail_scan_budget_exceeded"
	// ReasonUnavailable: the provider could not be reached or answer, and
	// the filter fails closed.
	ReasonUnavailable = "guardrail_unavailable"
	// ReasonUnscanned: text could not be screened for another reason (the
	// provider refused it, it could not be decoded).
	ReasonUnscanned = "guardrail_unscanned"
	// ReasonUnreadableRequest: the body reads differently to different JSON
	// parsers (a repeated or case-variant key), so it cannot be screened.
	ReasonUnreadableRequest = "guardrail_unreadable_request"
)

// reasonRank orders reason codes when a block has several causes: the one a
// client most needs to know comes first.
var reasonRank = map[string]int{
	ReasonDetected: 0, ReasonTooLargeToScan: 1, ReasonScanBudgetExceeded: 2, ReasonUnavailable: 3, ReasonUnscanned: 4,
}

// categoryWords names a finding's category for a client, without detector
// names or scores.
var categoryWords = map[string]string{
	guardrails.CategoryInjection:  "a likely prompt injection",
	guardrails.CategorySecrets:    "credentials",
	guardrails.CategoryPII:        "personal data",
	guardrails.CategoryModeration: "harmful content",
	guardrails.CategoryLeak:       "a likely system prompt or data leak",
	guardrails.CategoryTopic:      "a blocked topic",
}

// segmentSource says where a segment's text came from, for a client.
func segmentSource(seg guardrails.Segment) string {
	tool := safeLabel(seg.ToolName)
	switch segmentKind(seg) {
	case guardrails.KindToolResult:
		if tool != "" {
			return "a tool result from " + tool
		}
		return "a tool result"
	case guardrails.KindToolCall:
		if tool != "" {
			return "a call to the tool " + tool
		}
		return "a tool call"
	case guardrails.KindToolDefinition:
		if tool != "" {
			return "the definition of the tool " + tool
		}
		return "a tool definition"
	}
	switch seg.Role {
	case "system":
		return "the system prompt"
	case "assistant":
		return "an assistant message"
	case "user":
		return "a user message"
	}
	return "the request"
}

// unscannedReasonCode maps an unscanned reason to its code.
func unscannedReasonCode(reason string) string {
	switch reason {
	case unscannedSize:
		return ReasonTooLargeToScan
	case unscannedPieces, unscannedTimeBudget:
		return ReasonScanBudgetExceeded
	case unscannedProviderError, unscannedProviderPanic, unscannedBusy, unscannedShadowDropped:
		return ReasonUnavailable
	}
	return ReasonUnscanned
}

// blockReason explains a guardrail block for the client: a stable code and a
// sentence naming the cause and where it was, never what was matched, the
// score or the detector. When several segments were blocked, the most
// telling cause is named and the rest counted.
func blockReason(cfg guardrails.Config, segs []guardrails.Segment, o outcome, unscanned map[int]string) (code, text string) {
	type cause struct {
		code, text string
	}
	var causes []cause
	idx := mapsKeys(o.actions)
	slices.Sort(idx)
	for _, i := range idx {
		if o.actions[i] != guardrails.ActionBlock || i < 0 || i >= len(segs) {
			continue
		}
		seg := segs[i]
		src := segmentSource(seg)
		if reason, ok := unscanned[i]; ok {
			c := cause{code: unscannedReasonCode(reason)}
			switch reason {
			case unscannedSize:
				c.text = fmt.Sprintf("%s of %s characters is longer than this guardrail scans (%s characters, budget.max_scan_chars), so it was not checked",
					src, thousands(utf8.RuneCountInString(seg.Text)), thousands(cfg.Budget.MaxScanChars))
			case unscannedPieces:
				c.text = fmt.Sprintf("the request has more text than this guardrail scans in one request (%s pieces, budget.max_pieces), so %s was not checked",
					thousands(cfg.Budget.MaxPieces), src)
			case unscannedTimeBudget:
				c.text = fmt.Sprintf("the guardrail ran out of time (budget.time_ms %s) before it checked %s", thousands(cfg.Budget.TimeMs), src)
			case unscannedProviderError, unscannedProviderPanic, unscannedBusy, unscannedShadowDropped:
				c.text = fmt.Sprintf("the guardrail provider could not check %s", src)
			default:
				c.text = fmt.Sprintf("%s could not be screened (%s)", src, reason)
			}
			causes = append(causes, c)
			continue
		}
		what := "content the guardrail's policy blocks"
		for _, f := range o.findings[i] {
			if w, ok := categoryWords[f.Category]; ok {
				what = w
				break
			}
		}
		causes = append(causes, cause{code: ReasonDetected, text: fmt.Sprintf("the guardrail found %s in %s", what, src)})
	}
	if len(causes) == 0 {
		// A block with no blocked segment (a verdict on the request as a
		// whole): the provider flagged it.
		return ReasonDetected, "the guardrail flagged the request"
	}
	slices.SortStableFunc(causes, func(a, b cause) int { return reasonRank[a.code] - reasonRank[b.code] })
	text = causes[0].text
	if n := len(causes) - 1; n == 1 {
		text += " (and 1 more message or tool result)"
	} else if n > 1 {
		text += fmt.Sprintf(" (and %d more messages or tool results)", n)
	}
	return causes[0].code, text
}

func mapsKeys[V any](m map[int]V) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// thousands formats n with comma separators.
func thousands(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return s
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
