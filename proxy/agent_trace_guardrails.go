package proxy

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/scripting"
)

// recordFilterOutcome records what one filter did to a request or response
// on the request's capture (spec §3.2), next to its compliance events: a
// guardrail that found something becomes one guardrail gateway event (a
// block, a quarantine or redaction, or a finding it only logged), and a
// script filter that blocks becomes a filter event. The event names the
// filter; its detail lists the detectors and where they matched, never the
// content. A filter that found nothing records nothing, and nothing is
// recorded with tracing off.
func recordFilterOutcome(ctx context.Context, filter *models.Filter, out *scripting.ScriptOutput) {
	rec := agenttrace.From(ctx)
	if rec == nil || filter == nil || out == nil {
		return
	}
	if !filter.IsGuardrail() {
		if out.Block {
			recordGatewayEvent(ctx, agenttrace.GatewayEvent{Type: agenttrace.GatewayEventFilter, Outcome: agenttrace.OutcomeBlock, Name: filter.Name})
		}
		return
	}
	outcome, detail, ok := guardrailOutcome(out)
	if !ok {
		return
	}
	recordGatewayEvent(ctx, agenttrace.GatewayEvent{Type: agenttrace.GatewayEventGuardrail, Outcome: outcome, Name: filter.Name, Detail: detail})
}

// guardrailOutcome summarises a guardrail's run from its detection events:
// the strongest action taken (block, then quarantine or redact, then log or
// alert), and a detail such as "injection.instruction_override in
// tool_result (search_kb); quarantine". ok is false when no detector matched.
func guardrailOutcome(out *scripting.ScriptOutput) (outcome, detail string, ok bool) {
	var detectors, kinds, tools []string
	strongest := ""
	for _, ce := range out.ComplianceEvents {
		det, _ := ce.Metadata["detector"].(string)
		action, _ := ce.Metadata["action"].(string)
		if det == "" || action == "" {
			continue // judge verdicts and errors
		}
		detectors = append(detectors, det)
		where := metaStrings(ce.Metadata["kinds"])
		if len(where) == 0 {
			where = metaStrings(ce.Metadata["roles"])
		}
		kinds = append(kinds, where...)
		tools = append(tools, metaStrings(ce.Metadata["tool_names"])...)
		if actionRank(action) > actionRank(strongest) {
			strongest = action
		}
	}
	if out.Block {
		strongest = guardrails.ActionBlock
	}
	if strongest == "" {
		return "", "", false
	}
	switch strongest {
	case guardrails.ActionBlock:
		outcome = agenttrace.OutcomeBlock
	case guardrails.ActionQuarantine, guardrails.ActionRedact:
		outcome = agenttrace.OutcomeModify
	default:
		outcome = agenttrace.OutcomeAllow // logged or alerted: the request went on as it was
	}
	if len(detectors) == 0 {
		return outcome, strongest, true // a block with no finding (fail closed)
	}
	var b strings.Builder
	b.WriteString(strings.Join(uniqueSorted(detectors), ", "))
	if k := uniqueSorted(kinds); len(k) > 0 {
		b.WriteString(" in " + strings.Join(k, ", "))
	}
	if t := uniqueSorted(tools); len(t) > 0 {
		b.WriteString(" (" + strings.Join(t, ", ") + ")")
	}
	fmt.Fprintf(&b, "; %s", strongest)
	return outcome, b.String(), true
}

func actionRank(action string) int {
	switch action {
	case guardrails.ActionBlock:
		return 4
	case guardrails.ActionQuarantine:
		return 3
	case guardrails.ActionRedact:
		return 2
	case "":
		return 0
	}
	return 1 // log, alert
}

// metaStrings reads a compliance event metadata list ([]string in process,
// []any after a JSON round trip).
func metaStrings(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, x := range s {
			if str, ok := x.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func uniqueSorted(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
