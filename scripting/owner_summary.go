package scripting

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// OwnerSummary is what an App's owner sees of a guardrail compliance event
// (evidence.share_with_app_owners): the action taken, the reason code the
// caller got (empty when the request went on) and a sentence naming the
// cause and where it was. It reads only the metadata fields that carry no
// text: never an excerpt, a score or a detector.
func OwnerSummary(eventType string, m map[string]any) (action, code, text string) {
	action, _ = m["action"].(string)
	if blocked, _ := m["blocked"].(bool); blocked {
		action = guardrails.ActionBlock
	}
	seg := guardrails.Segment{}
	if segs, ok := m["segments"].([]any); ok && len(segs) > 0 {
		if s, ok := segs[0].(map[string]any); ok {
			seg.Role, _ = s["role"].(string)
			seg.Kind, _ = s["kind"].(string)
			seg.ToolName, _ = s["tool"].(string)
		}
	} else if kinds, ok := m["kinds"].([]any); ok && len(kinds) == 1 {
		seg.Kind, _ = kinds[0].(string)
		if roles, ok := m["roles"].([]any); ok && len(roles) == 1 {
			seg.Role, _ = roles[0].(string)
		}
		if tools, ok := m["tool_names"].([]any); ok && len(tools) == 1 {
			seg.ToolName, _ = tools[0].(string)
		}
	}
	src := segmentSource(seg)
	num := func(k string) int {
		f, _ := m[k].(float64)
		return int(f)
	}
	switch eventType {
	case "guardrail.unscanned":
		reason, _ := m["reason"].(string)
		code = unscannedReasonCode(reason)
		switch reason {
		case unscannedSize:
			text = fmt.Sprintf("%s of %s characters is longer than the guardrail scans (%s characters), so it was not checked",
				src, thousands(num("max_chars")), thousands(num("limit")))
		case unscannedPieces, unscannedTimeBudget:
			text = fmt.Sprintf("the request had more text than the guardrail checks per request, so %s was not checked", src)
		default:
			text = fmt.Sprintf("the guardrail could not check %s", src)
		}
	case "guardrail.error":
		code = ReasonUnavailable
		text = "the guardrail provider was unavailable"
	case "guardrail.unreadable_request":
		code = ReasonUnreadableRequest
		text = "the request has a repeated JSON key and cannot be screened"
	default:
		code = ReasonDetected
		category, _ := m["category"].(string)
		what, ok := categoryWords[category]
		if !ok {
			what = "content the guardrail's policy covers"
		}
		text = fmt.Sprintf("the guardrail found %s in %s", what, src)
	}
	if action != guardrails.ActionBlock {
		code = ""
	}
	return action, code, text
}
