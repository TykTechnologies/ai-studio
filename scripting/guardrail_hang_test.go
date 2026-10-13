package scripting

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// stuckProvider ignores its context: a stuck call returns only when the
// test ends. The first stuckFirst calls are stuck, and every call while
// always is set.
type stuckProvider struct {
	caps       guardrails.Capabilities
	stuckFirst int32
	always     atomic.Bool
	release    chan struct{}
	calls      atomic.Int32
}

func (p *stuckProvider) Capabilities() guardrails.Capabilities { return p.caps }

func (p *stuckProvider) Classify(context.Context, guardrails.Input) (guardrails.Verdict, error) {
	if n := p.calls.Add(1); n <= p.stuckFirst || p.always.Load() {
		<-p.release
	}
	return guardrails.Verdict{}, nil
}

func newStuckProvider(t *testing.T, caps guardrails.Capabilities) *stuckProvider {
	p := &stuckProvider{caps: caps, release: make(chan struct{})}
	t.Cleanup(func() { close(p.release) })
	return p
}

// stuckFilter registers p under a fresh name and returns a saved filter
// version using it, with cfg merged over a block config.
func stuckFilter(t *testing.T, p *stuckProvider, response bool, cfg map[string]any) *models.Filter {
	t.Helper()
	id := scanFilterSeq.Add(1)
	name := fmt.Sprintf("fake_stuck_%d", id)
	guardrails.RegisterSpec(guardrails.Spec{Name: name, DisplayName: name, Detectors: []guardrails.DetectorSpec{{Name: "secrets", Category: "secrets"}}})
	guardrails.RegisterFactory(name, func(guardrails.Config) (guardrails.Provider, error) { return p, nil })
	c := models.JSONMap{
		"provider": name, "detectors": []any{map[string]any{"name": "secrets"}},
		"on_detect": guardrails.ActionBlock, "timeout_ms": 50, "cache": false,
	}
	for k, v := range cfg {
		c[k] = v
	}
	f := &models.Filter{Name: name, Kind: models.FilterKindGuardrail, ResponseFilter: response, Config: c}
	f.ID, f.UpdatedAt = uint(10000+id), time.Now()
	return f
}

func runResponse(t *testing.T, f *models.Filter, text string) *ScriptOutput {
	t.Helper()
	out, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: text, IsResponse: true}, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	return out
}

// errorClasses lists the error classes of the guardrail.error events, as
// providerError names them in the description.
func errorClasses(out *ScriptOutput) []string {
	const marker = "provider call failed: "
	var classes []string
	for _, e := range out.ComplianceEvents {
		if e.EventType != "guardrail.error" {
			continue
		}
		_, rest, ok := strings.Cut(e.Description, marker)
		if !ok {
			classes = append(classes, "")
			continue
		}
		class, _, _ := strings.Cut(rest, ";")
		classes = append(classes, class)
	}
	return classes
}

// A local provider that ignores its context is given up on shortly after
// timeout_ms, as a remote one is: it has no run-wide budget, but a call of
// a few bytes has no reason to take longer (midsommar-clean-4uc.49).
func TestGuardrailHang_LocalProviderIsAbandoned(t *testing.T) {
	// Stuck calls hold their slots until they return, as they should: give
	// the test slots enough for every run.
	useLocalLimiter(t, newLocalLimiter(64, 1<<30))
	p := newStuckProvider(t, guardrails.Capabilities{Local: true})
	p.always.Store(true)
	f := stuckFilter(t, p, false, map[string]any{"fail_mode": guardrails.FailClosed})

	start := time.Now()
	out := runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: "hello"}), app(1))
	if took := time.Since(start); took > time.Second {
		t.Errorf("the run took %v; it waited out a local provider that ignores its context", took)
	}
	if !out.Block {
		t.Error("fail closed let the request through when the provider never answered")
	}
}

// The response side has no run-wide budget either; its calls are given up
// on after timeout_ms, and the fail mode (open by default) decides.
func TestGuardrailHang_ResponseSideIsAbandoned(t *testing.T) {
	p := newStuckProvider(t, guardrails.Capabilities{})
	p.always.Store(true)
	f := stuckFilter(t, p, true, nil)

	start := time.Now()
	out := runResponse(t, f, "the model's answer")
	if took := time.Since(start); took > time.Second {
		t.Errorf("the run took %v; it waited out a provider that ignores its context", took)
	}
	if out.Block || out.Payload != "the model's answer" {
		t.Errorf("block=%v payload=%q, want fail open to pass the response", out.Block, out.Payload)
	}
	if got := errorClasses(out); len(got) != 1 || got[0] != "timeout" {
		t.Errorf("error classes = %v, want one timeout", got)
	}
}

// An abandoned call frees its slot: a run of more calls than run at once is
// not held up behind the calls it gave up on.
func TestGuardrailHang_AbandonedCallFreesItsSlot(t *testing.T) {
	p := newStuckProvider(t, guardrails.Capabilities{MaxChars: 40, MaxCallChars: 40})
	p.stuckFirst = scanParallel
	f := stuckFilter(t, p, true, nil)
	text := strings.Repeat("0123456789 ", 30) // eight or more calls of 40

	start := time.Now()
	runResponse(t, f, text)
	if took := time.Since(start); took > time.Second {
		t.Errorf("the run took %v; later calls waited for the stuck ones' slots", took)
	}
	if n := p.calls.Load(); n <= scanParallel {
		t.Errorf("%d calls made; the calls after the stuck ones never ran", n)
	}
}

// After breakerAbandons abandoned calls the filter's calls are suspended:
// the provider is not called, each run fails as an operational failure at
// once, and after the cool-down a single probe that answers resumes them.
func TestGuardrailHang_BreakerSuspendsAHangingProvider(t *testing.T) {
	now := time.Now()
	prev := breakerNow
	breakerNow = func() time.Time { return now }
	t.Cleanup(func() { breakerNow = prev })

	p := newStuckProvider(t, guardrails.Capabilities{})
	p.always.Store(true)
	f := stuckFilter(t, p, false, map[string]any{"fail_mode": guardrails.FailClosed})
	body := chatBody(t, chatMsg{Role: "user", Content: "hello"})

	for i := 0; i < breakerAbandons; i++ {
		if out := runRequest(t, f, body, app(int64(i%breakerTenants))); !out.Block {
			t.Fatalf("run %d: fail closed let the request through", i)
		}
	}
	if n := p.calls.Load(); n != breakerAbandons {
		t.Fatalf("%d calls before the breaker opened, want %d", n, breakerAbandons)
	}

	start := time.Now()
	out := runRequest(t, f, body, app(1))
	if took := time.Since(start); took > 40*time.Millisecond {
		t.Errorf("a suspended run took %v; it waited for the provider", took)
	}
	if !out.Block {
		t.Error("fail closed let the request through while the provider was suspended")
	}
	if got := errorClasses(out); len(got) != 1 || got[0] != "suspended" {
		t.Errorf("error classes = %v, want one suspended", got)
	}
	if n := p.calls.Load(); n != breakerAbandons {
		t.Errorf("the provider was called while suspended (%d calls)", n)
	}

	// A probe that hangs keeps the calls suspended for another cool-down.
	now = now.Add(breakerCooldown)
	runRequest(t, f, body, app(1))
	if n := p.calls.Load(); n != breakerAbandons+1 {
		t.Fatalf("%d calls after the cool-down, want one probe", n)
	}
	runRequest(t, f, body, app(1))
	if n := p.calls.Load(); n != breakerAbandons+1 {
		t.Errorf("the provider was called after a hung probe (%d calls)", n)
	}

	// A probe that answers resumes the calls.
	p.always.Store(false)
	now = now.Add(breakerCooldown)
	if out := runRequest(t, f, body, app(1)); out.Block {
		t.Errorf("the probe answered clean but the request was blocked: %v", eventTypes(out))
	}
	if out := runRequest(t, f, body, app(1)); out.Block || p.calls.Load() != breakerAbandons+3 {
		t.Errorf("block=%v calls=%d after a good probe, want calls resumed", out.Block, p.calls.Load())
	}
}

// Under fail open a suspended filter passes a small run, as any provider
// outage does, but text in a run that is not small is unscanned and takes
// the filter's action.
func TestGuardrailHang_SuspendedFollowsTheFailMode(t *testing.T) {
	now := time.Now()
	prev := breakerNow
	breakerNow = func() time.Time { return now }
	t.Cleanup(func() { breakerNow = prev })

	p := newStuckProvider(t, guardrails.Capabilities{})
	p.always.Store(true)
	f := stuckFilter(t, p, false, map[string]any{"fail_mode": guardrails.FailOpen, "budget": map[string]any{"single_call_chars": 100}})
	small := chatBody(t, chatMsg{Role: "user", Content: "hello"})
	for i := 0; i < breakerAbandons; i++ {
		runRequest(t, f, small, app(int64(i%breakerTenants)))
	}
	if out := runRequest(t, f, small, app(1)); out.Block {
		t.Error("fail open blocked a small run while the provider was suspended")
	}
	large := chatBody(t, chatMsg{Role: "user", Content: strings.Repeat("hello ", 50)})
	if out := runRequest(t, f, large, app(1)); !out.Block || unscannedReason(out) != unscannedProviderError {
		t.Errorf("block=%v unscanned=%q, want the large run's text unscanned and blocked", out.Block, unscannedReason(out))
	}
	if n := p.calls.Load(); n != breakerAbandons {
		t.Errorf("the provider was called while suspended (%d calls)", n)
	}
}

// Abandoned calls older than the window do not count towards opening the
// breaker, and a call that answers in time does not close an open one: only
// a probe does.
func TestCallBreaker_Window(t *testing.T) {
	now := time.Now()
	prev := breakerNow
	breakerNow = func() time.Time { return now }
	t.Cleanup(func() { breakerNow = prev })

	tenant := func(i int) string { return fmt.Sprintf("app_id:%d", i%breakerTenants) }
	b := &callBreaker{}
	for i := 0; i < breakerAbandons-1; i++ {
		b.done(false, true, tenant(i))
	}
	now = now.Add(breakerWindow)
	b.done(false, true, tenant(0))
	if ok, _ := b.allow(); !ok {
		t.Fatal("the breaker opened on abandons spread over more than its window")
	}
	for i := 1; i < breakerAbandons; i++ {
		b.done(false, true, tenant(i))
	}
	if ok, _ := b.allow(); ok {
		t.Fatal("the breaker stayed closed after breakerAbandons abandons within its window")
	}
	b.done(false, false, tenant(0)) // a call that was in flight when it opened
	if ok, _ := b.allow(); ok {
		t.Error("a call that was not a probe closed the breaker")
	}
	now = now.Add(breakerCooldown)
	ok, probe := b.allow()
	if !ok || !probe {
		t.Fatalf("allow() = %v, %v after the cool-down, want a probe", ok, probe)
	}
	if ok, _ := b.allow(); ok {
		t.Error("a second call went while the probe was out")
	}
	b.cancelProbe()
	if ok, probe := b.allow(); !ok || !probe {
		t.Errorf("allow() = %v, %v after a cancelled probe, want another probe", ok, probe)
	}

	var nilBreaker *callBreaker
	if ok, probe := nilBreaker.allow(); !ok || probe {
		t.Error("a filter without a breaker was refused")
	}
	nilBreaker.done(false, true, "")
}

// One tenant cannot open the breaker alone, however many small-run abandons
// it causes; abandons from several tenants, as an outage causes, do
// (midsommar-clean-4uc.59).
func TestGuardrailHang_BreakerNeedsSeveralTenants(t *testing.T) {
	p := newStuckProvider(t, guardrails.Capabilities{})
	p.always.Store(true)
	f := stuckFilter(t, p, false, map[string]any{"fail_mode": guardrails.FailClosed})
	body := chatBody(t, chatMsg{Role: "user", Content: "hello"})
	for range 20 {
		runRequest(t, f, body, app(1))
	}
	if n := p.calls.Load(); n != 20 {
		t.Fatalf("%d calls after one tenant's 20 abandons, want 20: it suspended the filter alone", n)
	}
	for i := range breakerAbandons {
		runRequest(t, f, body, app(int64(2+i%3)))
	}
	before := p.calls.Load()
	runRequest(t, f, body, app(9))
	if p.calls.Load() != before {
		t.Error("abandons from several tenants did not suspend the filter")
	}
}

// Abandons in large runs never count: a sender can make a provider slow with
// large inputs. The large run's text is unscanned.
func TestGuardrailHang_LargeRunAbandonsDoNotCount(t *testing.T) {
	p := newStuckProvider(t, guardrails.Capabilities{})
	p.always.Store(true)
	f := stuckFilter(t, p, false, map[string]any{"fail_mode": guardrails.FailOpen, "budget": map[string]any{"single_call_chars": 100}})
	large := chatBody(t, chatMsg{Role: "user", Content: strings.Repeat("hello ", 50)})
	for i := range 3 * breakerAbandons {
		out := runRequest(t, f, large, app(int64(i%4)))
		if !out.Block || unscannedReason(out) == "" {
			t.Fatalf("run %d: block=%v unscanned=%q, want the large run's text unscanned", i, out.Block, unscannedReason(out))
		}
	}
	if n := p.calls.Load(); n != 3*breakerAbandons {
		t.Errorf("%d calls, want %d: large-run abandons suspended the filter", n, 3*breakerAbandons)
	}
}

// A local provider's abandoned call is unscanned even in a small run under
// fail open, and local abandons never suspend the filter: a sender can cause
// them by loading the host.
func TestGuardrailHang_LocalAbandonIsUnscanned(t *testing.T) {
	// Stuck calls hold their slots until they return, as they should: give
	// the test slots enough for every run.
	useLocalLimiter(t, newLocalLimiter(64, 1<<30))
	p := newStuckProvider(t, guardrails.Capabilities{Local: true})
	p.always.Store(true)
	f := stuckFilter(t, p, false, map[string]any{"fail_mode": guardrails.FailOpen})
	body := chatBody(t, chatMsg{Role: "user", Content: "hello"})
	for i := range 2 * breakerAbandons {
		out := runRequest(t, f, body, app(int64(i%4)))
		if !out.Block || unscannedReason(out) != unscannedTimeBudget {
			t.Fatalf("run %d: block=%v unscanned=%q, want the text unscanned (time_budget), not passed by fail open", i, out.Block, unscannedReason(out))
		}
	}
	if n := p.calls.Load(); n != 2*breakerAbandons {
		t.Errorf("%d calls, want %d: local abandons suspended the filter", n, 2*breakerAbandons)
	}
}

// slowLocal is an in-process provider that takes d per call (honouring its
// context) and flags "SECRET".
type slowLocal struct {
	d     time.Duration
	calls atomic.Int32
}

func (*slowLocal) Capabilities() guardrails.Capabilities {
	return guardrails.Capabilities{Local: true}
}

func (s *slowLocal) Classify(ctx context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	s.calls.Add(1)
	select {
	case <-ctx.Done():
		return guardrails.Verdict{}, ctx.Err()
	case <-time.After(s.d):
	}
	v := guardrails.Verdict{}
	for _, seg := range in.Segments {
		if strings.Contains(seg.Text, "SECRET") {
			v.Flagged = true
			v.Findings = append(v.Findings, guardrails.Finding{Detector: "secrets.test_key", Category: "secrets", Score: 1, SegmentIndex: seg.Index})
		}
	}
	return v, nil
}

// One tenant flooding a local provider with large tool results does not
// starve another: the other tenant's small request gets a slot and a real
// verdict within its budget (midsommar-clean-4uc.59).
func TestGuardrailHang_LocalFloodDoesNotStarveOtherTenants(t *testing.T) {
	useLocalLimiter(t, newLocalLimiter(4, 1<<30))
	p := &slowLocal{d: 300 * time.Millisecond}
	id := scanFilterSeq.Add(1)
	name := fmt.Sprintf("fake_slow_local_%d", id)
	guardrails.RegisterSpec(guardrails.Spec{Name: name, DisplayName: name, Detectors: []guardrails.DetectorSpec{{Name: "secrets", Category: "secrets"}}})
	guardrails.RegisterFactory(name, func(guardrails.Config) (guardrails.Provider, error) { return p, nil })
	f := &models.Filter{Name: name, Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": name, "detectors": []any{map[string]any{"name": "secrets"}}, "on_detect": guardrails.ActionBlock,
		"timeout_ms": 2000, "cache": false,
	}}
	f.ID, f.UpdatedAt = uint(10000+id), time.Now()

	flood := chatBody(t, chatMsg{Role: "user", Content: "go"}, chatMsg{Role: "tool", Content: strings.Repeat("x", 1<<20), ToolCallID: "c"})
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = NewFilterRunner(f).RunScript(&ScriptInput{RawInput: flood, Format: "openai", VendorName: "openai", Context: app(1)}, nil)
		}()
	}
	time.Sleep(50 * time.Millisecond) // the flood holds its share of the slots

	start := time.Now()
	out := runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: "my SECRET"}), app(2))
	took := time.Since(start)
	if !out.Block || unscannedReason(out) != "" || !slices.Contains(eventTypes(out), "guardrail.secrets.test_key") {
		t.Errorf("block=%v unscanned=%q events=%v, want a real verdict on tenant 2's request", out.Block, unscannedReason(out), eventTypes(out))
	}
	if took > time.Second {
		t.Errorf("tenant 2's request took %v behind tenant 1's flood", took)
	}
	wg.Wait()
}
