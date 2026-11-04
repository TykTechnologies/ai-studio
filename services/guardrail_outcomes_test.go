package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type outcomeSink struct {
	mu   sync.Mutex
	got  []GuardrailOutcomeEvent
	tops []string
}

func (o *outcomeSink) handler(ev eventbridge.Event) {
	var e GuardrailOutcomeEvent
	_ = json.Unmarshal(ev.Payload, &e)
	o.mu.Lock()
	o.got = append(o.got, e)
	o.tops = append(o.tops, ev.Topic)
	o.mu.Unlock()
}

func (o *outcomeSink) all() ([]GuardrailOutcomeEvent, []string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]GuardrailOutcomeEvent(nil), o.got...), append([]string(nil), o.tops...)
}

func (o *outcomeSink) wait(t *testing.T, n int) ([]GuardrailOutcomeEvent, []string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got, tops := o.all(); len(got) >= n {
			return got, tops
		}
		time.Sleep(5 * time.Millisecond)
	}
	return o.all()
}

// outcomeFixture is an exception fixture whose filter blocks, with its judge
// on judgeMode ("" for none), and a bus collecting guardrail.outcome.*.
func outcomeFixture(t *testing.T, judgeMode string) (*exceptionFixture, *outcomeSink) {
	t.Helper()
	f := newExceptionFixture(t, "read_only")
	f.filter.Config = models.JSONMap{
		"provider": guardrails.ProviderBuiltin, "detectors": []any{map[string]any{"name": guardrails.CategoryInjection}},
		"on_detect": guardrails.ActionBlock, "evidence": map[string]any{"excerpt": true},
	}
	if judgeMode != "" {
		f.filter.Config["judge"] = map[string]any{"mode": judgeMode, "llm_id": 5}
	}
	require.NoError(t, f.db.Save(f.filter).Error)
	bus := eventbridge.NewBus()
	f.svc.SystemEvents = NewSystemEventEmitter(bus, "test")
	sink := &outcomeSink{}
	for _, topic := range GuardrailOutcomeTopics {
		sub := bus.Subscribe(topic, sink.handler)
		t.Cleanup(func() { bus.Unsubscribe(sub) })
	}
	return f, sink
}

func judgeMeta(f *exceptionFixture, judge map[string]any) map[string]any {
	j := map[string]any{"llm_id": 5, "mode": "soften", "prompt_version": "guardrail-judge/2", "model": "claude-sonnet-5-5",
		"detectors": []string{"prompt_injection"}, "action": "block", "verdict": "benign", "reason": "code_or_data",
		"coverage": "full", "softened_from": "block", "softened_to": "log", "agreed": false}
	for k, v := range judge {
		j[k] = v
	}
	return map[string]any{"filter_id": f.filter.ID, "provider": "builtin", "judge": j, "action": "log", "blocked": false, "request_id": "req-9"}
}

// A block, a quarantine and a judge softening each raise their outcome
// event with the payload; a log, a fail-open error and the judge agreeing
// raise none.
func TestPublishGuardrailOutcomes_OnePerOutcome(t *testing.T) {
	f, sink := outcomeFixture(t, guardrails.JudgeSoften)
	blocked := storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"action": "block", "reason_code": "detected"}))
	quarantined := storedEvent(t, f, f.filter.Name, "guardrail.tool_result.critical", detectorMeta(f, map[string]any{"action": "quarantine", "kinds": []string{"tool_result"}, "tool_names": []string{"search"}, "exceptions": []uint{2}}))
	failClosed := storedEvent(t, f, f.filter.Name, "guardrail.error", map[string]any{"filter_id": f.filter.ID, "provider": "http", "fail_mode": "closed", "blocked": true})
	softened := storedEvent(t, f, f.filter.Name, "guardrail.judge", judgeMeta(f, nil))
	none := []*models.ComplianceEvent{
		storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, nil)),
		storedEvent(t, f, f.filter.Name, "guardrail.error", map[string]any{"filter_id": f.filter.ID, "fail_mode": "open", "blocked": false}),
		storedEvent(t, f, f.filter.Name, "guardrail.judge", judgeMeta(f, map[string]any{"verdict": "attack", "reason": "instructions_to_model", "softened_from": nil, "softened_to": nil, "agreed": true})),
	}

	f.svc.PublishGuardrailEvents(append([]*models.ComplianceEvent{blocked, quarantined, failClosed, softened}, none...))
	got, tops := sink.wait(t, 4)
	time.Sleep(20 * time.Millisecond)
	got, tops = sink.all()
	require.Len(t, got, 4, "topics %v", tops)
	assert.Equal(t, []string{"guardrail.outcome.blocked", "guardrail.outcome.quarantined", "guardrail.outcome.blocked", "guardrail.outcome.judge_softened"}, tops)

	b := got[0]
	assert.Equal(t, "guardrail", b.ObjectType)
	assert.Equal(t, "blocked", b.Action)
	assert.Equal(t, blocked.ID, b.ObjectID)
	p := b.Object
	assert.Equal(t, "blocked", p.Outcome)
	assert.Equal(t, "guardrail.outcome.blocked", p.Topic)
	assert.Equal(t, f.filter.ID, p.FilterID)
	assert.Equal(t, f.filter.Name, p.FilterName)
	assert.Equal(t, f.app.ID, p.AppID)
	assert.Equal(t, "Visor", p.AppName)
	assert.Equal(t, "guardrail.prompt_injection", p.EventType)
	assert.Equal(t, "prompt_injection", p.Detector)
	assert.Equal(t, "block", p.Action)
	assert.Equal(t, "detected", p.ReasonCode)
	assert.Equal(t, 0.97, p.MaxScore)
	assert.Equal(t, "req-1", p.RequestID)
	assert.Equal(t, fmt.Sprintf("/admin/compliance?event=%d", blocked.ID), p.Link)
	assert.Equal(t, f.filter.Name+": prompt_injection (app Visor)", p.Name)
	assert.Nil(t, p.Judge)

	q := got[1].Object
	assert.Equal(t, "quarantine", q.Action)
	assert.Equal(t, []string{"search"}, q.ToolNames)
	assert.Equal(t, []uint{2}, q.ExceptionIDs)

	e := got[2].Object
	assert.Equal(t, "block", e.Action, "a fail-closed error blocked")
	assert.Equal(t, f.filter.Name+": error (app Visor)", e.Name)

	s := got[3].Object
	assert.Equal(t, "judge_softened", s.Outcome)
	assert.Equal(t, "log", s.Action, "the action the traffic saw")
	assert.Equal(t, []string{"prompt_injection"}, s.Detectors)
	assert.Equal(t, "req-9", s.RequestID)
	require.NotNil(t, s.Judge)
	assert.Equal(t, GuardrailOutcomeJudge{LLMID: 5, Model: "claude-sonnet-5-5", PromptVersion: "guardrail-judge/2", Verdict: "benign", Reason: "code_or_data", Coverage: "full", SoftenedFrom: "block", SoftenedTo: "log"}, *s.Judge)
}

// The payload never carries the flagged text, though the event's segments
// hold an excerpt.
func TestPublishGuardrailOutcomes_NoFlaggedText(t *testing.T) {
	f, sink := outcomeFixture(t, "")
	ev := storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"action": "block"}))
	f.svc.PublishGuardrailOutcomes([]*models.ComplianceEvent{ev})
	got, _ := sink.wait(t, 1)
	require.Len(t, got, 1)
	b, err := json.Marshal(got[0])
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(b), "ignore all previous"), string(b))
}

// The metadata is not trusted: an event that does not come from the
// guardrail filter it names, a shadow filter's event, and a judge softening
// from a filter whose judge does not soften raise nothing.
func TestPublishGuardrailOutcomes_UntrustedOrShadowIgnored(t *testing.T) {
	f, sink := outcomeFixture(t, guardrails.JudgeRecord)
	block := map[string]any{"action": "block"}
	events := []*models.ComplianceEvent{
		// Recorded under another filter's name (a script filter, or an edge
		// lying about the filter).
		storedEvent(t, f, "script filter", "guardrail.prompt_injection", detectorMeta(f, block)),
		// A script filter's id.
		storedEvent(t, f, f.script.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"filter_id": f.script.ID, "action": "block"})),
		// No filter id at all (stripped from script filters' events).
		storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", map[string]any{"action": "block"}),
		// Not a guardrail event.
		storedEvent(t, f, f.filter.Name, "pii_redacted", detectorMeta(f, block)),
		// A shadow filter's event changes no traffic.
		storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"action": "block", "shadow": true})),
		// A softening from a judge the hub has on record only.
		storedEvent(t, f, f.filter.Name, "guardrail.judge", judgeMeta(f, nil)),
		// A judge event claiming record mode softened.
		storedEvent(t, f, f.filter.Name, "guardrail.judge", judgeMeta(f, map[string]any{"mode": "record"})),
	}
	f.svc.PublishGuardrailOutcomes(events)
	time.Sleep(50 * time.Millisecond)
	got, tops := sink.all()
	assert.Empty(t, got, "topics %v", tops)
}

// Alerts and outcomes are raised side by side: an event that both alerts
// and quarantines raises one of each.
func TestPublishGuardrailEvents_AlertAndOutcome(t *testing.T) {
	f, sink := outcomeFixture(t, "")
	f.filter.Config["tool_results"] = map[string]any{"on_detect": guardrails.ActionQuarantine}
	f.filter.Config["alert"] = map[string]any{"event": "visor-pr-flag"}
	f.filter.Config["on_detect"] = guardrails.ActionAlert
	require.NoError(t, f.db.Save(f.filter).Error)
	alerts := &alertSink{}
	bus := eventbridge.NewBus()
	f.svc.SystemEvents = NewSystemEventEmitter(bus, "test")
	for _, topic := range append([]string{"guardrail.alert.visor-pr-flag"}, GuardrailOutcomeTopics...) {
		h := sink.handler
		if topic == "guardrail.alert.visor-pr-flag" {
			h = alerts.handler
		}
		sub := bus.Subscribe(topic, h)
		t.Cleanup(func() { bus.Unsubscribe(sub) })
	}
	ev := storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"action": "quarantine", "alert": "visor-pr-flag"}))
	f.svc.PublishGuardrailEvents([]*models.ComplianceEvent{ev})
	_, tops := sink.wait(t, 1)
	assert.Equal(t, []string{"guardrail.outcome.quarantined"}, tops)
	assert.Len(t, waitAlerts(t, alerts, 1), 1)
}

// Alerts and outcome events link to their compliance event: link is the
// Compliance page's path, url the same under SITE_URL (BASE_PATH included),
// and no url without an http(s) SITE_URL.
func TestGuardrailEvents_LinkToTheEvent(t *testing.T) {
	prev := config.Get("")
	t.Cleanup(func() { config.Set(prev) })
	for _, c := range []struct{ site, want string }{
		{"https://studio.example.com/ai-studio/", "https://studio.example.com/ai-studio/admin/compliance?event=%d"},
		{"http://localhost:3000", "http://localhost:3000/admin/compliance?event=%d"},
		{"", ""},
		{"studio.example.com", ""},
		{"javascript:alert(1)", ""},
	} {
		t.Run(c.site, func(t *testing.T) {
			conf := *prev
			conf.SiteURL = c.site
			config.Set(&conf)

			f, sink := outcomeFixture(t, "")
			f.filter.Config["on_detect"] = guardrails.ActionAlert
			f.filter.Config["alert"] = map[string]any{"event": "visor-pr-flag"}
			require.NoError(t, f.db.Save(f.filter).Error)
			alerts := &alertSink{}
			sub := f.svc.SystemEvents.bus.Subscribe("guardrail.alert.visor-pr-flag", alerts.handler)
			t.Cleanup(func() { f.svc.SystemEvents.bus.Unsubscribe(sub) })

			ev := storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"action": "quarantine", "alert": "visor-pr-flag"}))
			f.svc.PublishGuardrailEvents([]*models.ComplianceEvent{ev})
			got, _ := sink.wait(t, 1)
			al := waitAlerts(t, alerts, 1)
			require.Len(t, got, 1)
			require.Len(t, al, 1)
			link := fmt.Sprintf("/admin/compliance?event=%d", ev.ID)
			want := ""
			if c.want != "" {
				want = fmt.Sprintf(c.want, ev.ID)
			}
			assert.Equal(t, link, got[0].Object.Link)
			assert.Equal(t, want, got[0].Object.URL)
			assert.Equal(t, link, al[0].Link)
			assert.Equal(t, want, al[0].URL)
		})
	}
}
