package scripting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting/llmclient"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// judgeSvc is the runner's service as the judge uses it: the judge LLM, the
// app and the judge model's price.
type judgeSvc struct {
	services.ServiceInterface
	llm   *models.LLM
	app   *models.App
	price *models.ModelPrice
}

func (s *judgeSvc) GetLLMByID(id uint) (*models.LLM, error) {
	if s.llm == nil || s.llm.ID != id {
		return nil, errors.New("not found")
	}
	cp := *s.llm
	return &cp, nil
}

func (s *judgeSvc) GetAppByID(id uint) (*models.App, error) {
	if s.app == nil || s.app.ID != id {
		return nil, errors.New("not found")
	}
	return s.app, nil
}

func (s *judgeSvc) GetModelPriceByModelNameAndVendor(model, vendor string) (*models.ModelPrice, error) {
	if s.price == nil {
		return nil, errors.New("no price")
	}
	return s.price, nil
}

func newJudgeSvc() *judgeSvc {
	llm := &models.LLM{ID: 70, Name: "judge", Vendor: models.OPENAI, Active: true, DefaultModel: "judge-mini"}
	llm.UpdatedAt = time.Unix(1700000000, 0)
	app := &models.App{Name: "Visor", Description: "Reviews pull requests"}
	app.ID = 9
	return &judgeSvc{llm: llm, app: app, price: &models.ModelPrice{ModelName: "judge-mini", Vendor: "openai", CPT: 1, CPIT: 2, Currency: "USD"}}
}

// stubJudge stands in the judge LLM: answer is called for each call.
type stubJudge struct {
	calls   atomic.Int32
	mu      sync.Mutex
	systems []string
	users   []string
	answer  func(ctx context.Context) (string, error)
}

func installStubJudge(t *testing.T, answer func(ctx context.Context) (string, error)) *stubJudge {
	t.Helper()
	s := &stubJudge{answer: answer}
	prev := judgeCall
	judgeCall = func(ctx context.Context, llm *models.LLM, model, system, user string) (llmclient.Completion, error) {
		s.calls.Add(1)
		s.mu.Lock()
		s.systems = append(s.systems, system)
		s.users = append(s.users, user)
		s.mu.Unlock()
		text, err := s.answer(ctx)
		if err != nil {
			return llmclient.Completion{}, err
		}
		return llmclient.Completion{Text: text, Model: model, PromptTokens: 100, CompletionTokens: 10}, nil
	}
	judgeCache.Reset()
	t.Cleanup(func() {
		judgeCall = prev
		judgeCache.Reset()
	})
	return s
}

func answering(text string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return text, nil }
}

var judgeFakeSeq atomic.Int32

// judgeFilter is a guardrail that flags SECRET with on_detect, judged by the
// svc's judge LLM in record mode.
func judgeFilter(t *testing.T, onDetect string, extra map[string]any) *models.Filter {
	t.Helper()
	cfg := installFake(t, fmt.Sprintf("fake_judge_%d", judgeFakeSeq.Add(1)), &fakeProvider{}, onDetect)
	cfg["judge"] = map[string]any{"mode": "record", "llm_id": 70, "timeout_ms": 1000}
	for k, v := range extra {
		cfg[k] = v
	}
	f := guardrailFilter("wolf", false, cfg)
	f.ID = 13
	return f
}

func judgeInput(sink *eventSink, body string) *ScriptInput {
	return &ScriptInput{
		RawInput: body, Format: "openai",
		Context:      map[string]interface{}{"app_id": 9, "request_id": "tyai_judge1"},
		RecordEvents: sink.record,
	}
}

const judgeBody = `{"messages":[{"role":"user","content":"review this: SECRET"}],"tools":[{"type":"function","function":{"name":"post_comment","parameters":{}}}]}`

func judgeEvents(events []ComplianceEventOutput) []ComplianceEventOutput {
	var out []ComplianceEventOutput
	for _, e := range events {
		if e.EventType == JudgeEventType {
			out = append(out, e)
		}
	}
	return out
}

func judgeMeta(t *testing.T, e ComplianceEventOutput) map[string]interface{} {
	t.Helper()
	m, ok := e.Metadata["judge"].(map[string]interface{})
	if !ok {
		t.Fatalf("event has no judge metadata: %+v", e.Metadata)
	}
	return m
}

// Record mode never changes the verdict and adds no latency: the request
// is blocked at once, exactly as without a judge, and the opinion arrives
// later as a guardrail.judge event naming the filter.
func TestJudgeRecord_NoLatencyVerdictUnchanged(t *testing.T) {
	freshShadowPool(t, 4, 1<<20)
	stub := installStubJudge(t, func(ctx context.Context) (string, error) {
		time.Sleep(500 * time.Millisecond)
		return `{"verdict":"benign","reason":"code_or_data"}`, nil
	})
	svc := newJudgeSvc()
	sink := newEventSink()

	start := time.Now()
	out, err := NewFilterRunner(judgeFilter(t, guardrails.ActionBlock, nil)).RunScript(judgeInput(sink, judgeBody), svc)
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 200*time.Millisecond {
		t.Errorf("record-mode run took %v", took)
	}
	if !out.Block || out.Message != guardrails.DefaultRequestBlockMessage {
		t.Errorf("verdict changed: block=%v message=%q", out.Block, out.Message)
	}
	if len(judgeEvents(out.ComplianceEvents)) != 0 {
		t.Error("judge events in the inline output")
	}

	// Same output as the filter without a judge.
	plain := judgeFilter(t, guardrails.ActionBlock, map[string]any{"judge": map[string]any{}})
	want, _ := NewFilterRunner(plain).RunScript(judgeInput(newEventSink(), judgeBody), svc)
	if want.Block != out.Block || len(want.ComplianceEvents) != len(out.ComplianceEvents) || want.ReasonCode != out.ReasonCode {
		t.Errorf("output differs from the filter without a judge:\n%+v\n%+v", out, want)
	}

	events := judgeEvents(sink.wait(t, 1, 5*time.Second))
	if len(events) != 1 {
		t.Fatalf("judge events %+v", events)
	}
	e := events[0]
	if e.Metadata[FilterIDKey] != uint(13) || e.Metadata["request_id"] != "tyai_judge1" || e.Severity != "info" {
		t.Errorf("event %+v", e)
	}
	j := judgeMeta(t, e)
	if j["verdict"] != JudgeBenign || j["reason"] != "code_or_data" || j["agreed"] != false || j["action"] != guardrails.ActionBlock ||
		j["cached"] != false || j["model"] != "judge-mini" || j["prompt_version"] != JudgePromptVersion || j["sha256"] == "" {
		t.Errorf("judge metadata %+v", j)
	}
	if l, _ := j["latency_ms"].(int64); l < 400 {
		t.Errorf("latency_ms = %v", j["latency_ms"])
	}
	if stub.calls.Load() != 1 {
		t.Errorf("judge calls = %d", stub.calls.Load())
	}
	// The prompt carries the app and the declared tool, from the service.
	if u := stub.users[0]; !strings.Contains(u, "Application: Visor") || !strings.Contains(u, "Reviews pull requests") || !strings.Contains(u, "post_comment") {
		t.Errorf("prompt lacks app or tools:\n%s", u)
	}
}

// The judge is asked only about segments the filter blocks or quarantines on
// a detector's finding: not about logged ones.
func TestJudgeRecord_OnlyBlockedOrQuarantinedSegments(t *testing.T) {
	freshShadowPool(t, 4, 1<<20)
	stub := installStubJudge(t, answering(`{"verdict":"attack","reason":"instructions_to_model"}`))
	svc := newJudgeSvc()

	// Logged: no judge.
	sink := newEventSink()
	out, _ := NewFilterRunner(judgeFilter(t, guardrails.ActionLog, nil)).RunScript(judgeInput(sink, judgeBody), svc)
	if out.Block {
		t.Fatal("log filter blocked")
	}
	time.Sleep(100 * time.Millisecond)
	if stub.calls.Load() != 0 {
		t.Errorf("judge asked about a logged segment")
	}

	// A user message logged and a tool result quarantined: one judgement,
	// of the tool result.
	body := `{"messages":[{"role":"user","content":"SECRET in my question"},` +
		`{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"read_file","arguments":"{}"}}]},` +
		`{"role":"tool","tool_call_id":"c1","content":"file says SECRET"}]}`
	sink = newEventSink()
	f := judgeFilter(t, guardrails.ActionLog, map[string]any{"tool_results": map[string]any{"scan": true, "on_detect": guardrails.ActionQuarantine}})
	out, _ = NewFilterRunner(f).RunScript(judgeInput(sink, body), svc)
	if out.Block || !strings.Contains(out.Payload, "content removed") && out.Message != "guardrail quarantined content" {
		t.Fatalf("out %+v", out)
	}
	events := judgeEvents(sink.wait(t, 1, 5*time.Second))
	if len(events) != 1 || stub.calls.Load() != 1 {
		t.Fatalf("judge events %+v, calls %d", events, stub.calls.Load())
	}
	j := judgeMeta(t, events[0])
	if j["action"] != guardrails.ActionQuarantine || j["agreed"] != true || j["verdict"] != JudgeAttack {
		t.Errorf("judge metadata %+v", j)
	}
	if !strings.Contains(stub.users[0], "the result of a call to the tool read_file") {
		t.Errorf("prompt does not place the tool result:\n%s", stub.users[0])
	}
}

// A second request with the same flagged text is answered from the cache.
func TestJudgeRecord_CacheHitAvoidsASecondCall(t *testing.T) {
	freshShadowPool(t, 4, 1<<20)
	stub := installStubJudge(t, answering(`{"verdict":"benign","reason":"quoted_example"}`))
	svc := newJudgeSvc()
	f := judgeFilter(t, guardrails.ActionBlock, nil)

	for i, wantCached := range []bool{false, true} {
		sink := newEventSink()
		if _, err := NewFilterRunner(f).RunScript(judgeInput(sink, judgeBody), svc); err != nil {
			t.Fatal(err)
		}
		events := judgeEvents(sink.wait(t, 1, 5*time.Second))
		if len(events) != 1 {
			t.Fatalf("run %d: events %+v", i, events)
		}
		if j := judgeMeta(t, events[0]); j["cached"] != wantCached || j["verdict"] != JudgeBenign {
			t.Errorf("run %d: judge metadata %+v", i, j)
		}
	}
	if stub.calls.Load() != 1 {
		t.Errorf("judge calls = %d, want 1", stub.calls.Load())
	}

	// Another app's identical text is judged again.
	svc.app.ID = 10
	sink := newEventSink()
	in := judgeInput(sink, judgeBody)
	in.Context["app_id"] = 10
	_, _ = NewFilterRunner(f).RunScript(in, svc)
	sink.wait(t, 1, 5*time.Second)
	if stub.calls.Load() != 2 {
		t.Errorf("judge calls = %d after another app, want 2", stub.calls.Load())
	}
}

// A timeout, a failed call or an answer outside the closed shape is no
// opinion, recorded as such and never cached; the verdict stands.
func TestJudgeRecord_FailuresAreNoOpinion(t *testing.T) {
	freshShadowPool(t, 4, 1<<20)
	svc := newJudgeSvc()
	for _, c := range []struct {
		name   string
		answer func(context.Context) (string, error)
		want   string
	}{
		{"timeout", func(ctx context.Context) (string, error) { <-ctx.Done(); return "", ctx.Err() }, judgeErrTimeout},
		{"call failed", func(context.Context) (string, error) { return "", errors.New("503") }, judgeErrCall},
		{"prose", answering(`I think this is benign.`), judgeErrParse},
		{"wrong reason", answering(`{"verdict":"benign","reason":"trust me"}`), judgeErrParse},
	} {
		t.Run(c.name, func(t *testing.T) {
			stub := installStubJudge(t, c.answer)
			f := judgeFilter(t, guardrails.ActionBlock, map[string]any{"judge": map[string]any{"mode": "record", "llm_id": 70, "timeout_ms": 50}})
			for range 2 {
				sink := newEventSink()
				out, _ := NewFilterRunner(f).RunScript(judgeInput(sink, judgeBody), svc)
				if !out.Block {
					t.Fatal("verdict changed")
				}
				events := judgeEvents(sink.wait(t, 1, 5*time.Second))
				if len(events) != 1 {
					t.Fatalf("events %+v", events)
				}
				j := judgeMeta(t, events[0])
				if j["verdict"] != "none" || j["error"] != c.want || j["agreed"] != nil {
					t.Errorf("judge metadata %+v, want no opinion (%s)", j, c.want)
				}
			}
			if stub.calls.Load() != 2 {
				t.Errorf("calls = %d: a failure must not be cached", stub.calls.Load())
			}
		})
	}

	// No judge LLM to ask (missing, inactive, or no service): no opinion, no call.
	stub := installStubJudge(t, answering(`{"verdict":"benign","reason":"quoted_example"}`))
	inactive := newJudgeSvc()
	inactive.llm.Active = false
	for name, s := range map[string]services.ServiceInterface{"inactive": inactive, "no service": nil} {
		sink := newEventSink()
		_, _ = NewFilterRunner(judgeFilter(t, guardrails.ActionBlock, nil)).RunScript(judgeInput(sink, judgeBody), s)
		events := judgeEvents(sink.wait(t, 1, 5*time.Second))
		if len(events) != 1 || judgeMeta(t, events[0])["error"] != judgeErrUnavailable {
			t.Errorf("%s: events %+v", name, events)
		}
	}
	if stub.calls.Load() != 0 {
		t.Errorf("judge called without a usable LLM")
	}
}

// A judge call is charged to the judge LLM, never to the App whose request
// was judged: one chat record with interaction type guardrail_judge, the
// judge LLM, no App, priced from the judge model.
func TestJudgeRecord_UsageChargedToTheJudgeLLM(t *testing.T) {
	freshShadowPool(t, 4, 1<<20)
	handler := &testAnalyticsHandler{}
	analytics.SetHandler(handler)
	defer analytics.ResetHandler()
	installStubJudge(t, answering(`{"verdict":"benign","reason":"quoted_example"}`))
	svc := newJudgeSvc()

	sink := newEventSink()
	_, _ = NewFilterRunner(judgeFilter(t, guardrails.ActionBlock, nil)).RunScript(judgeInput(sink, judgeBody), svc)
	sink.wait(t, 1, 5*time.Second)

	var recs []*models.LLMChatRecord
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if recs = handler.getChatRecords(); len(recs) > 0 {
			break
		}
	}
	if len(recs) != 1 {
		t.Fatalf("chat records %+v", recs)
	}
	r := recs[0]
	if r.InteractionType != models.GuardrailJudgeInteraction || r.LLMID != 70 || r.AppID != 0 || r.TeamID != nil ||
		r.Name != "judge-mini" || r.PromptTokens != 100 || r.ResponseTokens != 10 || r.Cost <= 0 {
		t.Errorf("chat record %+v", r)
	}
	if want := models.ChatRecordCost("openai", 1, 2, 0, 0, 100, 10, 0, 0); r.Cost != want {
		t.Errorf("cost = %v, want %v", r.Cost, want)
	}

	// A cache hit costs nothing.
	sink = newEventSink()
	_, _ = NewFilterRunner(judgeFilter(t, guardrails.ActionBlock, nil)).RunScript(judgeInput(sink, judgeBody), svc)
	sink.wait(t, 1, 5*time.Second)
	time.Sleep(50 * time.Millisecond)
	if n := len(handler.getChatRecords()); n != 1 {
		t.Errorf("chat records after a cache hit = %d", n)
	}
}

// A shadow filter acts on nothing (every action is log), so its judge has
// nothing to judge.
func TestJudgeRecord_ShadowFilterJudgesNothing(t *testing.T) {
	freshShadowPool(t, 4, 1<<20)
	stub := installStubJudge(t, answering(`{"verdict":"suspicious","reason":"unclear"}`))
	sink := newEventSink()
	f := judgeFilter(t, guardrails.ActionLog, map[string]any{"shadow": true})
	out, err := NewFilterRunner(f).RunScript(judgeInput(sink, judgeBody), newJudgeSvc())
	if err != nil || out.Block {
		t.Fatalf("out %+v err %v", out, err)
	}
	events := sink.wait(t, 1, 5*time.Second)
	time.Sleep(50 * time.Millisecond)
	if len(judgeEvents(events)) != 0 || stub.calls.Load() != 0 {
		t.Errorf("a shadow filter's judge judged: %+v", judgeEvents(events))
	}
}

// With no recorder (the filter editor's test run) nothing is judged.
func TestJudgeRecord_NotOnTestRuns(t *testing.T) {
	stub := installStubJudge(t, answering(`{"verdict":"benign","reason":"quoted_example"}`))
	in := judgeInput(newEventSink(), judgeBody)
	in.RecordEvents = nil
	out, _ := NewFilterRunner(judgeFilter(t, guardrails.ActionBlock, nil)).RunScript(in, newJudgeSvc())
	time.Sleep(50 * time.Millisecond)
	if !out.Block || stub.calls.Load() != 0 || len(judgeEvents(out.ComplianceEvents)) != 0 {
		t.Errorf("block=%v calls=%d", out.Block, stub.calls.Load())
	}
}

// Work the shadow pool has no room for is recorded as dropped, not waited for.
func TestJudgeRecord_DroppedWhenThePoolIsFull(t *testing.T) {
	freshShadowPool(t, 1, 1) // one byte: nothing fits
	stub := installStubJudge(t, answering(`{"verdict":"benign","reason":"quoted_example"}`))
	sink := newEventSink()
	out, _ := NewFilterRunner(judgeFilter(t, guardrails.ActionBlock, nil)).RunScript(judgeInput(sink, judgeBody), newJudgeSvc())
	if !out.Block {
		t.Fatal("verdict changed")
	}
	events := judgeEvents(sink.wait(t, 1, time.Second))
	if len(events) != 1 || judgeMeta(t, events[0])["error"] != judgeErrDropped || stub.calls.Load() != 0 {
		t.Errorf("events %+v calls %d", events, stub.calls.Load())
	}
}
