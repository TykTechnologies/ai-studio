package scripting

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/guardrails/segments"
	"github.com/TykTechnologies/midsommar/v2/guardrails/templates"
	"github.com/TykTechnologies/midsommar/v2/metrics"
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
	templateOverCap = "over_cap"
	// templateUnhandled: a provider that reads templates was sent one and
	// did not acknowledge running its backstop; the backstop then ran as a
	// second call.
	templateUnhandled = "backstop_unhandled"
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
	// backstop is the backstop mode for this message: the filter's, or
	// enforce for a match over the exemption cap, whatever the filter
	// says (off included). Under a filter whose action is log, enforcing
	// still only logs.
	backstop string
	// minScore, when set, is the lowest score a backstop finding needs to
	// be enforced: the gateway forced enforce on a filter set to off or log
	// (over the cap). Findings below it follow the filter's own mode.
	minScore float64
	// fullText is the text the template matched and the backstop scores:
	// the message, or for a message in several text blocks the blocks
	// joined with nothing between them (the segment's text joins them with
	// a separator).
	fullText string
	// view is the detection-only view of the blocks joined (selection
	// viewOf) scored as its own residual while the scan runs, and
	// viewText its text; view is -1 for a message in one block.
	view     int
	viewText string
}

// forceBackstop enforces the backstop of a match over the exemption cap.
// On a filter that does not enforce it itself, only findings scoring at
// least the filter's template_backstop_min_score are enforced (modeFor).
func (ts *templatedSegment) forceBackstop(cfg guardrails.Config) {
	if ts.backstop != guardrails.BackstopEnforce {
		ts.minScore = cfg.BackstopMinScore()
	}
	ts.backstop = guardrails.BackstopEnforce
}

// modeFor is the backstop mode for finding f of this message: under a
// forced enforce, a finding scoring below minScore follows the filter's
// own mode (filterMode). A finding without a score (0) always acts.
func (ts *templatedSegment) modeFor(f guardrails.Finding, filterMode string) string {
	if ts.minScore > 0 && f.Score > 0 && f.Score < ts.minScore {
		return filterMode
	}
	return ts.backstop
}

// templateRef is what a compliance event says of a segment's template.
func (p *templatePlan) templateRef(si int) map[string]any {
	if p == nil {
		return nil
	}
	if ts, ok := p.matched[si]; ok {
		ref := map[string]any{"id": ts.match.Template.Ref(), "exempt_share": roundShare(ts.match.ExemptShare())}
		var signals []string
		if ts.match.Mimicry {
			signals = append(signals, "template.mimicry")
		}
		if ts.match.OverCap {
			signals = append(signals, "template.over_cap")
			ref["backstop"] = "enforced"
			if ts.minScore > 0 {
				ref["backstop_min_score"] = ts.minScore
			}
		}
		if signals != nil {
			ref["signals"] = signals
		}
		return ref
	}
	switch {
	case p.mimicry[si]:
		return map[string]any{"signals": []string{"template.mimicry"}}
	case p.missed[si]:
		return map[string]any{"signals": []string{"template.miss"}}
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
		cand, ok := sel.templateCandidate(input, si)
		if !ok {
			continue
		}
		seg := &sel.segments[si]
		out := templates.MatchMessage(ts, templates.Message{
			Role: cand.role, Text: cand.text,
			First: cand.message == first[cand.role], Last: cand.message == last[cand.role],
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
			metrics.RecordGuardrailTemplate(ctx, cfg.Provider, templateMiss)
			continue
		}
		src.Matched(m.Template)
		metrics.RecordGuardrailTemplate(ctx, cfg.Provider, templateHit)
		metrics.RecordGuardrailTemplateExempt(ctx, cfg.Provider, m.ExemptShare())
		matched := &templatedSegment{match: m, residual: templates.NewResidual(seg.Text, cand.inSegment(m.Exempt)), backstop: plan.backstop, fullText: cand.text, view: -1}
		if m.OverCap {
			matched.forceBackstop(cfg)
			metrics.RecordGuardrailTemplate(ctx, cfg.Provider, templateOverCap)
		}
		plan.matched[si] = matched
		if len(m.Exempt) == 0 {
			continue // every constant refused or capped: scored whole, as sent
		}
		exempt := make([]guardrails.Span, len(m.Exempt))
		for i, e := range m.Exempt {
			exempt[i] = guardrails.Span{Start: e.Start, End: e.End}
		}
		seg.Text = matched.residual.Text
		sel.residualView(cand, matched)
		seg.Template = &guardrails.SegmentTemplate{ID: m.Template.Name, Version: m.Template.Version, FullText: cand.text, Exempt: exempt, Backstop: matched.backstop, BackstopMinScore: matched.minScore}
		matched.inCall = caps.Templates && (pieceMax <= 0 || utf8.RuneCountInString(seg.Text) <= pieceMax)
	}
	if !fits {
		return nil
	}
	for range plan.mimicry {
		metrics.RecordGuardrailTemplate(ctx, cfg.Provider, templateMimicry)
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

// templateRole is the role a unit has for template placements: an OpenAI
// developer message (read as user text by the segmenter) is the app's
// instructions, placed with system messages.
func templateRole(u segments.Unit) string {
	if u.Developer {
		return templates.RoleSystem
	}
	return u.Role
}

// tplCandidate is a message a template may match.
type tplCandidate struct {
	// message is its position, role its role for placements.
	message int
	role    string
	// text is what templates match: the segment's text, or for a message
	// in several text blocks the blocks joined with nothing between them,
	// as the app wrote them.
	text string
	// For a message in several blocks: where each block starts in text
	// (joined) and in the segment's text (seg), and the detection-only
	// view holding text; view is -1 otherwise.
	joined, seg []int
	view        int
}

// inSegment maps exempt ranges of c.text onto the segment's text. For a
// message in several blocks a range is cut at each block boundary, so no
// exemption covers the separator between two blocks and a rewrite of the
// residual still splits back into the blocks.
func (c tplCandidate) inSegment(exempt []templates.Span) []templates.Span {
	if c.joined == nil {
		return exempt
	}
	var out []templates.Span
	for _, e := range exempt {
		for i, js := range c.joined {
			je := len(c.text)
			if i+1 < len(c.joined) {
				je = c.joined[i+1]
			}
			lo, hi := max(e.Start, js), min(e.End, je)
			if lo < hi {
				out = append(out, templates.Span{Start: c.seg[i] + lo - js, End: c.seg[i] + hi - js})
			}
		}
	}
	return out
}

// residualView scores the joined-blocks view of a matched message in
// several blocks as its own residual, so the approved constants are not
// scored there either; restore puts its text back.
func (sel *selection) residualView(c tplCandidate, ts *templatedSegment) {
	if c.view < 0 {
		return
	}
	ts.view, ts.viewText = c.view, sel.segments[c.view].Text
	sel.segments[c.view].Text = templates.NewResidual(c.text, ts.match.Exempt).Text
}

// templateCandidate reports whether the segment at si is a message a
// template may match, and what is matched. Only the client's own system and
// user text (an OpenAI developer message counts as system): never a tool
// result, tool call, tool definition or document, never a detection-only
// view. A message the vendor carries in several text blocks (an Anthropic
// system prompt with cache_control on one block) is matched on its blocks
// joined with nothing between them; only in request bodies, where the
// blocks' boundaries are known.
func (sel *selection) templateCandidate(input *ScriptInput, si int) (tplCandidate, bool) {
	seg := sel.segments[si]
	c := tplCandidate{role: seg.Role, text: seg.Text, view: -1}
	if _, view := sel.viewOf[si]; view || seg.Untrusted() || segmentKind(seg) != guardrails.KindText {
		return c, false
	}
	switch sel.mode {
	case modeBody:
		if si >= len(sel.units) {
			return c, false
		}
		u := sel.units[si]
		c.message, c.role = u.Message, templateRole(u)
		offsets, ok := u.LeafOffsets()
		if u.Undecodable || !ok || u.Text != seg.Text {
			return c, false
		}
		if len(offsets) > 1 {
			// Blocks the vendor sets side by side; a unit whose leaves are
			// separate fields has no joined text and is never matched.
			if u.Joined == "" {
				return c, false
			}
			c.text, c.seg = u.Joined, offsets
			at := 0
			for _, l := range u.Leaves {
				c.joined = append(c.joined, at)
				at += len(l.Text)
			}
			for v, parent := range sel.viewOf {
				if parent == si && sel.segments[v].Text == u.Joined {
					c.view = v
				}
			}
		}
	case modeMessages:
		i := seg.Index
		if i < 0 || i >= len(input.Messages) || textParts(input.Messages[i]) != 1 {
			return c, false
		}
		c.message = i
	default:
		if si != 0 {
			return c, false
		}
	}
	if c.role != templates.RoleSystem && c.role != templates.RoleUser {
		return c, false
	}
	return c, true
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
		if ts.view >= 0 {
			sel.segments[ts.view].Text = ts.viewText
		}
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

// acknowledge keeps the in-call backstop only for the segments the provider
// said it ran it on (Verdict.TemplatesHandled). Any other segment sent with
// a template gets the second classification instead, and its Backstop marks
// count as ordinary findings: a classifier that does not read templates
// scores the residual only, and its backstop must not vanish unnoticed.
func (p *templatePlan) acknowledge(job *scanJob, filterName string, res *scanResult) {
	unhandled := 0
	for si, ts := range p.matched {
		if !ts.inCall || res.templatesHandled[si] {
			continue
		}
		if _, skipped := res.unscanned[si]; skipped {
			continue // never scanned: nothing to acknowledge
		}
		ts.inCall = false
		unhandled++
		metrics.RecordGuardrailTemplate(job.ctx, job.cfg.Provider, templateUnhandled)
	}
	if unhandled > 0 && unhandledLogDue(filterName) {
		slog.Warn("guardrail provider did not acknowledge the trusted prompt template it was sent; running the template backstop as a second call",
			"filter", filterName, "provider", job.cfg.Provider, "segments", unhandled)
	}
}

// unhandledLogged is when each filter last logged an unacknowledged
// template, so a misconfigured classifier logs once a minute, not per
// request (the metric counts every one).
var unhandledLogged sync.Map // filter name -> time.Time

func unhandledLogDue(filterName string) bool {
	now := time.Now()
	if last, ok := unhandledLogged.Load(filterName); ok && now.Sub(last.(time.Time)) < time.Minute {
		return false
	}
	unhandledLogged.Store(filterName, now)
	return true
}

// needsSecondBackstop lists the matched segments whose backstop did not run
// in the provider's call.
func (p *templatePlan) needsSecondBackstop(sel *selection) []int {
	var out []int
	for si, ts := range p.matched {
		if len(ts.match.Exempt) > 0 && !ts.inCall && ts.backstop != guardrails.BackstopOff {
			out = append(out, si)
		}
	}
	slices.Sort(out)
	return out
}

// runBackstop classifies the full text of the matched segments that need
// it, as a run of its own (no rewrite), and returns its findings on sel's
// positions, with what it could not scan.
func (g *guardrailRunner) runBackstop(job *scanJob, sel *selection, plan *templatePlan, segs []int, p *preparedGuardrail, input *ScriptInput) scanResult {
	texts := make([]guardrails.Segment, len(segs))
	for k, si := range segs {
		s := sel.segments[si]
		s.Template = nil
		s.Text = plan.matched[si].fullText
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
	if res.err == nil {
		plan.acknowledge(job, g.filter.Name, res)
	}
	backstop := plan.restore(sel, res)
	cfg := job.cfg
	ec := evidenceContext{requestID: requestIDOf(input), clientRequestID: clientRequestIDOf(input), excerptChars: job.excerptChars, excerptStatus: excerptStatus(cfg, input), templates: plan}
	events := plan.mimicryEvents(g.filter.Name, cfg, sel.segments, ec)
	if res.err != nil {
		// A run that failed is decided by the fail mode; the backstop adds
		// nothing to it.
		return events
	}
	enforced := func(si int) bool {
		ts, ok := plan.matched[si]
		return ok && ts.backstop == guardrails.BackstopEnforce
	}
	if segs := plan.needsSecondBackstop(sel); len(segs) > 0 {
		br := g.runBackstop(job, sel, plan, segs, p, input)
		res.latency += br.latency
		backstop = append(backstop, br.findings...)
		// Under enforce, what the backstop could not scan, and its failure,
		// count as for the main scan.
		anyEnforced := false
		for _, si := range segs {
			anyEnforced = anyEnforced || enforced(si)
		}
		for si, why := range br.unscanned {
			if _, ok := res.unscanned[si]; !ok && enforced(si) {
				res.unscanned[si] = why
			}
		}
		if br.err != nil && anyEnforced {
			res.err = br.err
		}
	}
	var enforce, log []guardrails.Finding
	for _, f := range backstop {
		ts, ok := plan.matched[f.SegmentIndex]
		if !ok {
			continue
		}
		switch ts.modeFor(f, plan.backstop) {
		case guardrails.BackstopOff:
			// A provider that reads templates was told off, or the finding
			// is below a forced backstop's minimum on a filter set to off.
		case guardrails.BackstopEnforce:
			enforce = append(enforce, f)
		default:
			log = append(log, f)
		}
	}
	for range backstopOnly(*res, append(slices.Clip(enforce), log...)) {
		metrics.RecordGuardrailTemplate(job.ctx, cfg.Provider, "backstop_only")
	}
	if len(enforce) > 0 {
		applyBackstop(sel, res, enforce)
	}
	if len(log) == 0 {
		return events
	}
	o := outcome{actions: map[int]string{}, overall: guardrails.ActionLog}
	for _, f := range log {
		o.actions[f.SegmentIndex] = guardrails.ActionLog
	}
	logged := complianceEvents(g.filter.Name, cfg, log, sel.segments, o, guardrails.Verdict{}, ec)
	for i := range logged {
		logged[i].Metadata["template_backstop"] = guardrails.BackstopLog
		logged[i].Description += " (template backstop on the full message; logged only)"
	}
	return append(events, logged...)
}

// mimicryEvents records template text where it does not belong (inside a
// slot, the placeholder in a message, a constant copied many times): one
// event, action log, whatever the guardrail found. The text was scored as
// always; the event is for the operator, since it is how an attacker who
// knows the template would try to pass as it.
func (p *templatePlan) mimicryEvents(filterName string, cfg guardrails.Config, segs []guardrails.Segment, ec evidenceContext) []ComplianceEventOutput {
	var idx []int
	for si := range p.mimicry {
		idx = append(idx, si)
	}
	if len(idx) == 0 {
		return nil
	}
	slices.Sort(idx)
	roles := map[string]bool{}
	var refs []map[string]any
	for _, si := range idx {
		seg := segs[si]
		roles[seg.Role] = true
		if len(refs) < maxEvidenceSegments {
			ref := map[string]any{"index": si, "role": seg.Role, "kind": segmentKind(seg), "sha256": guardrails.TextSHA256(seg.Text)}
			if t := p.templateRef(si); t != nil {
				ref["template"] = t
			}
			refs = append(refs, ref)
		}
	}
	metadata := map[string]interface{}{
		"provider": cfg.Provider,
		"signal":   "template.mimicry",
		"action":   guardrails.ActionLog,
		"count":    len(idx),
		"roles":    sortedKeys(roles),
		"segments": refs,
	}
	if ec.requestID != "" {
		metadata["request_id"] = ec.requestID
	}
	if ec.clientRequestID != "" {
		metadata["client_request_id"] = ec.clientRequestID
	}
	return []ComplianceEventOutput{{
		EventType:   "guardrail.template_mimicry",
		Severity:    "warning",
		Description: fmt.Sprintf("guardrail %q (%s): app template text found where the template does not put it (%d message(s)); the text was scored", filterName, cfg.Provider, len(idx)),
		Metadata:    metadata,
	}}
}
