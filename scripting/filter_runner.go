package scripting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/tmc/langchaingo/llms"
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
// (chat messages, tool envelopes, response buffers: one text that is the
// raw input itself) or as a rewritten Messages array (proxy requests, where
// the raw input is the vendor JSON and the reconstructor rebuilds it).
type segmentMode int

const (
	modePayload segmentMode = iota
	modeMessages
)

func (g *guardrailRunner) RunScript(input *ScriptInput, _ services.ServiceInterface) (*ScriptOutput, error) {
	cfg, err := guardrails.ParseConfig(g.filter.Config)
	if err != nil {
		return nil, fmt.Errorf("guardrail '%s': %w", g.filter.Name, err)
	}
	cfg, err = guardrails.Normalize(cfg, g.filter.ResponseFilter)
	if err != nil {
		return nil, fmt.Errorf("guardrail '%s': %w", g.filter.Name, err)
	}
	// Connection values are stored as $SECRET/ and $ENV/ references. Resolve
	// them here, where the filter runs: on Studio against the secret store,
	// at the edge against the values the hub already resolved into the
	// snapshot (GetValue leaves a non-reference untouched).
	resolved := make(map[string]string, len(cfg.Connection))
	for k, v := range cfg.Connection {
		resolved[k] = secrets.GetValue(v, false)
	}
	cfg.Connection = resolved

	provider, err := guardrails.NewProvider(cfg)
	if errors.Is(err, guardrails.ErrNoProviders) {
		warnNoProvidersOnce.Do(func() {
			slog.Warn("guardrail filters are an Enterprise feature; attached guardrails are evaluated as pass-through")
		})
		return passThrough(input), nil
	}
	if err != nil {
		return g.failed(input, cfg, err), nil
	}
	caps := provider.Capabilities()

	if input.IsChunk && !shouldEvaluateChunk(input, cfg.EvaluateEvery(caps)) {
		return passThrough(input), nil
	}

	segments, mode := buildSegments(input, cfg.Scope)
	if !anyText(segments) {
		return passThrough(input), nil
	}

	direction := guardrails.DirectionRequest
	if input.IsResponse {
		direction = guardrails.DirectionResponse
	}
	wantRewrite := cfg.OnDetect == guardrails.ActionRedact && caps.Redacts && !input.IsResponse

	pieces, parents := splitSegments(segments, caps.MaxChars)
	in := guardrails.Input{
		Segments:  pieces,
		Direction: direction,
		Meta:      meta(input),
	}
	if wantRewrite {
		in.Redact = &guardrails.RedactionSpec{Style: cfg.Redaction.Style, Placeholder: cfg.Redaction.Placeholder}
	}

	ctx := input.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	verdict, err := guardrails.Classify(ctx, cfg, provider, in)
	if err != nil {
		metrics.RecordGuardrail(ctx, cfg.Provider, "error", cfg.OnDetect, verdict.Latency.Seconds())
		return g.failed(input, cfg, err), nil
	}

	outcome := "clean"
	if verdict.Flagged {
		outcome = "flagged"
	}
	metrics.RecordGuardrail(ctx, cfg.Provider, outcome, cfg.OnDetect, verdict.Latency.Seconds())

	out := passThrough(input)
	if !verdict.Flagged {
		return out, nil
	}

	// Map piece-level findings back onto the original segments so the
	// compliance events name the message role, and merge piece rewrites.
	findings := remapFindings(verdict.Findings, parents)
	rewritten := mergeRewrites(segments, pieces, parents, verdict.Rewritten)

	action := cfg.OnDetect
	if action == guardrails.ActionRedact && (input.IsResponse || len(rewritten) == 0) {
		// The LLM response path is block-only, and a provider that found
		// something but could not rewrite it has nothing to apply: record
		// the finding and let the text through.
		action = guardrails.ActionLog
	}

	out.ComplianceEvents = complianceEvents(g.filter.Name, cfg, findings, segments, action, verdict)

	switch action {
	case guardrails.ActionBlock:
		out.Block = true
		out.Payload = ""
		out.Message = cfg.BlockMessage
	case guardrails.ActionRedact:
		applyRewrites(out, input, segments, rewritten, mode)
		out.Message = "guardrail redacted content"
	}
	return out, nil
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
func buildSegments(input *ScriptInput, scope string) ([]guardrails.Segment, segmentMode) {
	if input.IsResponse {
		text := input.RawInput
		if input.IsChunk {
			text = input.CurrentBuffer
		}
		return []guardrails.Segment{{Index: 0, Role: "assistant", Text: text}}, modePayload
	}

	// Chat messages and tool envelopes arrive as one message whose content is
	// the raw input itself; a rewrite goes back as the payload.
	if len(input.Messages) == 1 && messageText(input.Messages[0]) == input.RawInput {
		return []guardrails.Segment{{Index: 0, Role: roleName(input.Messages[0].Role), Text: input.RawInput}}, modePayload
	}
	if len(input.Messages) == 0 || !anyMessageText(input.Messages) {
		// No extracted messages (an unknown body shape, or the test endpoint
		// handing over a body it could not map): classify the raw input.
		return []guardrails.Segment{{Index: 0, Role: "user", Text: input.RawInput}}, modePayload
	}

	// Proxy requests: the extracted messages, filtered by scope. Index is the
	// position in input.Messages so a rewrite lands on the right message.
	segments := make([]guardrails.Segment, 0, len(input.Messages))
	lastUser := -1
	for i, m := range input.Messages {
		if m.Role == llms.ChatMessageTypeHuman {
			lastUser = i
		}
	}
	for i, m := range input.Messages {
		role := roleName(m.Role)
		include := false
		switch scope {
		case guardrails.ScopeLastUser:
			include = i == lastUser
		case guardrails.ScopeAllUser:
			include = m.Role == llms.ChatMessageTypeHuman
		case guardrails.ScopeSystemAndUser:
			include = m.Role == llms.ChatMessageTypeHuman || m.Role == llms.ChatMessageTypeSystem
		default:
			include = true
		}
		if include {
			segments = append(segments, guardrails.Segment{Index: i, Role: role, Text: messageText(m)})
		}
	}
	return segments, modeMessages
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

// splitSegments cuts segments longer than maxChars into pieces at rune
// boundaries. parents[i] is the index into segments of piece i; a segment
// that fits is a single piece.
func splitSegments(segments []guardrails.Segment, maxChars int) ([]guardrails.Segment, []int) {
	pieces := make([]guardrails.Segment, 0, len(segments))
	parents := make([]int, 0, len(segments))
	for si, s := range segments {
		if maxChars <= 0 || utf8.RuneCountInString(s.Text) <= maxChars {
			pieces = append(pieces, guardrails.Segment{Index: len(pieces), Role: s.Role, Text: s.Text})
			parents = append(parents, si)
			continue
		}
		runes := []rune(s.Text)
		for start := 0; start < len(runes); start += maxChars {
			end := start + maxChars
			if end > len(runes) {
				end = len(runes)
			}
			pieces = append(pieces, guardrails.Segment{Index: len(pieces), Role: s.Role, Text: string(runes[start:end])})
			parents = append(parents, si)
		}
	}
	return pieces, parents
}

// remapFindings points each finding at its original segment (position in
// the segments slice) instead of its piece.
func remapFindings(findings []guardrails.Finding, parents []int) []guardrails.Finding {
	out := make([]guardrails.Finding, 0, len(findings))
	for _, f := range findings {
		if f.SegmentIndex >= 0 && f.SegmentIndex < len(parents) {
			f.SegmentIndex = parents[f.SegmentIndex]
		}
		f.Span = nil // piece-relative; meaningless once remapped
		out = append(out, f)
	}
	return out
}

// mergeRewrites reassembles rewritten pieces into whole-segment texts. Only
// segments with at least one rewritten piece are returned, keyed by their
// position in segments.
func mergeRewrites(segments, pieces []guardrails.Segment, parents []int, rewritten map[int]string) map[int]string {
	if len(rewritten) == 0 {
		return nil
	}
	touched := map[int]bool{}
	for pi := range rewritten {
		if pi >= 0 && pi < len(parents) {
			touched[parents[pi]] = true
		}
	}
	out := make(map[int]string, len(touched))
	for si := range touched {
		var b strings.Builder
		for pi, p := range pieces {
			if parents[pi] != si {
				continue
			}
			if r, ok := rewritten[pi]; ok {
				b.WriteString(r)
			} else {
				b.WriteString(p.Text)
			}
		}
		if b.String() != segments[si].Text {
			out[si] = b.String()
		}
	}
	return out
}

// applyRewrites writes redacted text back in the form the call site
// consumes.
func applyRewrites(out *ScriptOutput, input *ScriptInput, segments []guardrails.Segment, rewritten map[int]string, mode segmentMode) {
	if mode == modePayload {
		if r, ok := rewritten[0]; ok {
			out.Payload = r
		}
		return
	}
	// Messages mode: emit every message so the reconstructor rebuilds the
	// full conversation, with rewritten content where the guardrail changed it.
	bySegment := map[int]string{}
	for si, text := range rewritten {
		bySegment[segments[si].Index] = text
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
func complianceEvents(filterName string, cfg guardrails.Config, findings []guardrails.Finding, segments []guardrails.Segment, action string, verdict guardrails.Verdict) []ComplianceEventOutput {
	type group struct {
		f        guardrails.Finding
		count    int
		maxScore float64
		severity string
		roles    map[string]bool
	}
	groups := map[string]*group{}
	order := []string{}
	for _, f := range findings {
		g, ok := groups[f.Detector]
		if !ok {
			g = &group{f: f, severity: f.Severity, roles: map[string]bool{}}
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
		if f.SegmentIndex >= 0 && f.SegmentIndex < len(segments) {
			g.roles[segments[f.SegmentIndex].Role] = true
		}
	}
	sort.Strings(order)

	events := make([]ComplianceEventOutput, 0, len(order))
	for _, det := range order {
		g := groups[det]
		roles := make([]string, 0, len(g.roles))
		for r := range g.roles {
			roles = append(roles, r)
		}
		sort.Strings(roles)
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
		events = append(events, ComplianceEventOutput{
			EventType:   "guardrail." + strings.ReplaceAll(det, "/", "."),
			Severity:    sev,
			Description: fmt.Sprintf("guardrail %q (%s): %s detected (%d) in %s; action %s", filterName, cfg.Provider, label, g.count, strings.Join(roles, ","), action),
			Metadata: map[string]interface{}{
				"provider":   cfg.Provider,
				"detector":   det,
				"category":   g.f.Category,
				"count":      g.count,
				"max_score":  g.maxScore,
				"action":     action,
				"roles":      roles,
				"latency_ms": verdict.Latency.Milliseconds(),
			},
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
