package scripting

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"sync/atomic"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/guardrails/templates"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// Trusted prompt templates in the request guardrails
// (features/TrustedPromptTemplates.md).
//
// A message of an app's request that matches one of the app's approved
// templates is scored as its residual: the slots in order, with a
// placeholder for each exempt constant. Nothing else changes: tool results,
// tool calls, tool definitions and documents are never matched; a message
// that does not match is scored whole; the budgets, the unscanned rules and
// the fail mode apply to the residual as to any text. An app with no
// approved templates, or a build with no template source (edge gateways for
// now), takes none of this path.
//
// The backstop scores the full text as well (the filter's template_backstop):
// in the same call for a provider that reads templates
// (Capabilities.Templates) and is sent the segment whole, else as a second
// classification. What only the full text shows is logged, or under enforce
// treated as the filter's own finding.

// PromptTemplateSource serves an app's approved prompt templates to the
// request guardrails. Studio sets one (services.PromptTemplateSource).
type PromptTemplateSource interface {
	// ApprovedTemplates returns the app's approved templates; nil for none.
	ApprovedTemplates(ctx context.Context, appID uint) []*templates.Template
	// Matched records that a template matched a request.
	Matched(t *templates.Template)
}

type templateSourceHolder struct{ src PromptTemplateSource }

var promptTemplateSource atomic.Pointer[templateSourceHolder]

// SetPromptTemplateSource sets where the guardrails read an app's approved
// templates; nil turns templates off.
func SetPromptTemplateSource(src PromptTemplateSource) {
	if src == nil {
		promptTemplateSource.Store(nil)
		return
	}
	promptTemplateSource.Store(&templateSourceHolder{src: src})
}

func currentTemplateSource() PromptTemplateSource {
	if h := promptTemplateSource.Load(); h != nil {
		return h.src
	}
	return nil
}

// Template signals recorded on a message (compliance events, metrics).
const (
	templateHit     = "hit"
	templateMiss    = "miss"
	templateMimicry = "mimicry"
)

// templatePlan is how a request's segments were matched against the app's
// templates. Positions are in the selection's segments.
type templatePlan struct {
	// matched are the segments a template matched.
	matched map[int]*templatedSegment
	// missed are segments a template's placement fits that none matched;
	// mimicry those with template text where it does not belong.
	missed, mimicry map[int]bool
	backstop        string
}

// templatedSegment is a matched message.
type templatedSegment struct {
	match    *templates.Match
	residual templates.Residual
	// inCall: the provider reads templates and was sent the segment
	// whole, so the backstop ran in its call.
	inCall bool
}

// templateRef is what a compliance event says of a segment's template.
func (p *templatePlan) templateRef(si int) map[string]any {
	if p == nil {
		return nil
	}
	if ts, ok := p.matched[si]; ok {
		ref := map[string]any{"id": ts.match.Template.Ref(), "exempt_share": roundShare(ts.match.ExemptShare())}
		if ts.match.Mimicry {
			ref["signal"] = "template.mimicry"
		}
		return ref
	}
	switch {
	case p.mimicry[si]:
		return map[string]any{"signal": "template.mimicry"}
	case p.missed[si]:
		return map[string]any{"signal": "template.miss"}
	}
	return nil
}

func roundShare(f float64) float64 {
	return float64(int(f*1000+0.5)) / 1000
}

// appIDOf is the request's app, 0 when it has none.
func appIDOf(input *ScriptInput) uint {
	v, ok := input.Context["app_id"]
	if !ok || v == nil {
		return 0
	}
	id, err := strconv.ParseUint(fmt.Sprint(v), 10, 32)
	if err != nil {
		return 0
	}
	return uint(id)
}

// planTemplates matches the request's messages against the app's approved
// templates and replaces each matched message's text in sel with its
// residual. nil when the request has no app, the app no approved templates,
// or no message a template could fit: sel is then untouched.
func planTemplates(input *ScriptInput, sel *selection, cfg guardrails.Config, caps guardrails.Capabilities, pieceMax int) *templatePlan {
	if input.IsResponse || input.Format == FormatToolResult || input.Format == FormatToolCall {
		return nil
	}
	src := currentTemplateSource()
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
	ts := src.ApprovedTemplates(ctx, appID)
	if len(ts) == 0 {
		return nil
	}
	first, last := sel.roleBounds(input)
	plan := &templatePlan{matched: map[int]*templatedSegment{}, missed: map[int]bool{}, mimicry: map[int]bool{}, backstop: cfg.Backstop()}
	fits := false
	for si := range sel.segments {
		msgIndex, ok := sel.templateCandidate(input, si)
		if !ok {
			continue
		}
		seg := &sel.segments[si]
		out := templates.MatchMessage(ts, templates.Message{
			Role: seg.Role, Text: seg.Text,
			First: msgIndex == first[seg.Role], Last: msgIndex == last[seg.Role],
		})
		if !out.Fits {
			continue
		}
		fits = true
		if out.Mimicry {
			plan.mimicry[si] = true
		}
		m := out.Match
		if m == nil {
			plan.missed[si] = true
			continue
		}
		src.Matched(m.Template)
		ts := &templatedSegment{match: m, residual: templates.NewResidual(seg.Text, m.Exempt)}
		plan.matched[si] = ts
		if len(m.Exempt) == 0 {
			continue // every constant refused or capped: scored whole, as sent
		}
		exempt := make([]guardrails.Span, len(m.Exempt))
		for i, e := range m.Exempt {
			exempt[i] = guardrails.Span{Start: e.Start, End: e.End}
		}
		seg.Text = ts.residual.Text
		seg.Template = &guardrails.SegmentTemplate{ID: m.Template.Name, Version: m.Template.Version, FullText: ts.residual.Original, Exempt: exempt, Backstop: plan.backstop}
		ts.inCall = caps.Templates && (pieceMax <= 0 || utf8.RuneCountInString(seg.Text) <= pieceMax)
	}
	if !fits {
		return nil
	}
	return plan
}

// roleBounds is, per role, the first and last message holding text of the
// role in the whole request (not only the messages in the filter's scope),
// by message position.
func (sel *selection) roleBounds(input *ScriptInput) (first, last map[string]int) {
	first, last = map[string]int{}, map[string]int{}
	see := func(role string, msg int) {
		if f, ok := first[role]; !ok || msg < f {
			first[role] = msg
		}
		if l, ok := last[role]; !ok || msg > l {
			last[role] = msg
		}
	}
	switch sel.mode {
	case modeBody:
		for role, b := range sel.roleSpan {
			see(role, b[0])
			see(role, b[1])
		}
	case modeMessages:
		for i, m := range input.Messages {
			see(roleName(m.Role), i)
		}
	default:
		for _, s := range sel.segments {
			see(s.Role, 0)
		}
	}
	return first, last
}

// templateCandidate reports whether the segment at si is a message a
// template may match, and its message position. Only the client's own
// system and user text held in one block: never a tool result, tool call,
// tool definition or document, never a detection-only view, and never a
// message whose text the vendor joins from several blocks (the joined text
// is not what the app sent byte for byte).
func (sel *selection) templateCandidate(input *ScriptInput, si int) (int, bool) {
	seg := sel.segments[si]
	if _, view := sel.viewOf[si]; view || seg.Untrusted() || segmentKind(seg) != guardrails.KindText {
		return 0, false
	}
	if seg.Role != templates.RoleSystem && seg.Role != templates.RoleUser {
		return 0, false
	}
	switch sel.mode {
	case modeBody:
		if si >= len(sel.units) {
			return 0, false
		}
		u := sel.units[si]
		if u.Undecodable || u.Joined != "" || len(u.Leaves) != 1 || u.Leaves[0].Text != seg.Text {
			return 0, false
		}
		return u.Message, true
	case modeMessages:
		i := seg.Index
		if i < 0 || i >= len(input.Messages) || textParts(input.Messages[i]) != 1 {
			return 0, false
		}
		return i, true
	}
	return 0, si == 0
}

// textParts counts a message's text parts.
func textParts(m llms.MessageContent) int {
	n := 0
	for _, p := range m.Parts {
		if _, ok := p.(llms.TextContent); ok {
			n++
		}
	}
	return n
}

// restore puts the matched messages' text back in sel once the residuals
// are scored, and maps what the provider said onto the original: a
// redaction of the residual becomes a redaction of the message (or, when it
// cannot be placed, the segment is unredactable, so the redaction falls to
// a block), and a finding's span moves onto the message. Findings a
// template-aware provider marked as its backstop are taken out and
// returned; the mark is ignored on any segment not sent with a template.
func (p *templatePlan) restore(sel *selection, res *scanResult) []guardrails.Finding {
	var backstop []guardrails.Finding
	kept := res.findings[:0]
	for _, f := range res.findings {
		ts, ok := p.matched[f.SegmentIndex]
		templated := ok && sel.segments[f.SegmentIndex].Template != nil
		if f.Backstop {
			f.Backstop = false
			if templated && ts.inCall {
				// Its evidence is of the residual the call carried; the
				// event names the message itself.
				f.Evidence, f.Span = nil, nil
				backstop = append(backstop, f)
				continue
			}
		}
		if templated && f.Span != nil {
			s := ts.residual.MapSpan(templates.Span{Start: f.Span.Start, End: f.Span.End})
			f.Span = &guardrails.Span{Start: s.Start, End: s.End}
		}
		kept = append(kept, f)
	}
	res.findings = kept
	for si, ts := range p.matched {
		seg := &sel.segments[si]
		if seg.Template == nil {
			continue
		}
		seg.Text, seg.Template = ts.residual.Original, nil
		if r, ok := res.rewritten[si]; ok {
			if orig, ok := ts.residual.Restore(r); ok {
				res.rewritten[si] = orig
			} else {
				delete(res.rewritten, si)
				if sel.unredactable == nil {
					sel.unredactable = map[int]bool{}
				}
				sel.unredactable[si] = true
			}
		}
	}
	return backstop
}

// needsSecondBackstop lists the matched segments whose backstop did not run
// in the provider's call.
func (p *templatePlan) needsSecondBackstop(sel *selection) []int {
	if p.backstop == guardrails.BackstopOff {
		return nil
	}
	var out []int
	for si, ts := range p.matched {
		if len(ts.match.Exempt) > 0 && !ts.inCall {
			out = append(out, si)
		}
	}
	slices.Sort(out)
	return out
}

// runBackstop classifies the full text of the matched segments that need
// it, as a run of its own (no rewrite), and returns its findings on sel's
// positions, with what it could not scan.
func (g *guardrailRunner) runBackstop(job *scanJob, sel *selection, segs []int, p *preparedGuardrail, input *ScriptInput) scanResult {
	texts := make([]guardrails.Segment, len(segs))
	for k, si := range segs {
		s := sel.segments[si]
		s.Template = nil
		texts[k] = s
	}
	bj := &scanJob{
		ctx: job.ctx, cfg: job.cfg, provider: job.provider, caps: job.caps,
		segments: texts, direction: job.direction, meta: job.meta,
		cache:        newVerdictKeyer(job.ctx, p, input, job.direction, false),
		breaker:      job.breaker,
		tenant:       job.tenant,
		excerptChars: job.excerptChars,
	}
	res := bj.scan()
	for i := range res.findings {
		res.findings[i].SegmentIndex = segs[res.findings[i].SegmentIndex]
	}
	unscanned := map[int]string{}
	for k, why := range res.unscanned {
		unscanned[segs[k]] = why
	}
	res.unscanned = unscanned
	failed := map[int]bool{}
	for k := range res.failedTools {
		failed[segs[k]] = true
	}
	res.failedTools = failed
	return res
}

// applyBackstop folds the backstop's findings into the run under enforce: a
// finding the residual also made (same detector, same segment) is already
// acted on; any other is the filter's own finding, and makes the segment
// unredactable, since a redaction of the residual does not reach it. Under
// log nothing changes: the caller records them.
func applyBackstop(sel *selection, res *scanResult, backstop []guardrails.Finding) {
	type hit struct {
		seg      int
		detector string
	}
	own := map[hit]bool{}
	for _, f := range res.findings {
		own[hit{f.SegmentIndex, f.Detector}] = true
	}
	for _, f := range backstop {
		if own[hit{f.SegmentIndex, f.Detector}] {
			continue
		}
		f.Span = nil // full-text offsets; no redaction can use them
		res.findings = append(res.findings, f)
		if sel.unredactable == nil {
			sel.unredactable = map[int]bool{}
		}
		sel.unredactable[f.SegmentIndex] = true
	}
}

// backstopOnly are the backstop findings on segments where the residual
// found nothing: what scoring the residual alone would have missed.
func backstopOnly(res scanResult, backstop []guardrails.Finding) []guardrails.Finding {
	flagged := map[int]bool{}
	for _, f := range res.findings {
		flagged[f.SegmentIndex] = true
	}
	var out []guardrails.Finding
	for _, f := range backstop {
		if !flagged[f.SegmentIndex] {
			out = append(out, f)
		}
	}
	return out
}

// finishTemplates maps the scan of the residuals back onto the request and
// runs the backstop. Under enforce the backstop's findings, failures and
// unscanned text join the run's; under log they are returned as events,
// action log, and change nothing.
func (g *guardrailRunner) finishTemplates(plan *templatePlan, job *scanJob, sel *selection, res *scanResult, p *preparedGuardrail, input *ScriptInput) []ComplianceEventOutput {
	backstop := plan.restore(sel, res)
	if res.err != nil || plan.backstop == guardrails.BackstopOff {
		// A run that failed is decided by the fail mode; the backstop adds
		// nothing to it.
		return nil
	}
	enforce := plan.backstop == guardrails.BackstopEnforce
	if segs := plan.needsSecondBackstop(sel); len(segs) > 0 {
		br := g.runBackstop(job, sel, segs, p, input)
		res.latency += br.latency
		backstop = append(backstop, br.findings...)
		if enforce {
			for si, why := range br.unscanned {
				if _, ok := res.unscanned[si]; !ok {
					res.unscanned[si] = why
				}
			}
			if br.err != nil {
				res.err = br.err
			}
		}
	}
	if len(backstop) == 0 {
		return nil
	}
	if enforce {
		applyBackstop(sel, res, backstop)
		return nil
	}
	cfg := job.cfg
	o := outcome{actions: map[int]string{}, overall: guardrails.ActionLog}
	for _, f := range backstop {
		o.actions[f.SegmentIndex] = guardrails.ActionLog
	}
	ec := evidenceContext{requestID: requestIDOf(input), clientRequestID: clientRequestIDOf(input), excerptChars: job.excerptChars, excerptStatus: excerptStatus(cfg, input)}
	events := complianceEvents(g.filter.Name, cfg, backstop, sel.segments, o, guardrails.Verdict{}, ec)
	for i := range events {
		events[i].Metadata["template_backstop"] = guardrails.BackstopLog
		events[i].Description += " (template backstop on the full message; logged only)"
	}
	return events
}
