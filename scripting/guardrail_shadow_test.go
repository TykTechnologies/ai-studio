package scripting

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// eventSink is a RecordEvents that collects what it is handed.
type eventSink struct {
	mu     sync.Mutex
	events []ComplianceEventOutput
	names  []string
	got    chan struct{}
}

func newEventSink() *eventSink { return &eventSink{got: make(chan struct{}, 1000)} }

func (s *eventSink) record(_ context.Context, filterName string, out *ScriptOutput) {
	s.mu.Lock()
	s.events = append(s.events, out.ComplianceEvents...)
	s.names = append(s.names, filterName)
	s.mu.Unlock()
	s.got <- struct{}{}
}

func (s *eventSink) wait(t *testing.T, n int, within time.Duration) []ComplianceEventOutput {
	t.Helper()
	deadline := time.After(within)
	for i := 0; i < n; i++ {
		select {
		case <-s.got:
		case <-deadline:
			t.Fatalf("got %d of %d recordings", i, n)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ComplianceEventOutput(nil), s.events...)
}

// freshShadowPool gives the test its own pool.
func freshShadowPool(t *testing.T, workers int, bytes int64) *shadowPool {
	t.Helper()
	p := newShadowPool(workers, bytes)
	shadowPoolMu.Lock()
	prev := sharedShadowPool
	sharedShadowPool = p
	shadowPoolMu.Unlock()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p.drain(ctx)
		shadowPoolMu.Lock()
		sharedShadowPool = prev
		shadowPoolMu.Unlock()
	})
	return p
}

// slowFake flags SECRET after sleeping, ignoring nothing but its context.
func slowFake(d time.Duration) *fakeProvider {
	return &fakeProvider{verdict: func(in guardrails.Input) guardrails.Verdict {
		time.Sleep(d)
		v := guardrails.Verdict{}
		for _, s := range in.Segments {
			if strings.Contains(s.Text, "SECRET") {
				v.Flagged = true
				v.Findings = append(v.Findings, guardrails.Finding{Detector: "secrets.test_key", Category: "secrets", Score: 1, Severity: "critical", SegmentIndex: s.Index})
			}
		}
		return v
	}}
}

const shadowBody = `{"messages":[{"role":"user","content":"my key is SECRET"},{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"read","arguments":"{}"}}]},{"role":"tool","tool_call_id":"c1","content":"tool says SECRET"}]}`

func shadowFilter(t *testing.T, p *fakeProvider, extra map[string]any) *models.Filter {
	t.Helper()
	cfg := installFake(t, fmt.Sprintf("fake_shadow_%d", shadowSeq.Add(1)), p, guardrails.ActionLog)
	cfg["timeout_ms"] = 5000
	for k, v := range extra {
		cfg[k] = v
	}
	return guardrailFilter("shadow", false, cfg)
}

var shadowSeq atomic.Uint32

// A shadow filter adds no provider latency: with a provider that takes 2 s,
// the request returns at once, unchanged, and the events arrive later with
// what an inline log run records.
func TestShadow_NoLatencyEventsArriveLater(t *testing.T) {
	freshShadowPool(t, 4, 1<<20)
	for _, response := range []bool{false, true} {
		sink := newEventSink()
		f := shadowFilter(t, slowFake(2*time.Second), map[string]any{"shadow": true})
		f.ResponseFilter = response
		in := &ScriptInput{RawInput: shadowBody, Format: "openai", IsResponse: response, Context: map[string]interface{}{"app_id": int64(3)}, RecordEvents: sink.record}
		if response {
			in.RawInput, in.Format = "the reply has SECRET", ""
		}
		start := time.Now()
		out, err := NewFilterRunner(f).RunScript(in, nil)
		if err != nil {
			t.Fatal(err)
		}
		if took := time.Since(start); took > 100*time.Millisecond {
			t.Errorf("response=%v: shadow run took %v", response, took)
		}
		if out.Block || out.Payload != in.RawInput || len(out.ComplianceEvents) != 0 {
			t.Errorf("response=%v: shadow output %+v", response, out)
		}
		events := sink.wait(t, 1, 5*time.Second)

		// Same events as the filter inline in log mode.
		inline := shadowFilter(t, slowFake(0), nil)
		inline.ResponseFilter = response
		in2 := *in
		in2.RecordEvents = nil
		want, err := NewFilterRunner(inline).RunScript(&in2, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != len(want.ComplianceEvents) || len(events) == 0 {
			t.Fatalf("response=%v: shadow events %+v, inline %+v", response, events, want.ComplianceEvents)
		}
		for i := range events {
			for _, k := range []string{"detector", "action", "roles", "kinds", "count"} {
				if !reflect.DeepEqual(events[i].Metadata[k], want.ComplianceEvents[i].Metadata[k]) {
					t.Errorf("response=%v: event %d %s = %v, inline %v", response, i, k, events[i].Metadata[k], want.ComplianceEvents[i].Metadata[k])
				}
			}
		}
		if sink.names[0] != f.Name {
			t.Errorf("recorded under %q", sink.names[0])
		}
	}
}

// Without a recorder (the Test panel) a shadow filter runs inline.
func TestShadow_InlineWithoutRecorder(t *testing.T) {
	f := shadowFilter(t, slowFake(0), map[string]any{"shadow": true})
	out, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: shadowBody, Format: "openai"}, nil)
	if err != nil || len(out.ComplianceEvents) == 0 || out.Block {
		t.Fatalf("out %+v err %v", out, err)
	}
}

// A full pool drops the evaluation, never waits: the drop is recorded as
// guardrail.unscanned (shadow_dropped) and the requests all return at once,
// under concurrent load.
func TestShadow_QueueFullDropsNeverBlocks(t *testing.T) {
	freshShadowPool(t, 1, 4096)
	sink := newEventSink()
	f := shadowFilter(t, slowFake(300*time.Millisecond), map[string]any{"shadow": true})
	var wg sync.WaitGroup
	var slow atomic.Int32
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			out, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: shadowBody, Format: "openai",
				Context: map[string]interface{}{"app_id": int64(i % 3)}, RecordEvents: sink.record}, nil)
			if err != nil || out.Block {
				t.Errorf("out %+v err %v", out, err)
			}
			if time.Since(start) > 100*time.Millisecond {
				slow.Add(1)
			}
		}()
	}
	wg.Wait()
	if slow.Load() > 0 {
		t.Errorf("%d requests waited", slow.Load())
	}
	sink.mu.Lock()
	dropped := 0
	for _, e := range sink.events {
		if e.EventType == "guardrail.unscanned" && e.Metadata["reason"] == unscannedShadowDropped {
			dropped++
			if d := e.Metadata["drop"]; d != shadowDropQueueFull && d != shadowDropTenantShare {
				t.Errorf("drop reason %v", d)
			}
		}
	}
	sink.mu.Unlock()
	if dropped == 0 {
		t.Error("no drop recorded with a 4 KiB queue and 40 requests")
	}
}

// One tenant cannot take more than half the queue.
func TestShadow_TenantShare(t *testing.T) {
	p := newShadowPool(0, 1000)
	block := func() {}
	if r := p.enqueue(&shadowJob{run: block, bytes: 400, tenant: "app_id:1"}); r != "" {
		t.Fatal(r)
	}
	if r := p.enqueue(&shadowJob{run: block, bytes: 200, tenant: "app_id:1"}); r != shadowDropTenantShare {
		t.Fatalf("second job for the tenant: %q", r)
	}
	if r := p.enqueue(&shadowJob{run: block, bytes: 400, tenant: "app_id:2"}); r != "" {
		t.Fatalf("another tenant: %q", r)
	}
	if r := p.enqueue(&shadowJob{run: block, bytes: 300, tenant: "app_id:3"}); r != shadowDropQueueFull {
		t.Fatalf("over the queue: %q", r)
	}
}

// A shutdown waits for queued work within its bound, then abandons the
// rest; work queued after it is dropped by the old pool.
func TestShadow_DrainBounded(t *testing.T) {
	p := newShadowPool(1, 1<<20)
	var ran atomic.Int32
	for range 5 {
		p.enqueue(&shadowJob{bytes: 1, run: func() { time.Sleep(200 * time.Millisecond); ran.Add(1) }})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if p.drain(ctx) {
		t.Error("drain reported every job finished")
	}
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Errorf("drain took %v", took)
	}
	if r := p.enqueue(&shadowJob{bytes: 1, run: func() {}}); r != shadowDropShutdown {
		t.Errorf("after drain: %q", r)
	}
	time.Sleep(700 * time.Millisecond)
	if n := ran.Load(); n >= 5 {
		t.Errorf("abandoned jobs still ran: %d", n)
	}
}

// A shadow filter that cannot even be prepared never changes the traffic,
// whatever its fail mode.
func TestShadow_PrepareFailureNeverBlocks(t *testing.T) {
	sink := newEventSink()
	name := fmt.Sprintf("fake_shadow_fail_%d", shadowSeq.Add(1))
	guardrails.RegisterSpec(guardrails.Spec{Name: name, DisplayName: name, OpenDetectors: true})
	guardrails.RegisterFactory(name, func(guardrails.Config) (guardrails.Provider, error) { return nil, errors.New("boom") })
	f := guardrailFilter("shadow-fail", false, models.JSONMap{"provider": name, "detectors": []any{map[string]any{"name": "x"}},
		"on_detect": "log", "fail_mode": "closed", "shadow": true})
	out, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: shadowBody, Format: "openai", RecordEvents: sink.record}, nil)
	if err != nil || out.Block {
		t.Fatalf("out %+v err %v", out, err)
	}
	if ev := sink.wait(t, 1, time.Second); len(ev) != 1 || ev[0].EventType != "guardrail.error" {
		t.Errorf("events %+v", ev)
	}
}

// Shadow is refused unless every action is log.
func TestShadow_RequiresLogOnly(t *testing.T) {
	base := func() guardrails.Config {
		return guardrails.Config{Provider: guardrails.ProviderBuiltin, Detectors: []guardrails.DetectorConfig{{Name: "injection"}}, OnDetect: guardrails.ActionLog, Shadow: true}
	}
	if _, err := guardrails.Normalize(base(), false); err != nil {
		t.Fatalf("log-only shadow refused: %v", err)
	}
	for name, mutate := range map[string]func(*guardrails.Config){
		"block":        func(c *guardrails.Config) { c.OnDetect = guardrails.ActionBlock },
		"redact":       func(c *guardrails.Config) { c.OnDetect = guardrails.ActionRedact },
		"tool results": func(c *guardrails.Config) { c.ToolResults.OnDetect = guardrails.ActionQuarantine },
		"tool calls":   func(c *guardrails.Config) { c.ToolCalls.OnDetect = guardrails.ActionBlock },
		"block, results log": func(c *guardrails.Config) {
			c.OnDetect = guardrails.ActionBlock
			c.ToolResults.OnDetect = guardrails.ActionLog
		},
	} {
		c := base()
		mutate(&c)
		if _, err := guardrails.Normalize(c, false); !errors.Is(err, guardrails.ErrInvalidConfig) || !strings.Contains(err.Error(), "shadow") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
