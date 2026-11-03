package scripting

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// softenFilter is judgeFilter in soften mode.
func softenFilter(t *testing.T, onDetect string, timeoutMs int, extra map[string]any) *models.Filter {
	t.Helper()
	cfg := map[string]any{"judge": map[string]any{"mode": "soften", "llm_id": 70, "timeout_ms": timeoutMs}}
	for k, v := range extra {
		cfg[k] = v
	}
	return judgeFilter(t, onDetect, cfg)
}

// withCapability serves app 9's attested capability (and no exceptions).
func withCapability(t *testing.T, c guardrails.Capability) {
	t.Helper()
	withExceptions(t, &fakeExceptions{byApp: map[uint]*AppExceptions{9: {Capability: c}}})
}

func secretsEvent(t *testing.T, out *ScriptOutput) ComplianceEventOutput {
	t.Helper()
	for _, e := range out.ComplianceEvents {
		if e.EventType == "guardrail.prompt_attack" {
			return e
		}
	}
	t.Fatalf("no detector event in %+v", out.ComplianceEvents)
	return ComplianceEventOutput{}
}

const softenToolBody = `{"messages":[{"role":"user","content":"read it"},` +
	`{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"read_file","arguments":"{}"}}]},` +
	`{"role":"tool","tool_call_id":"c1","content":"file says SECRET"}]}`

// A benign opinion that saw everything flagged softens the segment to the
// softest action the app's capability allows: log for a read-only app's own
// text. The detector event and the judge's event say so.
func TestJudgeSoften_BenignSoftensToTheFloor(t *testing.T) {
	stub := installStubJudge(t, answering(`{"verdict":"benign","reason":"code_or_data"}`))
	withCapability(t, guardrails.CapabilityReadOnly)
	out, err := NewFilterRunner(softenFilter(t, guardrails.ActionBlock, 2000, nil)).RunScript(judgeInput(newEventSink(), judgeBody), newJudgeSvc())
	if err != nil {
		t.Fatal(err)
	}
	if out.Block {
		t.Fatal("a benign opinion did not soften the block")
	}
	if stub.calls.Load() != 1 {
		t.Errorf("judge calls = %d", stub.calls.Load())
	}
	ev := secretsEvent(t, out)
	if ev.Metadata["action"] != guardrails.ActionLog {
		t.Errorf("action = %v", ev.Metadata["action"])
	}
	if j, _ := ev.Metadata["judge"].(map[string]any); j["verdict"] != JudgeBenign || j["softened_from"] != guardrails.ActionBlock {
		t.Errorf("event judge = %v", ev.Metadata["judge"])
	}
	if !strings.Contains(ev.Description, "softened by the LLM judge") {
		t.Errorf("description %q", ev.Description)
	}
	seg := ev.Metadata["segments"].([]map[string]any)[0]
	if sj, _ := seg["judge"].(map[string]any); sj["to"] != guardrails.ActionLog {
		t.Errorf("segment judge = %v", seg["judge"])
	}
	// The opinion is recorded inline, marked as what it changed.
	jes := judgeEvents(out.ComplianceEvents)
	if len(jes) != 1 {
		t.Fatalf("judge events %+v", jes)
	}
	if j := judgeMeta(t, jes[0]); j["softened_from"] != guardrails.ActionBlock || j["softened_to"] != guardrails.ActionLog || j["mode"] != guardrails.JudgeSoften {
		t.Errorf("judge event %+v", j)
	}
}

// Never below the floor: an unattested (acting) app's own text can only
// have the flagged part removed, and an acting app's tool results cannot be
// softened at all, so the judge is not asked inline for them.
func TestJudgeSoften_NeverBelowTheFloor(t *testing.T) {
	stub := installStubJudge(t, answering(`{"verdict":"benign","reason":"code_or_data"}`))
	withCapability(t, guardrails.CapabilityUnattested)
	out, _ := NewFilterRunner(softenFilter(t, guardrails.ActionBlock, 2000, nil)).RunScript(judgeInput(newEventSink(), judgeBody), newJudgeSvc())
	if out.Block {
		t.Fatal("not softened")
	}
	if a := secretsEvent(t, out).Metadata["action"]; a != guardrails.ActionQuarantine {
		t.Errorf("acting app's text softened to %v, want quarantine", a)
	}
	if stub.calls.Load() != 1 {
		t.Fatalf("calls %d", stub.calls.Load())
	}

	// Tool results of an acting app: nothing to soften to; quarantined as
	// configured, and the judge only records, in the background.
	freshShadowPool(t, 2, 1<<20)
	judgeCache.Reset()
	sink := newEventSink()
	f := softenFilter(t, guardrails.ActionLog, 2000, map[string]any{"tool_results": map[string]any{"scan": true, "on_detect": guardrails.ActionQuarantine}})
	start := time.Now()
	out, _ = NewFilterRunner(f).RunScript(judgeInput(sink, softenToolBody), newJudgeSvc())
	if time.Since(start) > time.Second {
		t.Error("an unsoftenable segment waited for the judge")
	}
	if out.Message != "guardrail quarantined content" {
		t.Errorf("tool result not quarantined: %+v", out)
	}
	if len(judgeEvents(out.ComplianceEvents)) != 0 {
		t.Error("judged inline")
	}
	if events := judgeEvents(sink.wait(t, 1, 5*time.Second)); len(events) != 1 || judgeMeta(t, events[0])["softened_to"] != nil {
		t.Errorf("background opinion %+v", events)
	}
}

// Anything but a benign opinion that saw everything keeps the verdict.
func TestJudgeSoften_OtherwiseTheVerdictStands(t *testing.T) {
	withCapability(t, guardrails.CapabilityReadOnly)
	pad := strings.Repeat("func add(a, b int) int { return a + b } // test fixture ", 100)
	for _, c := range []struct {
		name    string
		answer  func(context.Context) (string, error)
		content string
		want    string
	}{
		{"suspicious", answering(`{"verdict":"suspicious","reason":"unclear"}`), "review this: SECRET", "suspicious"},
		{"attack", answering(`{"verdict":"attack","reason":"instructions_to_model"}`), "review this: SECRET", "attack"},
		{"unparseable", answering(`benign!`), "review this: SECRET", "none"},
		{"timeout", func(ctx context.Context) (string, error) { <-ctx.Done(); return "", ctx.Err() }, "review this: SECRET", "none"},
		// Benign, but the judge did not see past the window: no softening.
		{"partial coverage", answering(`{"verdict":"benign","reason":"code_or_data"}`), pad + "SECRET", "benign"},
	} {
		t.Run(c.name, func(t *testing.T) {
			installStubJudge(t, c.answer)
			body := `{"messages":[{"role":"user","content":"` + c.content + `"}]}`
			out, _ := NewFilterRunner(softenFilter(t, guardrails.ActionBlock, 100, nil)).RunScript(judgeInput(newEventSink(), body), newJudgeSvc())
			if !out.Block {
				t.Fatal("verdict changed")
			}
			if _, softened := secretsEvent(t, out).Metadata["judge"]; softened {
				t.Error("event says the judge softened it")
			}
			jes := judgeEvents(out.ComplianceEvents)
			if len(jes) != 1 || judgeMeta(t, jes[0])["verdict"] != c.want || judgeMeta(t, jes[0])["softened_to"] != nil {
				t.Errorf("judge events %+v", jes)
			}
		})
	}
}

// Soften mode waits for the judge at most its timeout.
func TestJudgeSoften_LatencyBoundedByTheTimeout(t *testing.T) {
	withCapability(t, guardrails.CapabilityReadOnly)
	installStubJudge(t, func(ctx context.Context) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return `{"verdict":"benign","reason":"code_or_data"}`, nil
		}
	})
	start := time.Now()
	out, _ := NewFilterRunner(softenFilter(t, guardrails.ActionBlock, 150, nil)).RunScript(judgeInput(newEventSink(), judgeBody), newJudgeSvc())
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("run took %v with a 150 ms judge timeout", took)
	}
	if !out.Block {
		t.Error("a judge that timed out softened the verdict")
	}
}

// A segment part of which was not scanned is never softened: the judge's
// opinion covers only the text it is shown.
func TestJudgeSoften_NotWhenPartUnscanned(t *testing.T) {
	stub := installStubJudge(t, answering(`{"verdict":"benign","reason":"code_or_data"}`))
	withCapability(t, guardrails.CapabilityReadOnly)
	cfg := guardrails.Config{Provider: "x", Judge: guardrails.Judge{Mode: guardrails.JudgeSoften, LLMID: 70, TimeoutMs: 1000}}
	sel := selection{segments: []guardrails.Segment{{Index: 0, Role: "user", Text: "review this: SECRET"}}}
	o := outcome{
		actions: map[int]string{0: guardrails.ActionBlock},
		findings: map[int][]guardrails.Finding{0: {
			{Detector: "prompt_attack", Category: guardrails.CategoryInjection, Score: 1, SegmentIndex: 0},
			unscannedFinding(0, "Not scanned in full (max_scan_chars)"),
		}},
		overall: guardrails.ActionBlock,
	}
	g := &guardrailRunner{filter: &models.Filter{ID: 13, Name: "wolf"}}
	events, handled := g.judgeSoften(judgeInput(newEventSink(), judgeBody), cfg, sel, &o, newJudgeSvc())
	if handled || events != nil || o.actions[0] != guardrails.ActionBlock || stub.calls.Load() != 0 {
		t.Errorf("handled %v events %v action %v calls %d", handled, events, o.actions[0], stub.calls.Load())
	}
}

// A secret, personal data or a leak is data, not an intent: the judge, asked
// whether the text is an attack, would rightly call it benign, so a segment
// with such a finding is never judged, in soften or record mode, and the
// verdict stands (midsommar-clean-9u1j.57).
func TestJudge_NeverJudgesDataFindings(t *testing.T) {
	withCapability(t, guardrails.CapabilityReadOnly)
	for _, f := range []guardrails.Finding{
		{Detector: "secrets.test_key", Category: guardrails.CategorySecrets},
		{Detector: "pii.email", Category: guardrails.CategoryPII},
		{Detector: "leak.system_prompt", Category: guardrails.CategoryLeak},
		{Detector: "secrets.aws_key"}, // category left out: the detector says it
	} {
		finding := f
		mixed := func(in guardrails.Input) guardrails.Verdict {
			v := guardrails.Verdict{}
			for _, s := range in.Segments {
				if strings.Contains(s.Text, "SECRET") {
					v.Flagged = true
					d := finding
					d.Score, d.Severity, d.SegmentIndex = 1, "critical", s.Index
					// An injection finding beside it does not make it judgeable.
					v.Findings = append(v.Findings, d, guardrails.Finding{Detector: "prompt_attack", Category: guardrails.CategoryInjection, Score: 1, Severity: "critical", SegmentIndex: s.Index})
				}
			}
			return v
		}
		for _, mode := range []string{guardrails.JudgeSoften, guardrails.JudgeRecord} {
			stub := installStubJudge(t, answering(`{"verdict":"benign","reason":"code_or_data"}`))
			freshShadowPool(t, 2, 1<<20)
			cfg := installFake(t, fmt.Sprintf("fake_judge_data_%d", judgeFakeSeq.Add(1)), &fakeProvider{verdict: mixed}, guardrails.ActionBlock)
			cfg["judge"] = map[string]any{"mode": mode, "llm_id": 70, "timeout_ms": 1000}
			flt := guardrailFilter("wolf", false, cfg)
			flt.ID = 13
			sink := newEventSink()
			out, _ := NewFilterRunner(flt).RunScript(judgeInput(sink, judgeBody), newJudgeSvc())
			if !out.Block {
				t.Fatalf("%s/%s: a data finding was softened", f.Detector, mode)
			}
			if len(judgeEvents(out.ComplianceEvents)) != 0 {
				t.Errorf("%s/%s: judged inline", f.Detector, mode)
			}
			// Nothing to soften: recorded in the background, as skipped.
			events := judgeEvents(sink.wait(t, 1, 5*time.Second))
			if len(events) != 1 || judgeMeta(t, events[0])["error"] != judgeErrCategory || judgeMeta(t, events[0])["verdict"] != "none" {
				t.Errorf("%s/%s: judge events %+v", f.Detector, mode, events)
			}
			if stub.calls.Load() != 0 {
				t.Errorf("%s/%s: the judge was asked about a data finding", f.Detector, mode)
			}
		}
	}
}

// The judge's question ("is this an attack?") fits injection findings only:
// moderation, denied topics, blocklists and other policy findings are never
// judged, nor data findings; an HTTP or Unix-socket classifier's own "other"
// (Wolf Defender) is judged (midsommar-clean-9u1j.61).
func TestJudgeableFinding(t *testing.T) {
	f := func(det, cat string) guardrails.Finding { return guardrails.Finding{Detector: det, Category: cat} }
	for _, c := range []struct {
		f        guardrails.Finding
		provider string
		want     bool
	}{
		{f("prompt_attack", guardrails.CategoryInjection), guardrails.ProviderLakera, true},
		{f("injection.ignore_instructions", guardrails.CategoryInjection), guardrails.ProviderBuiltin, true},
		{f("prompt_attack", guardrails.CategoryOther), guardrails.ProviderHTTP, true}, // Wolf
		{f("prompt_attack", ""), guardrails.ProviderHTTP, true},
		{f("secrets.aws", ""), guardrails.ProviderHTTP, false},
		{f("pii.email", guardrails.CategoryOther), guardrails.ProviderHTTP, false},
		{f("Hate", guardrails.CategoryModeration), guardrails.ProviderAzureContentSafety, false},
		{f("Violence", guardrails.CategoryModeration), guardrails.ProviderHTTP, false},
		{f("denied_topic.finance", "topic"), guardrails.ProviderBedrockGuardrails, false},
		{f("blocklist.custom", guardrails.CategoryOther), guardrails.ProviderAzureContentSafety, false},
		{f("word_filter", guardrails.CategoryOther), guardrails.ProviderBedrockGuardrails, false},
		{f("custom", guardrails.CategoryOther), guardrails.ProviderBuiltin, false},
		{f("unknown_links", ""), guardrails.ProviderBuiltin, false},
		{f("secrets.test_key", guardrails.CategorySecrets), guardrails.ProviderBuiltin, false},
	} {
		if got := judgeableFinding(c.f, c.provider); got != c.want {
			t.Errorf("judgeableFinding(%+v, %s) = %v, want %v", c.f, c.provider, got, c.want)
		}
	}
}

// A moderation finding (here beside an injection finding) keeps the segment
// from the judge in soften mode: the verdict stands, nothing is asked.
func TestJudgeSoften_ModerationNeverJudged(t *testing.T) {
	stub := installStubJudge(t, answering(`{"verdict":"benign","reason":"fits_app_purpose"}`))
	withCapability(t, guardrails.CapabilityReadOnly)
	freshShadowPool(t, 2, 1<<20)
	hate := func(in guardrails.Input) guardrails.Verdict {
		v := guardrails.Verdict{}
		for _, s := range in.Segments {
			if strings.Contains(s.Text, "SECRET") {
				v.Flagged = true
				v.Findings = append(v.Findings,
					guardrails.Finding{Detector: "Hate", Category: guardrails.CategoryModeration, Score: 1, Severity: "critical", SegmentIndex: s.Index},
					guardrails.Finding{Detector: "prompt_attack", Category: guardrails.CategoryInjection, Score: 1, Severity: "critical", SegmentIndex: s.Index})
			}
		}
		return v
	}
	cfg := installFake(t, fmt.Sprintf("fake_judge_mod_%d", judgeFakeSeq.Add(1)), &fakeProvider{verdict: hate}, guardrails.ActionBlock)
	cfg["judge"] = map[string]any{"mode": "soften", "llm_id": 70, "timeout_ms": 1000}
	flt := guardrailFilter("wolf", false, cfg)
	flt.ID = 13
	sink := newEventSink()
	out, _ := NewFilterRunner(flt).RunScript(judgeInput(sink, judgeBody), newJudgeSvc())
	if !out.Block {
		t.Fatal("a moderation block was softened")
	}
	events := judgeEvents(sink.wait(t, 1, 5*time.Second))
	if len(events) != 1 || judgeMeta(t, events[0])["error"] != judgeErrCategory || stub.calls.Load() != 0 {
		t.Errorf("events %+v calls %d", events, stub.calls.Load())
	}
}
