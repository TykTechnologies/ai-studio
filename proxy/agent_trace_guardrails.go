package proxy

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/requestid"
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
			noteRequestID(ctx, rec, out)
		}
		return
	}
	outcome, detail, ok := guardrailOutcome(out)
	if !ok {
		return
	}
	recordGatewayEvent(ctx, agenttrace.GatewayEvent{Type: agenttrace.GatewayEventGuardrail, Outcome: outcome, Name: filter.Name, Detail: detail})
	noteRequestID(ctx, rec, out)
}

// noteRequestID puts the gateway's request id on the capture (tkg.2), so a
// filter outcome on the trace leads to its compliance events, which record
// the same id as metadata.request_id. It is the id the events carry; a
// filter that reported none falls back to the request's context. It never
// mints one: an id nothing recorded would link to nothing.
func noteRequestID(ctx context.Context, rec *agenttrace.Recorder, out *scripting.ScriptOutput) {
	id := ""
	for _, ce := range out.ComplianceEvents {
		if s, _ := ce.Metadata["request_id"].(string); requestid.Valid(s) {
			id = s
			break
		}
	}
	if id == "" {
		id = requestid.FromContext(ctx)
	}
	if !requestid.Valid(id) {
		return
	}
	rec.Update(func(ev *agenttrace.Event) {
		if ev.Attributes == nil {
			ev.Attributes = map[string]string{}
		}
		ev.Attributes[agenttrace.AttrRequestID] = id
	})
}

// guardrailOutcome summarises a guardrail's run from its detection events:
// the strongest action taken (block, then quarantine, removal or redaction,
// then log or alert), and a detail per action, strongest first, such as
// "injection.instruction_override in tool_result (search_kb); quarantine" or,
// when one run quarantined a result and removed a definition (o4a.2),
// "tool_metadata.poisoning in tool_result (search_kb); quarantine;
// tool_metadata.poisoning in tool_definition (lookup_weather); remove".
// ok is false when no detector matched.
func guardrailOutcome(out *scripting.ScriptOutput) (outcome, detail string, ok bool) {
	type part struct{ detectors, kinds, tools []string }
	parts := map[string]*part{}
	strongest := ""
	for _, ce := range out.ComplianceEvents {
		det, _ := ce.Metadata["detector"].(string)
		action, _ := ce.Metadata["action"].(string)
		if det == "" || action == "" {
			continue // judge verdicts and errors
		}
		p := parts[action]
		if p == nil {
			p = &part{}
			parts[action] = p
		}
		p.detectors = append(p.detectors, det)
		where := metaStrings(ce.Metadata["kinds"])
		if len(where) == 0 {
			where = metaStrings(ce.Metadata["roles"])
		}
		p.kinds = append(p.kinds, where...)
		p.tools = append(p.tools, metaStrings(ce.Metadata["tool_names"])...)
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
	case guardrails.ActionQuarantine, guardrails.ActionRemove, guardrails.ActionRedact:
		outcome = agenttrace.OutcomeModify
	default:
		outcome = agenttrace.OutcomeAllow // logged or alerted: the request went on as it was
	}
	if len(parts) == 0 {
		return outcome, strongest, true // a block with no finding (fail closed)
	}
	actions := make([]string, 0, len(parts))
	for a := range parts {
		actions = append(actions, a)
	}
	sort.Slice(actions, func(i, j int) bool {
		if ri, rj := actionRank(actions[i]), actionRank(actions[j]); ri != rj {
			return ri > rj
		}
		return actions[i] < actions[j]
	})
	var b strings.Builder
	for i, a := range actions {
		p := parts[a]
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(strings.Join(uniqueSorted(p.detectors), ", "))
		if k := uniqueSorted(p.kinds); len(k) > 0 {
			b.WriteString(" in " + strings.Join(k, ", "))
		}
		if t := uniqueSorted(p.tools); len(t) > 0 {
			b.WriteString(" (" + strings.Join(t, ", ") + ")")
		}
		fmt.Fprintf(&b, "; %s", a)
	}
	return outcome, b.String(), true
}

func actionRank(action string) int {
	switch action {
	case guardrails.ActionBlock:
		return 4
	case guardrails.ActionQuarantine, guardrails.ActionRemove:
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
