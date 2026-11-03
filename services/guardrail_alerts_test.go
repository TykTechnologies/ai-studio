package services

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type alertSink struct {
	mu   sync.Mutex
	got  []GuardrailAlertPayload
	tops []string
}

func (a *alertSink) handler(ev eventbridge.Event) {
	var p GuardrailAlertPayload
	_ = json.Unmarshal(ev.Payload, &p)
	a.mu.Lock()
	a.got = append(a.got, p)
	a.tops = append(a.tops, ev.Topic)
	a.mu.Unlock()
}

func (a *alertSink) all() []GuardrailAlertPayload {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]GuardrailAlertPayload(nil), a.got...)
}

// alertFixture is an exception fixture whose filter alerts as
// "visor-pr-flag" (with evidence excerpts on, include_excerpt as given),
// with a bus that collects the alert topics named.
func alertFixture(t *testing.T, includeExcerpt bool, topics ...string) (*exceptionFixture, *alertSink) {
	t.Helper()
	f := newExceptionFixture(t, "read_only")
	f.filter.Config = models.JSONMap{
		"provider": guardrails.ProviderBuiltin, "detectors": []any{map[string]any{"name": guardrails.CategoryInjection}},
		"on_detect": guardrails.ActionAlert, "alert": map[string]any{"event": "visor-pr-flag", "include_excerpt": includeExcerpt},
		"evidence": map[string]any{"excerpt": true},
	}
	require.NoError(t, f.db.Save(f.filter).Error)
	bus := eventbridge.NewBus()
	f.svc.SystemEvents = NewSystemEventEmitter(bus, "test")
	sink := &alertSink{}
	for _, topic := range append([]string{"guardrail.alert.visor-pr-flag"}, topics...) {
		sub := bus.Subscribe(topic, sink.handler)
		t.Cleanup(func() { bus.Unsubscribe(sub) })
	}
	return f, sink
}

func storedEvent(t *testing.T, f *exceptionFixture, filterName, eventType string, meta map[string]any) *models.ComplianceEvent {
	t.Helper()
	b, err := json.Marshal(meta)
	require.NoError(t, err)
	ev := &models.ComplianceEvent{AppID: f.app.ID, FilterName: filterName, EventType: eventType, Severity: "warning", Metadata: string(b), TimeStamp: time.Now()}
	require.NoError(t, f.db.Create(ev).Error)
	return ev
}

func detectorMeta(f *exceptionFixture, extra map[string]any) map[string]any {
	m := map[string]any{
		"filter_id": f.filter.ID, "detector": "prompt_injection", "action": "log", "max_score": 0.97,
		"kinds": []string{"text"}, "roles": []string{"user"}, "request_id": "req-1",
		"segments": []map[string]any{{"index": 0, "sha256": "abc", "excerpt": "ignore all previous ***"}},
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func waitAlerts(t *testing.T, sink *alertSink, n int) []GuardrailAlertPayload {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := sink.all(); len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	return sink.all()
}

// One bus event per alerting compliance event, with the payload, and no
// flagged text by default; none for an event that does not alert.
func TestPublishGuardrailAlerts_OnePerAlertingEvent(t *testing.T) {
	f, sink := alertFixture(t, false)
	alerting := storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"alert": "visor-pr-flag"}))
	plain := storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, nil))

	f.svc.PublishGuardrailAlerts([]*models.ComplianceEvent{alerting, plain})
	got := waitAlerts(t, sink, 1)
	time.Sleep(20 * time.Millisecond)
	require.Len(t, sink.all(), 1)
	p := got[0]
	assert.Equal(t, "visor-pr-flag", p.Alert)
	assert.Equal(t, "guardrail.alert.visor-pr-flag", p.Topic)
	assert.Equal(t, f.filter.ID, p.FilterID)
	assert.Equal(t, f.filter.Name, p.FilterName)
	assert.Equal(t, f.app.ID, p.AppID)
	assert.Equal(t, "Visor", p.AppName)
	assert.Equal(t, "prompt_injection", p.Detector)
	assert.Equal(t, []string{"text"}, p.Kinds)
	assert.Equal(t, 0.97, p.MaxScore)
	assert.Equal(t, "log", p.Action)
	assert.Equal(t, "req-1", p.RequestID)
	assert.Equal(t, alerting.ID, p.ComplianceEventID)
	assert.Equal(t, fmt.Sprintf("/admin/compliance?event=%d", alerting.ID), p.Link)
	assert.NotEmpty(t, p.Timestamp)
	assert.Empty(t, p.Excerpts, "no flagged text by default")
}

func TestPublishGuardrailAlerts_ExcerptOnlyWhenOptedIn(t *testing.T) {
	f, sink := alertFixture(t, true)
	ev := storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"alert": "visor-pr-flag"}))
	f.svc.PublishGuardrailAlerts([]*models.ComplianceEvent{ev})
	got := waitAlerts(t, sink, 1)
	require.Len(t, got, 1)
	assert.Equal(t, []string{"ignore all previous ***"}, got[0].Excerpts)
}

// The metadata is not trusted: a name the hub does not configure for the
// event's filter or exception is not raised, for local and edge-ingested
// events alike (both are stored compliance events by the time they get
// here).
func TestPublishGuardrailAlerts_ForgedOrUnconfiguredNamesIgnored(t *testing.T) {
	f, sink := alertFixture(t, true, "guardrail.alert.someone-else", "guardrail.alert.visor-softened")
	other := &models.Filter{Name: "Other guardrail", Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": guardrails.ProviderBuiltin, "detectors": []any{map[string]any{"name": guardrails.CategoryInjection}},
		"on_detect": guardrails.ActionAlert, "alert": map[string]any{"event": "someone-else"},
	}}
	require.NoError(t, f.db.Create(other).Error)

	events := []*models.ComplianceEvent{
		// A name the filter does not configure.
		storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"alert": "someone-else"})),
		// Another filter's alert claimed under this filter's id.
		storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"alert": "someone-else", "alerts": []string{"someone-else"}})),
		// This filter's alert claimed by an event recorded under another name
		// (a script filter, or an edge lying about the filter).
		storedEvent(t, f, "script filter", "guardrail.prompt_injection", detectorMeta(f, map[string]any{"alert": "visor-pr-flag"})),
		// A script filter's id.
		storedEvent(t, f, f.script.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"filter_id": f.script.ID, "alert": "visor-pr-flag"})),
		// Not a guardrail event.
		storedEvent(t, f, f.filter.Name, "pii_redacted", detectorMeta(f, map[string]any{"alert": "visor-pr-flag"})),
		// An exception alert the event claims but no exception of this app
		// and filter names.
		storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"alert": "visor-softened", "exceptions": []uint{999}})),
	}
	f.svc.PublishGuardrailAlerts(events)
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, sink.all())
}

// An exception that softened the finding to alert raises its own event,
// without the flagged text even when the filter's own alert includes it.
func TestPublishGuardrailAlerts_FromAnException(t *testing.T) {
	f, sink := alertFixture(t, true, "guardrail.alert.visor-softened")
	s := f.spec("text_user", guardrails.ActionAlert)
	s.AlertEvent = ptr("visor-softened")
	e, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, s)
	require.NoError(t, err)

	ev := storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"alert": "visor-softened", "exceptions": []uint{e.ID}}))
	f.svc.PublishGuardrailAlerts([]*models.ComplianceEvent{ev})
	got := waitAlerts(t, sink, 1)
	require.Len(t, got, 1)
	assert.Equal(t, "guardrail.alert.visor-softened", got[0].Topic)
	assert.Equal(t, e.ID, got[0].ExceptionID)
	assert.Empty(t, got[0].Excerpts, "an exception's alert never carries the flagged text")

	// Both names on one event raise both, once each.
	both := storedEvent(t, f, f.filter.Name, "guardrail.prompt_injection", detectorMeta(f, map[string]any{"alert": "visor-pr-flag", "alerts": []string{"visor-pr-flag", "visor-softened"}, "exceptions": []uint{e.ID}}))
	f.svc.PublishGuardrailAlerts([]*models.ComplianceEvent{both})
	got = waitAlerts(t, sink, 3)
	assert.Len(t, got, 3)
}
