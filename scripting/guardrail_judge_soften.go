package scripting

// Soften mode (guardrails.Judge.Mode soften, midsommar-clean-9u1j.14): the
// judge is asked inline, before the verdict is applied, about the segments
// the filter blocks or quarantines, and a benign opinion that covers
// everything flagged in the segment (judgeResult.softens) turns its action
// into the softest the app's capability allows on that channel, never below
// that floor. Where the floor allows nothing (an acting app's tool results,
// a response's text) the judge is not asked inline. A suspicious or attack
// opinion, a timeout, a failed call or an unreadable answer keeps the
// filter's verdict. The request waits for the judge at most its timeout
// (the calls run at once).

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// judgeSoftening is what the judge did to a segment's action.
type judgeSoftening struct {
	verdict, reason string
	from, to        string
}

// appCapabilityOf is the request's app's attested guardrail capability,
// from the capability source, whether or not the app has exceptions;
// unattested (treated as acting) when unknown (midsommar-clean-9u1j.58).
func appCapabilityOf(input *ScriptInput) guardrails.Capability {
	return AppCapabilityFor(input)
}

// softestAllowed is the softest action the capability's floor allows on the
// channel, "" when it allows none (guardrails.CheckException's rule).
func softestAllowed(c guardrails.Capability, ch guardrails.Channel) string {
	if ch == "" {
		return ""
	}
	floor := guardrails.Floor(c, ch)
	if floor == "" {
		return ""
	}
	for _, a := range guardrails.ExceptionActions(ch) {
		if guardrails.ActionRank(a) >= guardrails.ActionRank(floor) {
			return a
		}
	}
	return ""
}

// judgeSoften runs soften mode on the outcome o: it asks the judge, inline,
// about the targets it could soften, softens those it found benign, and
// returns the guardrail.judge events. handled is false when soften mode does
// not apply here (another mode, or nothing it could soften), so the caller
// judges the work in the background as record mode does.
func (g *guardrailRunner) judgeSoften(input *ScriptInput, cfg guardrails.Config, sel selection, o *outcome, svc services.ServiceInterface) (events []ComplianceEventOutput, handled bool) {
	if cfg.Judge.Mode != guardrails.JudgeSoften || cfg.Shadow {
		return nil, false
	}
	w := judgeWorkFor(input, cfg, sel, *o, svc)
	if w == nil {
		return nil, true
	}
	capability := appCapabilityOf(input)
	softTo := make([]string, len(w.targets))
	possible := false
	for i := range w.targets {
		t := &w.targets[i]
		if t.over || t.unscanned {
			continue
		}
		to := softestAllowed(capability, channelOf(sel.segments[t.index], input.IsResponse))
		if to != "" && actionRank[to] < actionRank[t.action] {
			softTo[i], possible = to, true
		}
	}
	if !possible {
		return nil, false
	}
	ctx := input.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	results := w.results(ctx)
	for i := range w.targets {
		t := &w.targets[i]
		if softTo[i] == "" || !results[i].softens() {
			continue
		}
		results[i].softenedTo = softTo[i]
		if o.judged == nil {
			o.judged = map[int]judgeSoftening{}
		}
		o.judged[t.index] = judgeSoftening{verdict: results[i].opinion.Verdict, reason: results[i].opinion.Reason, from: t.action, to: softTo[i]}
		o.actions[t.index] = softTo[i]
	}
	o.overall = ""
	for _, a := range o.actions {
		if actionRank[a] > actionRank[o.overall] {
			o.overall = a
		}
	}
	return w.events(results), true
}
