package scripting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/guardrails/segments"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/lrucache"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// FilterRunner executes one filter, whatever its kind, against the same
// input and with the same output contract as a script. Every call site that
// runs filters goes through NewFilterRunner, so a guardrail filter is
// attached, scoped, streamed, audited and synced to edges exactly like a
// script filter.
type FilterRunner interface {
	RunScript(input *ScriptInput, serviceRef services.ServiceInterface) (*ScriptOutput, error)
}

// NewFilterRunner returns the runner for a filter: the Tengo script runner
// for a script filter, the guardrail runner for a guardrail filter.
func NewFilterRunner(filter *models.Filter) FilterRunner {
	if filter.IsGuardrail() {
		return &guardrailRunner{filter: filter}
	}
	return NewScriptRunner(filter.Script)
}

// guardrailRunner maps a filter input onto provider segments, runs the
// provider, and maps the verdict back onto a block, redact or log decision.
type guardrailRunner struct {
	filter *models.Filter
}

var warnNoProvidersOnce sync.Once

// segmentMode says how a rewrite is handed back: as a rewritten Payload
// (chat messages, tool envelopes, response buffers, and request bodies that
// are not JSON: one text that is the raw input itself), as a rewritten
// request body (proxy requests read by guardrails/segments: each unit's
// rewrite is spliced back into its string leaves, so tool blocks, ids and
// every other field survive) or as a rewritten Messages array (decision,
// Responses and embeddings requests, whose own extractors and reconstructors
// read and rebuild the body).
type segmentMode int

const (
	modePayload segmentMode = iota
	modeBody
	modeMessages
)

// selection is the text a provider sees and how a rewrite goes back.
type selection struct {
	segments []guardrails.Segment
	mode     segmentMode
	// units[i] is the body unit behind segments[i] (modeBody only).
	units []segments.Unit
	// refuse, when set, is why the request cannot be screened at all.
	refuse error
	// viewOf maps an extra, detection-only segment to the segment of its
	// unit: a unit's Joined text, or the decoded view of a tool result's
	// JSON (addView).
	viewOf map[int]int
	// unredactable marks units a view flagged for something their own text
	// did not show, which no rewrite of their text can cover.
	unredactable map[int]bool
	// definitions maps a tool definition segment to its unit, for removal
	// (in messages mode, where units holds no real units).
	definitions map[int]segments.Unit
	// replaceWhole: the raw input is one tool output read as JSON, so a
	// quarantine replaces all of it with the placeholder text.
	replaceWhole bool
	// undecodable are the segments of units holding a text document whose
	// bytes could not be decoded: they count as not scanned.
	undecodable []int
}

// unscannedUndecodable is the unscanned reason of a text document whose bytes
// could not be decoded.
const unscannedUndecodable = "undecodable"

// FormatToolResult marks a ScriptInput whose RawInput is the output of a
// tool the model called (Studio chat), screened by the chat's request
// guardrails before the model sees it. Context tool_name and tool_call_id
// name the call.
const FormatToolResult = "tool_result"

// FormatToolCall marks a ScriptInput whose RawInput is the arguments of a
// tool call the model asked a Studio chat to run, screened by the chat's
// request guardrails that scan tool calls before the call runs. Context
// tool_name and tool_call_id name the call, user_goal and system_prompt its
// goal. The guardrail never rewrites it: block keeps it from running.
const FormatToolCall = "tool_call"

// preparedGuardrail is a filter's config parsed, normalised and with its
// connection references resolved, plus the provider built from it. Preparing
// one means two JSON round trips, a catalogue check and a secret-store read
// per reference, which is too much to repeat on every request for every
// attached guardrail; prepared entries are kept per saved filter for
// preparedTTL, so a rotated secret takes effect within that window.
type preparedGuardrail struct {
	cfg      guardrails.Config
	provider guardrails.Provider
	caps     guardrails.Capabilities
	at       time.Time
	// key is the preparedKey it was prepared under; empty for a filter
	// that is not a saved version.
	key string

	// digest is configDigest's value, computed on first use.
	digestOnce sync.Once
	digest     []byte
}

const (
	preparedTTL = 30 * time.Second
	preparedMax = 1024
)

// prepared is bounded and least-recently-used; concurrent first requests
// for one filter version share a single preparation.
var prepared = lrucache.New[*preparedGuardrail](preparedMax)

// preparedNow is the clock prepared entries age by; tests move it.
var preparedNow = time.Now

// preparedKey identifies a saved filter version and the config it holds. A
// filter without an ID or an update time (the test endpoint's temporary
// filter) is never cached. The config's digest is part of the key because
// the config can change while the version stamp does not: at the edge each
// snapshot carries the hub's freshly resolved secrets under the filter's
// unchanged UpdatedAt. A secret rotated behind a $SECRET/ reference on the
// hub, where the stored config keeps the reference, takes effect when the
// entry expires (preparedTTL).
func preparedKey(f *models.Filter) string {
	if f.ID == 0 || f.UpdatedAt.IsZero() {
		return ""
	}
	b, err := json.Marshal(f.Config)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d@%d:", f.ID, f.UpdatedAt.UnixNano()) + guardrails.CacheDigest(b)
}

// lookupPrepared returns the live entry for key; an expired one is dropped
// so the next preparation replaces it.
func lookupPrepared(key string) (*preparedGuardrail, bool) {
	if key == "" {
		return nil, false
	}
	p, ok := prepared.Get(key)
	if !ok {
		return nil, false
	}
	if preparedNow().Sub(p.at) > preparedTTL {
		prepared.Remove(key)
		return nil, false
	}
	return p, true
}

// prepare parses, normalises and resolves the filter's config and builds
// its provider, from the cache when this filter version was prepared
// recently. Errors are never cached. A failed build still hands back the
// config it got as far as, so the caller can apply the filter's fail mode.
func (g *guardrailRunner) prepare() (*preparedGuardrail, error) {
	key := preparedKey(g.filter)
	if p, ok := lookupPrepared(key); ok {
		return p, nil
	}
	var partial *preparedGuardrail
	build := func() (*preparedGuardrail, error) {
		cfg, err := guardrails.ParseConfig(g.filter.Config)
		if err != nil {
			return nil, fmt.Errorf("guardrail '%s': %w", g.filter.Name, err)
		}
		cfg, err = guardrails.Normalize(cfg, g.filter.ResponseFilter)
		if err != nil {
			return nil, fmt.Errorf("guardrail '%s': %w", g.filter.Name, err)
		}
		// Connection values are stored as $SECRET/ and $ENV/ references.
		// Resolve them here, where the filter runs: on Studio against the
		// secret store, at the edge against the values the hub already
		// resolved into the snapshot (GetValue leaves a non-reference
		// untouched). $ENV/ only resolves names on AI_STUDIO_ENV_REF_ALLOWLIST.
		resolved := make(map[string]string, len(cfg.Connection))
		var unresolved []string
		for k, v := range cfg.Connection {
			resolved[k] = guardrails.ResolveConnectionValue(v)
			if guardrails.IsReference(resolved[k]) {
				unresolved = append(unresolved, "connection."+k)
			}
		}
		cfg.Connection = resolved
		// An unresolved reference is never sent to the provider as a
		// credential: the filter errors and its fail mode applies.
		if len(unresolved) > 0 {
			sort.Strings(unresolved)
			partial = &preparedGuardrail{cfg: cfg}
			return nil, fmt.Errorf("guardrail '%s': %s not resolved (an unknown secret, an $ENV/ variable not listed in %s, or a reference the hub could not resolve)", g.filter.Name, strings.Join(unresolved, ", "), secrets.EnvRefAllowlistVar)
		}

		provider, err := guardrails.NewProvider(cfg)
		if err != nil {
			partial = &preparedGuardrail{cfg: cfg}
			return nil, err
		}
		return &preparedGuardrail{cfg: cfg, provider: provider, caps: provider.Capabilities(), at: preparedNow(), key: key}, nil
	}
	if key == "" {
		p, err := build()
		if err != nil {
			return partial, err
		}
		return p, nil
	}
	p, err := prepared.Do(key, build)
	if err != nil {
		return partial, err
	}
	return p, nil
}

// RunScript runs the guardrail. A panic anywhere in it (preparing the
// provider, reading an attacker-shaped body, applying the outcome) is
// recovered, as scripts' panics are (scripting/run.go): several callers run
// filters on goroutines nothing else protects (the chat session loop, batch
// item workers), where a panic would end the process. A request whose
// screening panicked is blocked whatever the fail mode, since nothing about
// it is known to be clean; a response follows the filter's fail mode.
func (g *guardrailRunner) RunScript(input *ScriptInput, svc services.ServiceInterface) (out *ScriptOutput, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = g.panicked(input, r), nil
		}
	}()
	return g.run(input, svc)
}

// panicked is the output of a guardrail run that panicked. The panic value
// can hold request text, so it is logged at debug level only.
func (g *guardrailRunner) panicked(input *ScriptInput, r any) *ScriptOutput {
	safe.ReportRedacted("guardrail_runner", r)
	slog.Error("guardrail panicked", "filter_name", g.filter.Name)
	provider, _ := g.filter.Config["provider"].(string)
	failMode, _ := g.filter.Config["fail_mode"].(string)
	block := !input.IsResponse || failMode == guardrails.FailClosed

	out := passThrough(input)
	severity := "warning"
	if block {
		severity = "critical"
		out.Block = true
		out.Payload = ""
		out.Message = guardrails.DefaultRequestBlockMessage
		if input.IsResponse {
			out.Message = guardrails.DefaultResponseBlockMsg
		}
	}
	out.ComplianceEvents = []ComplianceEventOutput{{
		EventType:   "guardrail.error",
		Severity:    severity,
		Description: fmt.Sprintf("guardrail %q (%s) could not evaluate: internal error", g.filter.Name, provider),
		Metadata:    map[string]interface{}{"provider": provider, "error_class": "panic", "blocked": block},
	}}
	return out
}

func (g *guardrailRunner) run(input *ScriptInput, _ services.ServiceInterface) (*ScriptOutput, error) {
	p, err := g.prepare()
	if errors.Is(err, guardrails.ErrNoProviders) {
		warnNoProvidersOnce.Do(func() {
			slog.Warn("guardrail filters are an Enterprise feature; attached guardrails are evaluated as pass-through")
		})
		return passThrough(input), nil
	}
	if err != nil {
		if p == nil {
			return nil, err
		}
		failed := g.failed(input, p.cfg, err)
		if p.cfg.Shadow && input.RecordEvents != nil {
			// A shadow filter never changes traffic, even when it cannot
			// run: the failure is recorded and the traffic goes on.
			input.RecordEvents(input.Ctx, g.filter.Name, &ScriptOutput{ComplianceEvents: failed.ComplianceEvents})
			return passThrough(input), nil
		}
		return failed, nil
	}

	if input.IsChunk && !shouldEvaluateChunk(input, p.cfg.EvaluateEvery(p.caps)) {
		return passThrough(input), nil
	}
	if p.cfg.Shadow && input.RecordEvents != nil {
		return g.shadow(input, p), nil
	}
	return g.evaluate(input, p)
}

// evaluate classifies input with the prepared guardrail and maps the verdict
// onto the output: the work of a run once the filter is prepared, inline or
// (for a shadow filter) on the shadow pool.
func (g *guardrailRunner) evaluate(input *ScriptInput, p *preparedGuardrail) (*ScriptOutput, error) {
	cfg, provider, caps := p.cfg, p.provider, p.caps

	sel := buildSegments(input, cfg)
	if sel.refuse != nil {
		return g.refused(input, cfg, sel.refuse), nil
	}
	if !anyText(sel.segments) && len(sel.undecodable) == 0 {
		return passThrough(input), nil
	}

	direction := guardrails.DirectionRequest
	if input.IsResponse {
		direction = guardrails.DirectionResponse
	}
	wantRewrite := (cfg.OnDetect == guardrails.ActionRedact || cfg.ActionFor(guardrails.KindToolResult) == guardrails.ActionRedact) &&
		caps.Redacts && !input.IsResponse

	var redact *guardrails.RedactionSpec
	if wantRewrite {
		redact = &guardrails.RedactionSpec{Style: cfg.Redaction.Style, Placeholder: cfg.Redaction.Placeholder}
	}

	ctx := input.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	job := &scanJob{
		ctx: ctx, cfg: cfg, provider: provider, caps: caps,
		segments: sel.segments, direction: direction, redact: redact, meta: meta(input),
		cache:   newVerdictKeyer(ctx, p, input, direction, redact != nil),
		breaker: breakerFor(p),
		tenant:  tenantScope(input.Context),
	}
	res := job.scan()
	sel.foldViews(&res)
	for _, si := range sel.undecodable {
		if res.unscanned == nil {
			res.unscanned = map[int]string{}
		}
		if _, ok := res.unscanned[si]; !ok {
			res.unscanned[si] = unscannedUndecodable
		}
	}

	if errors.Is(res.err, guardrails.ErrCallBudget) {
		// The caller capped the provider calls (a native batch upload):
		// the request is refused with the reason, whatever the fail mode.
		return nil, res.err
	}
	out := passThrough(input)
	if res.err != nil {
		failed := g.failed(input, cfg, res.err)
		if failed.Block || len(res.unscanned) == 0 {
			return failed, nil
		}
		// Fail open lets the text the provider failed on through, but not
		// text a cap kept from it: that keeps its action.
		out.ComplianceEvents = failed.ComplianceEvents
		res.findings, res.rewritten = nil, nil
	}
	if len(res.findings) == 0 && len(res.unscanned) == 0 && len(res.failedTools) == 0 {
		return out, nil
	}
	verdict := guardrails.Verdict{Latency: res.latency}
	findings := append(slices.Clip(res.findings), res.unscannedFindings()...)
	findings = append(findings, res.failedToolFindings()...)
	rewritten := res.rewritten

	o := decide(input, cfg, sel, findings, rewritten, res.unscanned)
	if len(res.failedTools) > 0 {
		// Fail closed would block the request; only the tool results went
		// unscanned, so they are quarantined instead, whatever the filter's
		// action (never weaker than the block: not log, not redact).
		metrics.RecordGuardrailError(ctx, cfg.Provider, cfg.FailMode)
		o.quarantine(res.failedTools)
		out.ComplianceEvents = append(out.ComplianceEvents, failedToolsEvent(g.filter.Name, cfg, sel.segments, res.failedTools, res.toolErr))
	}
	if o.overall != guardrails.ActionBlock {
		if err := o.apply(out, input, sel, cfg, rewritten); err != nil {
			g.rewriteFailed()
			o.block()
		}
	}

	out.ComplianceEvents = append(out.ComplianceEvents, complianceEvents(g.filter.Name, cfg, res.findings, sel.segments, o, verdict)...)
	out.ComplianceEvents = append(out.ComplianceEvents, unscannedEvents(g.filter.Name, cfg, sel.segments, res.unscanned, o.overall)...)
	if len(o.unredactable) > 0 {
		out.ComplianceEvents = append(out.ComplianceEvents, unredactableEvent(g.filter.Name, cfg, sel.segments, o.unredactable, o.overall))
	}

	switch o.overall {
	case guardrails.ActionBlock:
		out.Block = true
		out.Payload = ""
		out.Messages = nil
		out.Message = cfg.BlockMessage
	case guardrails.ActionQuarantine:
		out.Message = "guardrail quarantined content"
	case guardrails.ActionRemove:
		out.Message = "guardrail removed tool definitions"
	case guardrails.ActionRedact:
		out.Message = "guardrail redacted content"
	}
	return out, nil
}

// rewriteFailed logs a redaction or quarantine that could not be written into
// the body (edits that overlap, as in a tool_result nested in another). The
// content was flagged, so the caller blocks whatever the fail mode; the error
// names body paths, which may hold tool data, so only its presence is logged.
func (g *guardrailRunner) rewriteFailed() {
	slog.Warn("guardrail could not rewrite the request body; blocking it", "filter_name", g.filter.Name)
}

// outcome is what a flagged verdict does to the request: an action for each
// flagged segment, and the strongest of them, which is what the request as a
// whole gets (block wins over everything).
type outcome struct {
	actions map[int]string
	// findings are the findings behind each flagged segment.
	findings map[int][]guardrails.Finding
	overall  string
	// unredactable are the segments a redaction could not be applied to,
	// which were quarantined or blocked instead.
	unredactable []int
}

// actionRank orders actions by strength.
var actionRank = map[string]int{guardrails.ActionLog: 1, guardrails.ActionRedact: 2, guardrails.ActionQuarantine: 3, guardrails.ActionRemove: 3, guardrails.ActionBlock: 4}

// decide picks the action for each flagged segment: the filter's on_detect,
// or for a tool result its tool_results action (block becoming quarantine).
// A redaction that cannot be applied (the segment was not scanned in full,
// or the rewrite cannot be mapped onto the body: conditions whoever wrote the
// text can arrange) never falls to the fail mode: a tool result is
// quarantined instead and anything else is blocked. On the response side
// quarantine is block and redact is log, as responses are block-only.
func decide(input *ScriptInput, cfg guardrails.Config, sel selection, findings []guardrails.Finding, rewritten map[int]string, unscanned map[int]string) outcome {
	o := outcome{actions: map[int]string{}, findings: sel.flaggedSegments(findings)}
	instead := func(seg guardrails.Segment) string {
		if seg.IsToolResult() {
			return guardrails.ActionQuarantine
		}
		return guardrails.ActionBlock
	}
	for si := range o.findings {
		seg := sel.segments[si]
		kind := seg.Kind
		if seg.IsToolResult() {
			kind = guardrails.KindToolResult
		}
		a := cfg.ActionFor(kind)
		if input.IsResponse {
			switch a {
			case guardrails.ActionQuarantine:
				a = guardrails.ActionBlock
			case guardrails.ActionRedact:
				a = guardrails.ActionLog
			}
		}
		if a == guardrails.ActionRedact {
			text, rewrote := rewritten[si]
			_, partial := unscanned[si]
			switch {
			case partial:
				a = instead(seg)
			case sel.unredactable[si]:
				a = instead(seg)
				o.unredactable = append(o.unredactable, si)
			case !rewrote:
				// The provider found something but did not rewrite it:
				// nothing to apply, record the finding.
				a = guardrails.ActionLog
			case sel.mode == modeBody:
				if _, ok := sel.units[si].MapRewrite(text); !ok {
					a = instead(seg)
					o.unredactable = append(o.unredactable, si)
				}
			}
		}
		o.actions[si] = a
		if actionRank[a] > actionRank[o.overall] {
			o.overall = a
		}
	}
	sort.Ints(o.unredactable)
	return o
}

// block turns the outcome into a block of the whole request.
func (o *outcome) block() {
	o.overall = guardrails.ActionBlock
}

// quarantine sets the action of the segments to quarantine, whatever decide
// chose for them.
func (o *outcome) quarantine(segs map[int]bool) {
	for si := range segs {
		o.actions[si] = guardrails.ActionQuarantine
	}
	if actionRank[guardrails.ActionQuarantine] > actionRank[o.overall] {
		o.overall = guardrails.ActionQuarantine
	}
}

// apply writes the redactions and quarantines into out, in the form the
// input takes. A segment that cannot be quarantined in place blocks the
// request; an error means the edits could not be written into the body.
func (o *outcome) apply(out *ScriptOutput, input *ScriptInput, sel selection, cfg guardrails.Config, rewritten map[int]string) error {
	texts := map[int]string{}
	var removals []segments.Unit
	for si, a := range o.actions {
		switch a {
		case guardrails.ActionRedact:
			texts[si] = rewritten[si]
		case guardrails.ActionQuarantine:
			texts[si] = renderQuarantine(*cfg.Quarantine.Placeholder, sel.segments[si], o.findings[si], input)
		case guardrails.ActionRemove:
			u, ok := sel.definitions[si]
			if !ok {
				o.block()
				return nil
			}
			removals = append(removals, u)
		}
	}
	if len(texts) == 0 && len(removals) == 0 {
		return nil
	}
	var removalEdits []segments.Edit
	if len(removals) > 0 {
		var err error
		if removalEdits, err = segments.RemovalEdits(input.Format, []byte(input.RawInput), removals); err != nil {
			// The tools array would be left empty, or the request forces a
			// removed tool: refuse the request rather than send it broken.
			o.block()
			return nil
		}
	}
	if sel.mode != modeBody {
		if len(removals) == 0 {
			return applyRewrites(out, input, sel, texts)
		}
		if len(texts) > 0 {
			// Message rewrites go back through the format's reconstructor,
			// which rebuilds the body from the messages and would lose the
			// removal: both at once cannot be written.
			o.block()
			return nil
		}
		body, err := segments.Apply([]byte(input.RawInput), removalEdits)
		if err != nil {
			return err
		}
		out.Payload, out.Messages = string(body), nil
		return nil
	}
	if sel.replaceWhole {
		for si, text := range texts {
			if o.actions[si] == guardrails.ActionQuarantine {
				out.Payload = text
				return nil
			}
		}
	}
	var edits []segments.Edit
	for si, text := range texts {
		var unitEdits []segments.Edit
		ok := true
		if o.actions[si] == guardrails.ActionQuarantine {
			unitEdits, ok = sel.units[si].Quarantine(text)
		} else {
			unitEdits, ok = sel.units[si].MapRewrite(text)
		}
		if !ok {
			o.block()
			return nil
		}
		edits = append(edits, unitEdits...)
	}
	edits = append(edits, removalEdits...)
	body, err := segments.Apply([]byte(input.RawInput), edits)
	if err != nil {
		return err
	}
	out.Payload = string(body)
	return nil
}

// unredactableEvent records the units a redaction could not be applied to and
// what was done instead (action: block, or redact with those tool results
// quarantined). Like every guardrail event it carries no text.
func unredactableEvent(filterName string, cfg guardrails.Config, segs []guardrails.Segment, units []int, action string) ComplianceEventOutput {
	roles, tools := map[string]bool{}, map[string]bool{}
	for _, si := range units {
		roles[segs[si].Role] = true
		if name := safeLabel(segs[si].ToolName); name != "" {
			tools[name] = true
		}
	}
	instead := "the tool results were quarantined"
	severity := "warning"
	if action == guardrails.ActionBlock {
		instead, severity = "the request was blocked", "critical"
	}
	return ComplianceEventOutput{
		EventType:   "guardrail.unredactable",
		Severity:    severity,
		Description: fmt.Sprintf("guardrail %q (%s): a redaction could not be applied in place (%d); %s", filterName, cfg.Provider, len(units), instead),
		Metadata: map[string]interface{}{
			"provider":   cfg.Provider,
			"count":      len(units),
			"action":     action,
			"roles":      sortedKeys(roles),
			"tool_names": sortedKeys(tools),
		},
	}
}

// flaggedSegments groups findings by the segment they flag. A finding that
// names no segment the provider was given (a request-level verdict, an index
// out of range), or a flagged verdict with no findings, flags every segment,
// so a verdict never ends as "flagged, nothing done". Views of units are
// never flagged themselves (foldViews has moved their findings).
func (sel selection) flaggedSegments(findings []guardrails.Finding) map[int][]guardrails.Finding {
	segs := sel.segments
	flagged := map[int][]guardrails.Finding{}
	var unplaced []guardrails.Finding
	for _, f := range findings {
		if f.SegmentIndex >= 0 && f.SegmentIndex < len(segs) {
			flagged[f.SegmentIndex] = append(flagged[f.SegmentIndex], f)
		} else {
			unplaced = append(unplaced, f)
		}
	}
	if len(unplaced) > 0 || len(findings) == 0 {
		for si := range segs {
			if _, joined := sel.viewOf[si]; !joined {
				flagged[si] = append(flagged[si], unplaced...)
			}
		}
	}
	return flagged
}

// renderQuarantine fills the placeholder template for one segment. Every
// value comes from a client or a provider, so each is reduced to a plain
// label before it is inserted.
func renderQuarantine(template string, seg guardrails.Segment, findings []guardrails.Finding, input *ScriptInput) string {
	var dets, cats []string
	seen := map[string]bool{}
	for _, f := range findings {
		if d := safeLabel(f.Detector); d != "" && !seen["d:"+d] {
			seen["d:"+d] = true
			dets = append(dets, d)
		}
		if c := safeLabel(f.Category); c != "" && !seen["c:"+c] {
			seen["c:"+c] = true
			cats = append(cats, c)
		}
	}
	sort.Strings(dets)
	sort.Strings(cats)
	detector := strings.Join(dets, ", ")
	if detector == "" {
		detector = "content"
	}

	toolName := seg.ToolName
	if toolName == "" && seg.Role == guardrails.RoleTool {
		toolName, _ = input.Context["tool_name"].(string)
	}
	toolName = safeLabel(toolName)
	source := map[string]string{
		guardrails.RoleSystem:    "system prompt",
		guardrails.RoleAssistant: "assistant message",
		guardrails.RoleTool:      "tool result",
	}[seg.Role]
	if source == "" {
		source = "user message"
	}
	if seg.Role == guardrails.RoleTool && toolName != "" {
		source += " from " + toolName
	}
	if seg.IsToolDefinition() {
		source = "tool definition"
		if toolName != "" {
			source += " of " + toolName
		}
	}
	if seg.IsToolCall() {
		source = "tool call"
		if toolName != "" {
			source += " to " + toolName
		}
	}

	return strings.NewReplacer(
		"{{detector}}", detector,
		"{{category}}", strings.Join(cats, ", "),
		"{{source}}", source,
		"{{tool_name}}", toolName,
	).Replace(template)
}

// safeLabel keeps letters, digits and _ . : - of s, up to 64 characters, so
// a tool name or detector id chosen by a client or a remote provider cannot
// carry instructions into text the model reads, or markup into the logs.
func safeLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= 64 {
			break
		}
		if r < utf8.RuneSelf && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_.:-", r)) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// failed applies the fail mode to a provider error. Neither branch returns
// an error to the call site: a closed guardrail blocks with the configured
// message, an open one lets the text through, and both record what happened.
func (g *guardrailRunner) failed(input *ScriptInput, cfg guardrails.Config, err error) *ScriptOutput {
	ctx := input.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	metrics.RecordGuardrailError(ctx, cfg.Provider, cfg.FailMode)
	slog.Error("guardrail provider call failed",
		"filter_name", g.filter.Name, "provider", cfg.Provider, "fail_mode", cfg.FailMode, "error", err)

	out := passThrough(input)
	severity := "warning"
	if cfg.FailMode == guardrails.FailClosed {
		severity = "critical"
		out.Block = true
		out.Payload = ""
		out.Message = cfg.BlockMessage
	}
	out.ComplianceEvents = []ComplianceEventOutput{{
		EventType:   "guardrail.error",
		Severity:    severity,
		Description: fmt.Sprintf("guardrail %q (%s) could not evaluate: %v; fail mode %s", g.filter.Name, cfg.Provider, truncate(err.Error(), 200), cfg.FailMode),
		Metadata: map[string]interface{}{
			"provider":  cfg.Provider,
			"fail_mode": cfg.FailMode,
			"blocked":   cfg.FailMode == guardrails.FailClosed,
		},
	}}
	return out
}

// refused blocks a request the guardrail cannot screen because its body
// reads differently to different parsers (a repeated or case-variant key):
// whichever copy was scanned, the vendor may read the other. This holds
// whatever the filter's action and fail mode, as the native API paths refuse
// such bodies.
func (g *guardrailRunner) refused(input *ScriptInput, cfg guardrails.Config, reason error) *ScriptOutput {
	out := passThrough(input)
	out.Block = true
	out.Payload = ""
	out.Message = "the request has duplicate or case-variant JSON keys and cannot be screened"
	out.ComplianceEvents = []ComplianceEventOutput{{
		EventType:   "guardrail.unreadable_request",
		Severity:    "critical",
		Description: fmt.Sprintf("guardrail %q (%s) refused a request it cannot screen: %s", g.filter.Name, cfg.Provider, truncate(reason.Error(), 200)),
		Metadata:    map[string]interface{}{"provider": cfg.Provider, "blocked": true},
	}}
	return out
}

// passThrough is the "nothing to do" output: not blocked, content unchanged.
func passThrough(input *ScriptInput) *ScriptOutput {
	return &ScriptOutput{Block: false, Payload: input.RawInput}
}

// shouldEvaluateChunk implements the streaming cadence without per-stream
// state: the provider runs each time the accumulated buffer crosses a
// multiple of every characters. RawInput on a chunk is the text delta the
// chunk added, so the previous length is the buffer minus the delta.
func shouldEvaluateChunk(input *ScriptInput, every int) bool {
	if input.IsFinal {
		// End of stream: always look at the whole response once, so a
		// response shorter than the cadence is not skipped.
		return strings.TrimSpace(input.CurrentBuffer) != ""
	}
	if every <= 0 {
		return true
	}
	cur := utf8.RuneCountInString(input.CurrentBuffer)
	delta := utf8.RuneCountInString(input.RawInput)
	if delta == 0 {
		return false
	}
	prev := cur - delta
	if prev < 0 {
		prev = 0
	}
	return cur/every > prev/every
}

// buildSegments picks the text the provider sees.
func buildSegments(input *ScriptInput, cfg guardrails.Config) selection {
	if input.IsResponse {
		text := input.RawInput
		if input.IsChunk {
			text = input.CurrentBuffer
		}
		sel := payloadSelection("assistant", text)
		if cfg.ScansToolCalls() && !input.IsChunk && input.ResponseBody != "" {
			sel.addResponseToolCalls(input, cfg)
		}
		return sel
	}

	if input.Format == FormatToolResult {
		return toolOutputSelection(input, cfg)
	}
	if input.Format == FormatToolCall {
		return toolCallSelection(input, cfg)
	}

	// Proxy requests: read the body block by block, unless the format has
	// its own extractor that found the text (decisions, Responses,
	// embeddings), whose reconstructor writes a rewrite back.
	if input.Format != "" && (segments.Supports(input.Format) || !anyMessageText(input.Messages)) {
		units, err := segments.ReadWith(input.Format, []byte(input.RawInput), segments.Options{ToolCalls: cfg.ScansToolCalls(), ToolDefinitions: cfg.ScansToolDefinitions()})
		switch {
		case errors.Is(err, segments.ErrDuplicateKey):
			return selection{refuse: err}
		case err != nil:
			// Not JSON: classify the bytes as they are.
			return payloadSelection("user", input.RawInput)
		}
		return bodySelection(units, cfg)
	}

	// Chat messages and tool envelopes arrive as one message whose content is
	// the raw input itself; a rewrite goes back as the payload.
	if len(input.Messages) == 1 && messageText(input.Messages[0]) == input.RawInput {
		return payloadSelection(roleName(input.Messages[0].Role), input.RawInput)
	}
	if len(input.Messages) == 0 || !anyMessageText(input.Messages) {
		// No extracted messages (an unknown body shape, or the test endpoint
		// handing over a body it could not map): classify the raw input.
		return payloadSelection("user", input.RawInput)
	}

	// Proxy requests in other formats (decisions, Responses, embeddings):
	// the extracted messages, filtered by scope. Index is the position in
	// input.Messages so a rewrite lands on the right message.
	cands := make([]candidate, len(input.Messages))
	named := len(input.MessageTools) == len(input.Messages)
	scanCalls := cfg.ScansToolCalls() && named
	turn := 0
	for i, m := range input.Messages {
		role := roleName(m.Role)
		if role == "function" {
			role = guardrails.RoleTool
		}
		cands[i] = candidate{role: role, message: i, turn: turn}
		switch {
		case role == guardrails.RoleTool:
			cands[i].kind = guardrails.KindToolResult
		case scanCalls && role == guardrails.RoleAssistant && input.MessageTools[i].Call:
			// A tool call's arguments (Responses function_call): without
			// tool-call scanning they stay assistant text, as before.
			cands[i].kind = guardrails.KindToolCall
		}
		if role == guardrails.RoleAssistant {
			turn++
		}
	}
	sel := selection{mode: modeMessages}
	var system []string
	lastUser := ""
	for i, in := range selectInScope(cands, cfg) {
		c := cands[i]
		text := messageText(input.Messages[i])
		switch {
		case c.role == guardrails.RoleSystem && text != "":
			system = append(system, text)
		case c.role == guardrails.RoleUser && strings.TrimSpace(text) != "":
			lastUser = text
		}
		if !in {
			continue
		}
		seg := guardrails.Segment{Index: i, Role: c.role, Kind: c.kind, Text: text}
		if named && c.kind != "" {
			seg.ToolName, seg.ToolCallID = input.MessageTools[i].Name, input.MessageTools[i].CallID
		}
		if c.kind == guardrails.KindToolCall {
			seg.Context = goalContext(cfg, guardrails.SegmentContext{UserGoal: lastUser, System: strings.Join(system, "\n\n")})
		}
		pos := len(sel.segments)
		sel.segments = append(sel.segments, seg)
		if c.kind == guardrails.KindToolCall {
			// Index is the message's position here, not the segment's.
			sel.addViewAt(pos, seg, segments.Unit{Segment: seg}, decodedJSON(text))
		}
	}
	if cfg.ScansToolDefinitions() && input.Format == segments.FormatOpenAIResponses {
		// The Responses extractor lists no tools; they are read from the
		// body, and a removal is written into it.
		if units, err := segments.ResponsesToolDefinitions([]byte(input.RawInput)); err == nil {
			for _, u := range units {
				pos := len(sel.segments)
				seg := u.Segment
				seg.Index = pos
				sel.segments = append(sel.segments, seg)
				if sel.definitions == nil {
					sel.definitions = map[int]segments.Unit{}
				}
				sel.definitions[pos] = u
				sel.addViewAt(pos, seg, u, decodedJSON(seg.Text))
			}
		}
	}
	return sel
}

// goalContext is the context sent with a tool call: the user's goal, and the
// system prompt only when the filter includes it, each clipped.
func goalContext(cfg guardrails.Config, c guardrails.SegmentContext) *guardrails.SegmentContext {
	out := guardrails.SegmentContext{UserGoal: guardrails.ClipContext(c.UserGoal)}
	if cfg.ToolCalls.IncludeSystem {
		out.System = guardrails.ClipContext(c.System)
	}
	return &out
}

// decodedJSON is segments.JSONView of text, or "" when it has none.
func decodedJSON(text string) string {
	v, _ := segments.JSONView(text)
	return v
}

// addResponseToolCalls adds the tool calls of a buffered response
// (input.ResponseBody, in input.ResponseFormat) as tool call segments, each
// with the goal of the request that produced it (input.RequestBody), and
// the decoded view of its arguments. A body that cannot be read adds none:
// its text is screened as before.
func (sel *selection) addResponseToolCalls(input *ScriptInput, cfg guardrails.Config) {
	units, err := segments.ResponseToolCalls(input.ResponseFormat, []byte(input.ResponseBody))
	if err != nil || len(units) == 0 {
		return
	}
	goal := goalContext(cfg, segments.Goal(input.ResponseFormat, []byte(input.RequestBody)))
	for _, u := range units {
		seg := u.Segment
		seg.Index = len(sel.segments)
		seg.Context = goal
		sel.segments = append(sel.segments, seg)
		sel.addView(seg, u, decodedJSON(seg.Text))
	}
}

// bodySelection picks the units the guardrail scans (selectInScope).
func bodySelection(units []segments.Unit, cfg guardrails.Config) selection {
	cands := make([]candidate, len(units))
	for i, u := range units {
		cands[i] = candidate{role: u.Role, kind: u.Kind, message: u.Message, turn: u.Turn}
	}
	in := selectInScope(cands, cfg)
	sel := selection{mode: modeBody}
	for i, u := range units {
		if in[i] {
			// The unit's tool metadata stays here for compliance events and
			// placeholders; providerSegment decides what providers see.
			seg := u.Segment
			seg.Index = len(sel.segments)
			if seg.IsToolCall() && seg.Context != nil {
				seg.Context = goalContext(cfg, *seg.Context)
			}
			if u.Undecodable {
				sel.undecodable = append(sel.undecodable, seg.Index)
			}
			sel.segments = append(sel.segments, seg)
			sel.units = append(sel.units, u)
			// The vendor may set the unit's blocks side by side, so a word
			// split across two of them is classified whole too.
			sel.addView(seg, u, u.Joined)
			sel.addDecodedViews(seg, u)
		}
	}
	return sel
}

// addView adds text as a detection-only view of the unit behind seg, when
// there is one.
func (sel *selection) addView(seg guardrails.Segment, u segments.Unit, text string) {
	sel.addViewAt(seg.Index, seg, u, text)
}

// addViewAt is addView for the segment at position parent, whose Index is
// not its position (messages mode, where Index is the message's).
func (sel *selection) addViewAt(parent int, seg guardrails.Segment, u segments.Unit, text string) {
	if text == "" {
		return
	}
	if sel.viewOf == nil {
		sel.viewOf = map[int]int{}
	}
	view := seg
	view.Text = text
	if sel.mode != modeMessages {
		view.Index = len(sel.segments)
	}
	sel.viewOf[len(sel.segments)] = parent
	sel.segments = append(sel.segments, view)
	sel.units = append(sel.units, u)
}

// addDecodedViews adds the decoded view of a tool result whose text is JSON,
// or holds JSON escapes (segments.JSONView): a pattern over the raw text
// would see the escapes the model reads through.
func (sel *selection) addDecodedViews(seg guardrails.Segment, u segments.Unit) {
	if seg.Kind == guardrails.KindToolDefinition {
		if sel.definitions == nil {
			sel.definitions = map[int]segments.Unit{}
		}
		sel.definitions[seg.Index] = u
		sel.addView(seg, u, decodedJSON(seg.Text))
		return
	}
	if seg.Kind == guardrails.KindToolCall {
		// Arguments held as an object have no leaves; their raw JSON is
		// the text.
		sel.addView(seg, u, decodedJSON(seg.Text))
		return
	}
	if seg.Kind != guardrails.KindToolResult {
		return
	}
	var views []string
	for _, l := range u.Leaves {
		if v, ok := segments.JSONView(l.Text); ok {
			views = append(views, v)
		}
	}
	sel.addView(seg, u, strings.Join(views, "\n"))
}

// foldViews moves what the provider reported on view segments onto their
// units' own segments. A finding the unit's own text also has is a
// duplicate and is dropped; one only a view has (a phrase split inside a
// word, an injection behind JSON escapes) is kept, and marks the unit
// unredactable, since a rewrite of a view cannot be mapped onto the body.
// Rewrites of views are never used.
func (sel *selection) foldViews(res *scanResult) {
	if len(sel.viewOf) == 0 {
		return
	}
	type hit struct {
		seg      int
		detector string
	}
	own := map[hit]bool{}
	for _, f := range res.findings {
		if _, joined := sel.viewOf[f.SegmentIndex]; !joined {
			own[hit{f.SegmentIndex, f.Detector}] = true
		}
	}
	kept := res.findings[:0]
	for _, f := range res.findings {
		if p, joined := sel.viewOf[f.SegmentIndex]; joined {
			if own[hit{p, f.Detector}] {
				continue
			}
			f.SegmentIndex = p
			if sel.unredactable == nil {
				sel.unredactable = map[int]bool{}
			}
			sel.unredactable[p] = true
		}
		kept = append(kept, f)
	}
	res.findings = kept
	for si := range res.rewritten {
		if _, joined := sel.viewOf[si]; joined {
			delete(res.rewritten, si)
		}
	}
	for si, why := range res.unscanned {
		if p, joined := sel.viewOf[si]; joined {
			delete(res.unscanned, si)
			if _, has := res.unscanned[p]; !has {
				res.unscanned[p] = why
			}
		}
	}
	for si := range res.failedTools {
		if p, joined := sel.viewOf[si]; joined {
			delete(res.failedTools, si)
			res.failedTools[p] = true
		}
	}
}

// toolOutputSelection reads a tool output: as JSON when it is (every string
// decoded, so a JSON escape cannot hide a phrase, and a redaction lands in
// place), else as text. It is one tool result whatever the scope, and is
// not scanned at all when the filter's tool-result scanning is off.
func toolOutputSelection(input *ScriptInput, cfg guardrails.Config) selection {
	if !cfg.ScansToolResults() {
		return selection{}
	}
	meta := guardrails.Segment{Role: guardrails.RoleTool, Kind: guardrails.KindToolResult}
	meta.ToolName, _ = input.Context["tool_name"].(string)
	meta.ToolCallID, _ = input.Context["tool_call_id"].(string)
	units, err := segments.Generic([]byte(input.RawInput))
	if err != nil {
		// Not JSON the gateway could rewrite (or JSON with a repeated key,
		// which the model still reads): classify the text, and its decoded
		// strings when it is JSON at all, every copy of a key included.
		sel := payloadSelection(meta.Role, input.RawInput)
		meta.Text = input.RawInput
		sel.segments[0] = meta
		if view, ok := segments.JSONView(input.RawInput); ok {
			sel.addView(meta, segments.Unit{Segment: meta}, view)
		}
		return sel
	}
	sel := selection{mode: modeBody, replaceWhole: true}
	for _, u := range units {
		meta.Index, meta.Text = len(sel.segments), u.Text
		u.Segment = meta
		sel.segments = append(sel.segments, meta)
		sel.units = append(sel.units, u)
		sel.addDecodedViews(meta, u)
	}
	return sel
}

// toolCallSelection reads a tool call the model asked a Studio chat to run
// (FormatToolCall): one tool call segment with its goal, and the decoded
// view of its arguments. Nothing when the filter does not scan tool calls.
func toolCallSelection(input *ScriptInput, cfg guardrails.Config) selection {
	if !cfg.ScansToolCalls() {
		return selection{}
	}
	seg := guardrails.Segment{Role: guardrails.RoleAssistant, Kind: guardrails.KindToolCall, Text: input.RawInput}
	seg.ToolName, _ = input.Context["tool_name"].(string)
	seg.ToolCallID, _ = input.Context["tool_call_id"].(string)
	goal, _ := input.Context["user_goal"].(string)
	system, _ := input.Context["system_prompt"].(string)
	seg.Context = goalContext(cfg, guardrails.SegmentContext{UserGoal: goal, System: system})
	if strings.TrimSpace(seg.Text) == "" {
		seg.Text = "{}"
	}
	sel := selection{segments: []guardrails.Segment{seg}, mode: modePayload}
	sel.addView(seg, segments.Unit{Segment: seg}, decodedJSON(seg.Text))
	return sel
}

// payloadSelection is a single text whose rewrite is the payload.
func payloadSelection(role, text string) selection {
	return selection{segments: []guardrails.Segment{{Index: 0, Role: role, Text: text}}, mode: modePayload}
}

// candidate is a message or unit the guardrail may scan.
type candidate struct {
	role, kind    string
	message, turn int
}

// selectInScope reports for each candidate whether the guardrail scans it.
//
// With tool-result scanning on (the default), every tool result is scanned
// whatever the scope, across the whole history: a client resends its tool
// results on every turn, so scanning only the current turn would let a
// poisoned result through on the next one. The scope then selects the other
// text: last_user is the user text of the current turn (everything the
// client sent since the model last spoke), all_user every user text,
// system_and_user adds the system prompt, all_messages everything.
//
// With it off, selection is by role as before: tool results only under
// all_messages, and last_user is the user text of the last message that has
// any.
func selectInScope(cands []candidate, cfg guardrails.Config) []bool {
	scanTools := cfg.ScansToolResults()
	lastMessage, currentTurn := -1, -1
	for _, c := range cands {
		if c.role == guardrails.RoleUser {
			lastMessage = max(lastMessage, c.message)
		}
		if c.role != guardrails.RoleAssistant {
			currentTurn = max(currentTurn, c.turn)
		}
	}
	in := make([]bool, len(cands))
	for i, c := range cands {
		tool := c.kind == guardrails.KindToolResult || c.role == guardrails.RoleTool
		switch {
		case c.kind == guardrails.KindToolDefinition:
			in[i] = cfg.ScansToolDefinitions()
		case c.kind == guardrails.KindToolCall:
			// Present only when the filter scans tool calls, across the
			// whole history like tool results.
			in[i] = cfg.ScansToolCalls()
		case tool && scanTools:
			in[i] = true
		case tool:
			in[i] = cfg.Scope == guardrails.ScopeAllMessages
		case cfg.Scope == guardrails.ScopeLastUser:
			current := c.message == lastMessage
			if scanTools {
				current = c.turn == currentTurn
			}
			in[i] = c.role == guardrails.RoleUser && current
		case cfg.Scope == guardrails.ScopeAllUser:
			in[i] = c.role == guardrails.RoleUser
		case cfg.Scope == guardrails.ScopeSystemAndUser:
			in[i] = c.role == guardrails.RoleUser || c.role == guardrails.RoleSystem
		default:
			in[i] = true
		}
	}
	return in
}

func messageText(m llms.MessageContent) string {
	var b strings.Builder
	for _, part := range m.Parts {
		if t, ok := part.(llms.TextContent); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

func roleName(r llms.ChatMessageType) string {
	switch r {
	case llms.ChatMessageTypeSystem:
		return "system"
	case llms.ChatMessageTypeHuman:
		return "user"
	case llms.ChatMessageTypeAI:
		return "assistant"
	case llms.ChatMessageTypeTool:
		return "tool"
	case llms.ChatMessageTypeFunction:
		return "function"
	}
	return "unknown"
}

func anyMessageText(msgs []llms.MessageContent) bool {
	for _, m := range msgs {
		if strings.TrimSpace(messageText(m)) != "" {
			return true
		}
	}
	return false
}

func anyText(segments []guardrails.Segment) bool {
	for _, s := range segments {
		if strings.TrimSpace(s.Text) != "" {
			return true
		}
	}
	return false
}

// applyRewrites writes redacted or replacement text back in the form the
// call site consumes, for payload and messages inputs (outcome.apply edits
// request bodies itself). rewritten is keyed by position in sel.segments.
func applyRewrites(out *ScriptOutput, input *ScriptInput, sel selection, rewritten map[int]string) error {
	switch sel.mode {
	case modePayload:
		if r, ok := rewritten[0]; ok {
			out.Payload = r
		}
		return nil
	case modeBody:
		return errors.New("request bodies are rewritten by outcome.apply")
	}
	// Messages mode: emit every message so the reconstructor rebuilds the
	// full conversation, with rewritten content where the guardrail changed it.
	bySegment := map[int]string{}
	for si, text := range rewritten {
		bySegment[sel.segments[si].Index] = text
	}
	msgs := make([]map[string]interface{}, 0, len(input.Messages))
	for i, m := range input.Messages {
		content := messageText(m)
		if r, ok := bySegment[i]; ok {
			content = r
		}
		msgs = append(msgs, map[string]interface{}{"role": roleName(m.Role), "content": content})
	}
	out.Messages = msgs
	out.Payload = ""
	return nil
}

func meta(input *ScriptInput) map[string]any {
	m := map[string]any{"vendor": input.VendorName, "model": input.ModelName, "is_chat": input.IsChat}
	for k, v := range input.Context {
		switch k {
		case "request_id", "llm_id", "app_id", "user_id", "session_id", "tool_id", "tool_name":
			m[k] = v
		}
	}
	return m
}

// complianceEvents summarises the verdict as one event per detector. The
// events carry counts and roles, never the matched text.
func complianceEvents(filterName string, cfg guardrails.Config, findings []guardrails.Finding, segments []guardrails.Segment, o outcome, verdict guardrails.Verdict) []ComplianceEventOutput {
	type group struct {
		f        guardrails.Finding
		count    int
		maxScore float64
		severity string
		roles    map[string]bool
		tools    map[string]bool
		kinds    map[string]bool
		action   string
	}
	groups := map[string]*group{}
	order := []string{}
	for _, f := range findings {
		g, ok := groups[f.Detector]
		if !ok {
			g = &group{f: f, severity: f.Severity, roles: map[string]bool{}, tools: map[string]bool{}, kinds: map[string]bool{}}
			groups[f.Detector] = g
			order = append(order, f.Detector)
		}
		g.count++
		if f.Score > g.maxScore {
			g.maxScore = f.Score
		}
		if severityRank(f.Severity) > severityRank(g.severity) {
			g.severity = f.Severity
		}
		// The request's outcome where the finding names no segment, else
		// the strongest action applied to the segments it flagged.
		a := o.overall
		if f.SegmentIndex >= 0 && f.SegmentIndex < len(segments) {
			seg := segments[f.SegmentIndex]
			g.roles[seg.Role] = true
			g.kinds[segmentKind(seg)] = true
			if name := safeLabel(seg.ToolName); name != "" {
				g.tools[name] = true
			}
			if sa, ok := o.actions[f.SegmentIndex]; ok && o.overall != guardrails.ActionBlock {
				a = sa
			}
		}
		if actionRank[a] > actionRank[g.action] {
			g.action = a
		}
	}
	sort.Strings(order)

	events := make([]ComplianceEventOutput, 0, len(order))
	for _, det := range order {
		g := groups[det]
		roles := sortedKeys(g.roles)
		tools := sortedKeys(g.tools)
		action := g.action
		label := g.f.Label
		if label == "" {
			label = det
		}
		sev := g.severity
		if sev == "" {
			sev = "warning"
		}
		if action == guardrails.ActionBlock && severityRank(sev) < severityRank("critical") {
			sev = "critical"
		}
		metadata := map[string]interface{}{
			"provider":   cfg.Provider,
			"detector":   det,
			"category":   g.f.Category,
			"count":      g.count,
			"max_score":  g.maxScore,
			"action":     action,
			"roles":      roles,
			"kinds":      sortedKeys(g.kinds),
			"latency_ms": verdict.Latency.Milliseconds(),
		}
		if len(tools) > 0 {
			metadata["tool_names"] = tools
		}
		events = append(events, ComplianceEventOutput{
			EventType:   "guardrail." + strings.ReplaceAll(det, "/", "."),
			Severity:    sev,
			Description: fmt.Sprintf("guardrail %q (%s): %s detected (%d) in %s; action %s", filterName, cfg.Provider, label, g.count, strings.Join(roles, ","), action),
			Metadata:    metadata,
		})
	}
	return events
}

func severityRank(s string) int {
	switch s {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	}
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
