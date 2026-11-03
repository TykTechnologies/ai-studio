package scripting

// Running the guardrail judge (second opinion). In record mode the judge is
// asked about every segment the filter blocked or quarantined, off the
// request path: on the shadow pool, after the request has gone on with the
// filter's own verdict. Its opinions are recorded as guardrail.judge events
// beside the filter's events and never change anything; they are the
// evidence an administrator needs before letting the judge soften verdicts
// (midsommar-clean-9u1j.14).
//
// The call goes straight to the judge LLM's vendor driver (llmclient), found
// through the runner's service (the store on the hub, the loaded
// configuration on an edge), never through the proxy: no filter recursion,
// and the call is charged to the judge LLM alone (an analytics row with
// interaction type guardrail_judge and no App), never to the App whose
// request was judged.

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/guardrails/segments"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/lrucache"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
	"github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting/llmclient"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// JudgeEventType is the compliance event recording one judge opinion.
const JudgeEventType = "guardrail.judge"

// Judge runtime bounds.
const (
	// judgeMaxSegments is the most segments of one request the judge is
	// asked about; the rest are recorded as not judged.
	judgeMaxSegments = 4
	judgeCacheTTL    = 10 * time.Minute
	judgeCacheMax    = 10_000
)

// Why the judge gave no opinion (metadata.judge.error).
const (
	judgeErrUnavailable = "judge_unavailable" // no service, no such LLM, inactive, not chat
	judgeErrTimeout     = "timeout"
	judgeErrCall        = "call_failed"
	judgeErrParse       = "unparseable"
	judgeErrDropped     = "dropped" // the shadow pool had no room
	judgeErrSkipped     = "over_segment_cap"
	// judgeErrCategory: the segment has a finding that marks the presence of
	// data (a secret, personal data, a leak), not an intent the judge can
	// weigh: "benign" means nothing there, so it is never asked.
	judgeErrCategory = "category"
	// judgeErrPrivacy: the judge LLM's privacy score is below that of the
	// LLM whose traffic it would read, or that LLM is unknown (Studio chat
	// records none), so the text is not sent to it.
	judgeErrPrivacy = "privacy"
	// judgeErrBudget: the judge LLM's own budget is spent (or cannot be
	// checked), so no call is made.
	judgeErrBudget = "over_budget"
)

// LLMBudgetChecker is implemented by a runner service that can check an
// LLM's own budget (services.Service on the hub, the gateway adapter on an
// edge). The judge is called only when the judge LLM's budget allows it: a
// service that cannot say gets no judge calls, so judge spend is never
// unmetered.
type LLMBudgetChecker interface {
	CheckLLMBudget(llm *models.LLM) error
}

// judgeClients makes the judge calls; drivers are cached per LLM version.
var judgeClients = llmclient.New(nil)

// judgeCall asks the judge LLM; a variable so tests can stand in a judge.
var judgeCall = func(ctx context.Context, llm *models.LLM, model, system, user string) (llmclient.Completion, error) {
	return judgeClients.GenerateFor(ctx, llm, model, system, user, JudgeMaxTokens)
}

// judgeCache holds opinions per (app, judge LLM version, model, prompt
// version, judged text and facts) for judgeCacheTTL: a client that sends the
// same flagged context to several checks gets one judge call. Only parsed
// opinions are kept.
var judgeCache = lrucache.New[cachedOpinion](judgeCacheMax)

type cachedOpinion struct {
	opinion JudgeOpinion
	at      time.Time
}

// judgeTarget is one flagged segment the judge is asked about.
type judgeTarget struct {
	index int
	// action is what the filter did to the segment (block, quarantine).
	action string
	// sha256 is the SHA-256 of the text the provider was sent, as the
	// detector event records it (the finding's piece), for correlation.
	sha256    string
	detectors []string
	req       judgeRequest
	// spans locates every finding the segment was flagged for, in req.Text
	// (nil for a finding without a span, or one in other coordinates), so
	// the event can say whether the judge saw them all.
	spans []*guardrails.Span
	// over is set for a segment beyond judgeMaxSegments: recorded, not asked.
	over bool
	// unscanned: part of the segment was not scanned, so no opinion on the
	// text the judge is shown may soften it.
	unscanned bool
	// data: the segment has a secrets, PII or leak finding; never judged.
	data bool
}

// judgeWork is one request's judging, detached from the request.
type judgeWork struct {
	judge     guardrails.Judge
	provider  string
	appID     uint
	requestID string
	// sourceLLMID is the LLM whose traffic is judged (context llm_id), 0
	// when the call site does not say.
	sourceLLMID uint
	svc         services.ServiceInterface
	// format and body are the request, read for the tool names it declares.
	format, body string
	targets      []judgeTarget
}

// dataCategories are the finding categories that mark data in the text
// rather than an intent: the judge (asked whether text is an attack) would
// rightly call a credential or an email address benign, so a segment with
// any such finding is never judged, in record or soften mode. A denylist,
// not an injection-only allowlist: HTTP classifiers (Wolf Defender) report
// their findings as "other".
var dataCategories = map[string]bool{guardrails.CategorySecrets: true, guardrails.CategoryPII: true, guardrails.CategoryLeak: true}

// isDataFinding reports a finding in a data category, by its category or,
// when a provider leaves that out, its detector's name.
func isDataFinding(f guardrails.Finding) bool {
	if dataCategories[strings.ToLower(strings.TrimSpace(f.Category))] {
		return true
	}
	det := strings.ToLower(f.Detector)
	for c := range dataCategories {
		if strings.HasPrefix(det, c+".") || strings.HasPrefix(det, c+"/") || det == c {
			return true
		}
	}
	return false
}

// judgeWorkFor is the judging a run's outcome calls for: every segment the
// filter blocks or quarantines on a detector's finding (never one blocked
// only because it was not scanned), nil when there is none or the judge is
// off. Only record mode is applied for now; soften is judged as record.
func judgeWorkFor(input *ScriptInput, cfg guardrails.Config, sel selection, o outcome, svc services.ServiceInterface) *judgeWork {
	if !cfg.Judge.Enabled() {
		return nil
	}
	idx := make([]int, 0, len(o.actions))
	for si, a := range o.actions {
		if a == guardrails.ActionBlock || a == guardrails.ActionQuarantine {
			idx = append(idx, si)
		}
	}
	sort.Ints(idx)
	w := &judgeWork{
		judge: cfg.Judge, provider: cfg.Provider, appID: appIDOf(input), requestID: requestIDOf(input),
		sourceLLMID: contextUint(input.Context, "llm_id"),
		svc:         svc,
	}
	if !input.IsResponse {
		w.format, w.body = input.Format, input.RawInput
	}
	for _, si := range idx {
		if si < 0 || si >= len(sel.segments) {
			continue
		}
		var top *guardrails.Finding
		var detectors []string
		unscanned, data := false, false
		for i, f := range o.findings[si] {
			if f.Detector == unscannedDetector {
				unscanned = true
				continue
			}
			if isDataFinding(f) {
				data = true
			}
			detectors = append(detectors, safeLabel(f.Detector))
			if top == nil || f.Score > top.Score {
				top = &o.findings[si][i]
			}
		}
		if top == nil {
			continue
		}
		seg := sel.segments[si]
		text := seg.Text
		if top.Backstop && seg.Template != nil {
			text = seg.Template.FullText
		}
		// Spans are in the text the finding was made in: a backstop finding's
		// in the template's full text, any other's in the segment's text.
		var spans []*guardrails.Span
		for _, f := range o.findings[si] {
			if f.Detector == unscannedDetector {
				continue
			}
			if f.Span == nil || f.Backstop != top.Backstop {
				spans = append(spans, nil)
				continue
			}
			sp := *f.Span
			spans = append(spans, &sp)
		}
		sum := ""
		if top.Evidence != nil {
			sum = top.Evidence.SHA256
		}
		if sum == "" {
			h := sha256.Sum256([]byte(text))
			sum = hex.EncodeToString(h[:])
		}
		kind := seg.Kind
		if seg.IsToolResult() {
			kind = guardrails.KindToolResult
		}
		t := judgeTarget{
			index: si, action: o.actions[si], sha256: sum, detectors: compactStrings(sortedCopy(detectors)), spans: spans,
			unscanned: unscanned, data: data,
			req: judgeRequest{
				Text: text, Span: top.Span, Detector: top.Detector, Category: top.Category,
				Kind: kind, Role: seg.Role, ToolName: seg.ToolName,
			},
		}
		if !t.data {
			asked := 0
			for _, prev := range w.targets {
				if !prev.data {
					asked++
				}
			}
			t.over = asked >= judgeMaxSegments
		}
		w.targets = append(w.targets, t)
	}
	if len(w.targets) == 0 {
		return nil
	}
	return w
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

// bytes is the work's weight on the shadow pool.
func (w *judgeWork) bytes() int64 {
	n := 512
	for _, t := range w.targets {
		if !t.over {
			n += min(len(t.req.Text), judgeWindowChars*4)
		}
	}
	return int64(n)
}

// run asks the judge about every target and returns the guardrail.judge
// events. ctx should be detached from the request; each call is bounded by
// the judge's timeout.
func (w *judgeWork) run(ctx context.Context) []ComplianceEventOutput {
	return w.events(w.results(ctx))
}

// results asks the judge about every target, at once (each call bounded by
// the judge's timeout), and returns its result per target, in order.
func (w *judgeWork) results(ctx context.Context) []judgeResult {
	askable := false
	for _, t := range w.targets {
		askable = askable || (!t.data && !t.over)
	}
	var llm *models.LLM
	unavailable := judgeErrSkipped
	if askable {
		llm, unavailable = w.judgeLLM()
	}
	if unavailable == "" {
		w.describeApp()
		tools := declaredToolNames(w.format, w.body)
		for i := range w.targets {
			w.targets[i].req.DeclaredTools = tools
		}
	}
	results := make([]judgeResult, len(w.targets))
	var wg sync.WaitGroup
	for i := range w.targets {
		t := &w.targets[i]
		switch {
		case t.data:
			results[i] = judgeResult{errClass: judgeErrCategory}
		case t.over:
			results[i] = judgeResult{errClass: judgeErrSkipped}
		case unavailable != "":
			results[i] = judgeResult{errClass: unavailable}
		default:
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i] = w.ask(ctx, llm, t)
			}()
		}
	}
	wg.Wait()
	return results
}

// events is the guardrail.judge event of each target's result.
func (w *judgeWork) events(results []judgeResult) []ComplianceEventOutput {
	events := make([]ComplianceEventOutput, len(w.targets))
	for i := range w.targets {
		events[i] = w.event(&w.targets[i], results[i])
	}
	return events
}

// dropped is the events of work the shadow pool had no room for.
func (w *judgeWork) dropped() []ComplianceEventOutput {
	events := make([]ComplianceEventOutput, len(w.targets))
	for i := range w.targets {
		events[i] = w.event(&w.targets[i], judgeResult{errClass: judgeErrDropped})
	}
	return events
}

// judgeLLM is the judge LLM, or why there is none to ask.
func (w *judgeWork) judgeLLM() (*models.LLM, string) {
	if w.svc == nil {
		return nil, judgeErrUnavailable
	}
	llm, err := w.svc.GetLLMByID(w.judge.LLMID)
	if err != nil || llm == nil || !llm.Active {
		return nil, judgeErrUnavailable
	}
	// Checked at save against the filter's LLMs, and again here: the
	// filter may have been attached to an LLM since.
	if w.sourceLLMID == 0 {
		return nil, judgeErrPrivacy
	}
	src, err := w.svc.GetLLMByID(w.sourceLLMID)
	if err != nil || src == nil || llm.PrivacyScore < src.PrivacyScore {
		return nil, judgeErrPrivacy
	}
	// Flagged traffic is free to send (it is blocked) but each judge call
	// is paid: the judge LLM's own budget bounds what it costs.
	bc, ok := w.svc.(LLMBudgetChecker)
	if !ok {
		return nil, judgeErrBudget
	}
	if err := bc.CheckLLMBudget(llm); err != nil {
		slog.Warn("guardrail judge skipped: judge LLM budget", "provider", w.provider, "judge_llm_id", llm.ID)
		return nil, judgeErrBudget
	}
	return llm, ""
}

// contextUint reads a non-negative integer id from a script context.
func contextUint(ctx map[string]interface{}, key string) uint {
	switch v := ctx[key].(type) {
	case int:
		if v > 0 {
			return uint(v)
		}
	case int64:
		if v > 0 {
			return uint(v)
		}
	case uint:
		return v
	case uint32:
		return uint(v)
	case float64:
		if v > 0 {
			return uint(v)
		}
	}
	return 0
}

// describeApp fills in the app's name and description.
func (w *judgeWork) describeApp() {
	if w.appID == 0 {
		return
	}
	app, err := w.svc.GetAppByID(w.appID)
	if err != nil || app == nil {
		return
	}
	for i := range w.targets {
		w.targets[i].req.AppName, w.targets[i].req.AppDescription = app.Name, app.Description
	}
}

// declaredToolNames are the tools a request body offers the model.
func declaredToolNames(format, body string) []string {
	if format == "" || body == "" {
		return nil
	}
	units, err := segments.ExtractUnitsWith(format, []byte(body), segments.Options{ToolDefinitions: true})
	if err != nil {
		return nil
	}
	var names []string
	for _, u := range units {
		if u.Kind == guardrails.KindToolDefinition && u.ToolName != "" {
			names = append(names, u.ToolName)
		}
	}
	return names
}

// judgeResult is one target's outcome.
type judgeResult struct {
	opinion JudgeOpinion
	ok      bool
	// covered: the judge was shown every finding of the segment
	// (judgeCovers). Without it a benign opinion cannot soften anything.
	covered bool
	// softenedTo is the action soften mode turned the segment's into, ""
	// when the opinion changed nothing.
	softenedTo string
	cached     bool
	errClass   string
	model      string
	latency    time.Duration
}

// softens reports whether the opinion may soften the filter's verdict: a
// benign opinion on text that covers everything that was flagged.
func (r judgeResult) softens() bool {
	return r.ok && r.covered && r.opinion.Benign()
}

// ask returns the judge's opinion on t, from the cache when it has one.
func (w *judgeWork) ask(ctx context.Context, llm *models.LLM, t *judgeTarget) judgeResult {
	model := w.judge.Model
	if model == "" {
		model = llm.DefaultModel
	}
	covered := judgeCovers(t.req.Text, t.req.Span, t.spans)
	key := judgeCacheKey(w.appID, llm, model, t.req)
	if c, ok := judgeCache.Get(key); ok {
		if time.Since(c.at) < judgeCacheTTL {
			return judgeResult{opinion: c.opinion, ok: true, cached: true, model: model, covered: covered}
		}
		judgeCache.Remove(key)
	}

	system, user := buildJudgePrompt(t.req)
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(w.judge.Timeout())*time.Millisecond)
	defer cancel()
	start := time.Now()
	out, err := judgeCall(callCtx, llm, model, system, user)
	res := judgeResult{model: model, latency: time.Since(start), covered: covered}
	if err != nil {
		res.errClass = judgeErrCall
		if errors.Is(err, context.DeadlineExceeded) || callCtx.Err() != nil {
			res.errClass = judgeErrTimeout
		}
		slog.Warn("guardrail judge call failed", "provider", w.provider, "judge_llm_id", llm.ID, "error_class", res.errClass)
		return res
	}
	recordJudgeUsage(ctx, w.svc, llm, out, res.latency)
	op, ok := parseJudgeAnswer(out.Text)
	if !ok {
		res.errClass = judgeErrParse
		return res
	}
	res.opinion, res.ok = op, true
	judgeCache.Add(key, cachedOpinion{opinion: op, at: time.Now()})
	return res
}

// judgeCacheKey keys an opinion on everything the prompt is made of, under
// the process's cache MAC (it covers screened text): the app, the judge
// LLM's version, the model, the prompt version, and the request's facts and
// text. The declared tools and app description are left out: an edit to
// them reaches the judge when the entry expires.
func judgeCacheKey(appID uint, llm *models.LLM, model string, r judgeRequest) string {
	mac := guardrails.NewCacheMAC()
	field := func(s string) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(s)))
		mac.Write(n[:])
		mac.Write([]byte(s))
	}
	field(JudgePromptVersion)
	field(strconv.FormatUint(uint64(appID), 10))
	field(strconv.FormatUint(uint64(llm.ID), 10))
	field(strconv.FormatInt(llm.UpdatedAt.UnixNano(), 10))
	field(model)
	field(r.Kind)
	field(r.Role)
	field(r.ToolName)
	field(r.Detector)
	field(r.Category)
	window, _, _, start := judgeWindow(r.Text, r.Span)
	field(strconv.Itoa(start))
	field(window)
	return string(mac.Sum(nil))
}

// recordJudgeUsage records the judge call's usage against the judge LLM, with
// no App: it counts toward the judge LLM's budget and analytics only.
func recordJudgeUsage(ctx context.Context, svc services.ServiceInterface, llm *models.LLM, out llmclient.Completion, latency time.Duration) {
	rec := &models.LLMChatRecord{
		LLMID:           llm.ID,
		Name:            out.Model,
		Vendor:          string(llm.Vendor),
		PromptTokens:    out.PromptTokens,
		ResponseTokens:  out.CompletionTokens,
		TotalTokens:     out.PromptTokens + out.CompletionTokens,
		TotalTimeMS:     int(latency.Milliseconds()),
		TimeStamp:       time.Now(),
		Choices:         1,
		Currency:        "USD",
		InteractionType: models.GuardrailJudgeInteraction,
	}
	if price, err := svc.GetModelPriceByModelNameAndVendor(out.Model, string(llm.Vendor)); err == nil && price != nil {
		rec.Currency = price.Currency
		rec.Cost = models.ChatRecordCost(string(llm.Vendor), price.CPT, price.CPIT, price.CacheWritePT, price.CacheReadPT,
			out.PromptTokens, out.CompletionTokens, 0, 0)
	}
	analytics.RecordChatRecord(ctx, rec)
}

// event is the guardrail.judge event for one target. The opinion travels
// under metadata.judge, a key only the guardrail runner may set
// (ReservedEventKeys), so a script filter cannot fake one.
func (w *judgeWork) event(t *judgeTarget, r judgeResult) ComplianceEventOutput {
	judge := map[string]interface{}{
		"llm_id":         w.judge.LLMID,
		"mode":           w.judge.Mode,
		"prompt_version": JudgePromptVersion,
		"segment":        t.index,
		"sha256":         t.sha256,
		"action":         t.action,
		"detectors":      t.detectors,
		"latency_ms":     r.latency.Milliseconds(),
		"cached":         r.cached,
	}
	if r.model != "" {
		judge["model"] = r.model
	}
	if r.softenedTo != "" {
		judge["softened_from"] = t.action
		judge["softened_to"] = r.softenedTo
	}
	description := "guardrail judge gave no opinion (" + r.errClass + ") on a " + t.action + " verdict"
	if r.ok {
		judge["verdict"] = r.opinion.Verdict
		judge["reason"] = r.opinion.Reason
		// The judge agrees with the filter unless it found the text benign.
		judge["agreed"] = !r.opinion.Benign()
		// partial: the judge was not shown everything that was flagged (a
		// finding without a span in text longer than its window, or a
		// finding outside it), so its opinion can never soften the verdict.
		judge["coverage"] = map[bool]string{true: "full", false: "partial"}[r.covered]
		description = "guardrail judge: " + r.opinion.Verdict + " (" + r.opinion.Reason + ") on a " + t.action + " verdict"
	} else {
		judge["verdict"] = "none"
		judge["error"] = r.errClass
	}
	metadata := map[string]interface{}{
		"provider": w.provider,
		"judge":    judge,
		"action":   guardrails.ActionLog,
		"blocked":  false,
	}
	if w.requestID != "" {
		metadata["request_id"] = w.requestID
	}
	return ComplianceEventOutput{EventType: JudgeEventType, Severity: "info", Description: description, Metadata: metadata}
}

// dispatchJudge has w judged on the shadow pool, without delaying the
// request, its events recorded through input.RecordEvents when done. A run
// without a recorder (the filter editor's test run, or a shadow filter's
// own background evaluation, which acts on nothing a judge could soften) is
// not judged.
func (g *guardrailRunner) dispatchJudge(input *ScriptInput, w *judgeWork) {
	record := input.RecordEvents
	if record == nil {
		return
	}
	parent := input.Ctx
	if parent == nil {
		parent = context.Background()
	}
	detached := context.WithoutCancel(parent)
	job := &shadowJob{bytes: w.bytes(), tenant: tenantScope(input.Context)}
	job.run = func() {
		defer func() {
			if r := recover(); r != nil {
				safe.ReportRedacted("guardrail_judge", r)
			}
		}()
		g.recordEvents(record, detached, &ScriptOutput{ComplianceEvents: w.run(detached)})
	}
	if reason := currentShadowPool().enqueue(job); reason != "" {
		slog.Warn("guardrail judge dropped: shadow pool full", "provider", w.provider, "reason", reason)
		g.recordEvents(record, detached, &ScriptOutput{ComplianceEvents: w.dropped()})
	}
}
