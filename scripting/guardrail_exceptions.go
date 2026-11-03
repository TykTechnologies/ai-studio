package scripting

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// Guardrail exceptions in the guardrails (features/GuardrailExceptions.md).
//
// An approved exception softens the action a filter takes on a finding for
// one app: the filter, the finding's detector ("" = any) and the channel of
// the flagged segment (and, on a tool channel, optionally one tool) must
// match. The action becomes the softer of the filter's and the exception's,
// within the floor for the app's guardrail capability; an exception outside
// the floor now (the capability changed since it was approved) changes
// nothing and is recorded as guardrail.exception_ignored. The action is
// decided per finding, and a segment takes the strongest of its findings', so
// a second detector without an exception still blocks.
//
// Detection is untouched: the provider scores everything, the verdict cache
// is keyed as before, every finding still produces its compliance event
// (tagged with the exception that softened it), and findings that only put
// unscanned text on record are never softened. An app with no exceptions, a
// request with no app (Studio chat, app id 0), and a build with no exception
// source take none of this path.

// GuardrailException and AppExceptions are the runtime view of an app's
// exceptions (in guardrails, so the sources need not import this package).
type (
	GuardrailException = guardrails.Exception
	AppExceptions      = guardrails.AppExceptions
)

// ExceptionSource serves an app's approved guardrail exceptions. Studio sets
// one (services.GuardrailExceptionSource); edge gateways set one from the
// configuration snapshot.
type ExceptionSource interface {
	// AppExceptions returns the app's approved exceptions; nil for none.
	AppExceptions(ctx context.Context, appID uint) *AppExceptions
	// Hit records that an exception applied to a request.
	Hit(id uint)
}

type exceptionSourceHolder struct{ src ExceptionSource }

var exceptionSource atomic.Pointer[exceptionSourceHolder]

// SetExceptionSource sets where the guardrails read an app's exceptions;
// nil turns exceptions off.
func SetExceptionSource(src ExceptionSource) {
	if src == nil {
		exceptionSource.Store(nil)
		return
	}
	exceptionSource.Store(&exceptionSourceHolder{src: src})
}

func currentExceptionSource() ExceptionSource {
	if h := exceptionSource.Load(); h != nil {
		return h.src
	}
	return nil
}

// exceptionPlan is the exceptions of the request's app that are for this
// filter. nil applies none.
type exceptionPlan struct {
	capability guardrails.Capability
	exceptions []GuardrailException
	src        ExceptionSource
}

// planExceptions reads the exceptions of the request's app for the filter;
// nil when there are none (or no app, or no source).
func planExceptions(input *ScriptInput, filterID uint) *exceptionPlan {
	if filterID == 0 {
		return nil
	}
	src := currentExceptionSource()
	if src == nil {
		return nil
	}
	appID := appIDOf(input)
	if appID == 0 {
		return nil
	}
	ctx := input.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	all := src.AppExceptions(ctx, appID)
	if all == nil {
		return nil
	}
	var mine []GuardrailException
	for _, e := range all.Exceptions {
		if e.FilterID == filterID {
			mine = append(mine, e)
		}
	}
	if len(mine) == 0 {
		return nil
	}
	return &exceptionPlan{capability: all.Capability, exceptions: mine, src: src}
}

// channelOf is the exception channel of a segment; "" where exceptions do
// not apply (a response's text, which is the model's, not the app's).
func channelOf(seg guardrails.Segment, isResponse bool) guardrails.Channel {
	switch {
	case seg.IsToolResult():
		return guardrails.ChannelToolResult
	case seg.IsToolCall():
		return guardrails.ChannelToolCall
	case seg.IsToolDefinition():
		return guardrails.ChannelToolDefinition
	case isResponse:
		return ""
	case seg.Role == guardrails.RoleSystem:
		return guardrails.ChannelTextSystem
	}
	return guardrails.ChannelTextUser
}

// match finds the exception for a finding on a segment: the most specific
// one (a named detector over any, a named tool over any).
func (p *exceptionPlan) match(f guardrails.Finding, seg guardrails.Segment, ch guardrails.Channel) *GuardrailException {
	if p == nil || ch == "" || f.Detector == unscannedDetector {
		return nil
	}
	var best *GuardrailException
	bestScore := -1
	for i := range p.exceptions {
		e := &p.exceptions[i]
		if e.Channel != ch {
			continue
		}
		if e.Detector != "" && e.Detector != f.Detector {
			continue
		}
		if e.ToolName != "" && e.ToolName != seg.ToolName {
			continue
		}
		score := 0
		if e.Detector != "" {
			score += 2
		}
		if e.ToolName != "" {
			score++
		}
		if score > bestScore {
			best, bestScore = e, score
		}
	}
	return best
}

// exceptionApplied is an exception that softened a finding's action.
type exceptionApplied struct {
	id       uint
	from, to string
}

// exceptionIgnored is an exception that matched a finding but is outside
// the floor for the app's capability now.
type exceptionIgnored struct {
	exception GuardrailException
	segment   int
	detector  string
}

// soften applies the plan to the action a for finding f on segment si:
// the softened action, recorded in o.
func (p *exceptionPlan) soften(o *outcome, a string, f guardrails.Finding, si int, seg guardrails.Segment, isResponse bool) string {
	ch := channelOf(seg, isResponse)
	e := p.match(f, seg, ch)
	if e == nil {
		return a
	}
	to, ok := guardrails.SoftenedAction(a, e.Action, p.capability, ch)
	if !ok {
		o.ignored = append(o.ignored, exceptionIgnored{exception: *e, segment: si, detector: f.Detector})
		return a
	}
	if o.hits == nil {
		o.hits = map[uint]bool{}
	}
	o.hits[e.ID] = true
	if to != a {
		if o.softened == nil {
			o.softened = map[int]map[string]exceptionApplied{}
		}
		if o.softened[si] == nil {
			o.softened[si] = map[string]exceptionApplied{}
		}
		o.softened[si][f.Detector] = exceptionApplied{id: e.ID, from: a, to: to}
	}
	return to
}

// recordHits reports each exception that applied, once per request.
func (p *exceptionPlan) recordHits(o outcome) {
	if p == nil || p.src == nil {
		return
	}
	ids := make([]uint, 0, len(o.hits))
	for id := range o.hits {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		p.src.Hit(id)
	}
}

// exceptionRef is the event metadata of an exception that softened a
// finding on a segment.
func (o outcome) exceptionRef(si int, detector string) map[string]any {
	e, ok := o.softened[si][detector]
	if !ok {
		return nil
	}
	return map[string]any{"id": e.id, "from": e.from, "to": e.to}
}

// exceptionIgnoredEvents records exceptions the floor no longer allows, one
// event per exception.
func exceptionIgnoredEvents(filterName string, filterID uint, cfg guardrails.Config, capability guardrails.Capability, ignored []exceptionIgnored, segs []guardrails.Segment) []ComplianceEventOutput {
	seen := map[uint]bool{}
	var out []ComplianceEventOutput
	for _, ig := range ignored {
		e := ig.exception
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		floor := guardrails.Floor(capability, e.Channel)
		if floor == "" {
			floor = "none"
		}
		meta := map[string]interface{}{
			"provider":     cfg.Provider,
			"exception_id": e.ID,
			"channel":      string(e.Channel),
			"action":       e.Action,
			"capability":   string(capability.Effective()),
			"floor":        floor,
			"detector":     ig.detector,
		}
		if filterID != 0 {
			meta["filter_id"] = filterID
		}
		if ig.segment >= 0 && ig.segment < len(segs) {
			if name := safeLabel(segs[ig.segment].ToolName); name != "" {
				meta["tool_name"] = name
			}
		}
		out = append(out, ComplianceEventOutput{
			EventType: "guardrail.exception_ignored",
			Severity:  "warning",
			Description: fmt.Sprintf("guardrail %q: exception #%d (%s to %s) was not applied: the app's guardrail capability (%s) allows %s at most on %s",
				filterName, e.ID, e.Channel, e.Action, capability.Effective(), floor, e.Channel),
			Metadata: meta,
		})
	}
	return out
}
