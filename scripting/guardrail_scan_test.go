package scripting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/toolfixtures"
)

// scanFake flags every segment containing "SECRET" or a tool fixture canary
// and records each call. Unlike fakeProvider it is safe for the concurrent
// calls of one run.
type scanFake struct {
	caps guardrails.Capabilities
	// fail, when set, decides the error of a call.
	fail func(in guardrails.Input) error
	// delay holds each call back, honouring the context.
	delay time.Duration
	// unpinned reports every finding without a segment index.
	unpinned bool

	mu    sync.Mutex
	calls []guardrails.Input
}

func (f *scanFake) Capabilities() guardrails.Capabilities { return f.caps }

func (f *scanFake) Classify(ctx context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	f.mu.Lock()
	f.calls = append(f.calls, in)
	f.mu.Unlock()
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return guardrails.Verdict{}, ctx.Err()
		case <-time.After(f.delay):
		}
	}
	if f.fail != nil {
		if err := f.fail(in); err != nil {
			return guardrails.Verdict{}, err
		}
	}
	v := guardrails.Verdict{}
	for _, s := range in.Segments {
		if !strings.Contains(s.Text, "SECRET") && !strings.Contains(s.Text, toolfixtures.CanaryPrefix) {
			continue
		}
		idx := s.Index
		if f.unpinned {
			idx = -1
		}
		v.Findings = append(v.Findings, guardrails.Finding{
			Detector: "secrets.test_key", Category: "secrets", Label: "Test key",
			Score: 1, Severity: "warning", SegmentIndex: idx,
		})
		if in.Redact != nil {
			if v.Rewritten == nil {
				v.Rewritten = map[int]string{}
			}
			v.Rewritten[s.Index] = strings.ReplaceAll(s.Text, "SECRET", "[REDACTED]")
		}
	}
	return v, nil
}

// sent lists every text the provider was sent, in call order.
func (f *scanFake) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, in := range f.calls {
		for _, s := range in.Segments {
			out = append(out, s.Text)
		}
	}
	return out
}

func (f *scanFake) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *scanFake) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

var scanFilterSeq atomic.Uint32

// scanFilter registers p under a fresh name and returns a saved filter
// version using it, with cfg merged over a block config.
func scanFilter(t testing.TB, p *scanFake, cfg map[string]any) *models.Filter {
	t.Helper()
	id := scanFilterSeq.Add(1)
	name := fmt.Sprintf("fake_scan_%d", id)
	guardrails.RegisterSpec(guardrails.Spec{
		Name: name, DisplayName: name, Redacts: true,
		Detectors: []guardrails.DetectorSpec{{Name: "secrets", Category: "secrets"}},
	})
	guardrails.RegisterFactory(name, func(guardrails.Config) (guardrails.Provider, error) { return p, nil })
	c := models.JSONMap{
		"provider":  name,
		"detectors": []any{map[string]any{"name": "secrets"}},
		"on_detect": guardrails.ActionBlock,
	}
	for k, v := range cfg {
		c[k] = v
	}
	f := &models.Filter{Name: name, Kind: models.FilterKindGuardrail, Config: c}
	f.ID, f.UpdatedAt = uint(10000+id), time.Now()
	return f
}

// freshVerdictCache gives the test its own verdict cache.
func freshVerdictCache(t *testing.T, ttl time.Duration) {
	t.Helper()
	verdictCacheMu.Lock()
	prev := sharedVerdictCache
	sharedVerdictCache = newVerdictCache(1000, 1<<20, ttl)
	verdictCacheMu.Unlock()
	t.Cleanup(func() {
		verdictCacheMu.Lock()
		sharedVerdictCache = prev
		verdictCacheMu.Unlock()
	})
}

type chatMsg struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

func chatBody(t *testing.T, msgs ...chatMsg) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"model": "m", "messages": msgs})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func runRequest(t *testing.T, f *models.Filter, body string, scope map[string]interface{}) *ScriptOutput {
	t.Helper()
	out, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: body, Format: "openai", VendorName: "openai", Context: scope}, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	return out
}

func app(id int64) map[string]interface{} { return map[string]interface{}{"app_id": id} }

func eventTypes(out *ScriptOutput) []string {
	var types []string
	for _, e := range out.ComplianceEvents {
		types = append(types, e.EventType)
	}
	slices.Sort(types)
	return types
}

// unscannedMeta is the metadata of the first guardrail.unscanned event.
func unscannedMeta(t *testing.T, out *ScriptOutput) map[string]interface{} {
	t.Helper()
	for _, e := range out.ComplianceEvents {
		if e.EventType == "guardrail.unscanned" {
			return e.Metadata
		}
	}
	t.Fatalf("no guardrail.unscanned event in %v", eventTypes(out))
	return nil
}

func unscannedReason(out *ScriptOutput) string {
	for _, e := range out.ComplianceEvents {
		if e.EventType == "guardrail.unscanned" {
			return e.Metadata["reason"].(string)
		}
	}
	return ""
}

// An agent loop resends its history every turn; with the cache the provider
// sees each segment once, and the verdicts for the history still apply.
func TestGuardrailScan_AgentLoopScansEachSegmentOnce(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	fx := toolfixtures.Get("openai_agent_loop")
	var full map[string]any
	if err := json.Unmarshal(fx.Body, &full); err != nil {
		t.Fatal(err)
	}
	msgs := full["messages"].([]any)
	turn := func(n int) string {
		b := map[string]any{}
		for k, v := range full {
			b[k] = v
		}
		b["messages"] = msgs[:n]
		out, _ := json.Marshal(b)
		return string(out)
	}

	p := &scanFake{}
	f := scanFilter(t, p, map[string]any{"scope": guardrails.ScopeAllMessages, "on_detect": guardrails.ActionLog})

	first := runRequest(t, f, turn(4), app(1))
	if !slices.Contains(p.sent(), msgs[3].(map[string]any)["content"].(string)) {
		t.Fatalf("turn 1: provider saw %q, want the poisoned tool result", p.sent())
	}
	if got := eventTypes(first); !slices.Equal(got, []string{"guardrail.secrets.test_key"}) {
		t.Fatalf("turn 1 events = %v", got)
	}

	p.reset()
	second := runRequest(t, f, turn(len(msgs)), app(1))
	sent := p.sent()
	if len(sent) != 2 {
		t.Fatalf("turn 2: provider saw %d segments %q, want only the 2 new ones", len(sent), sent)
	}
	for _, s := range sent {
		if strings.Contains(s, toolfixtures.CanaryPrefix) {
			t.Errorf("turn 2: the history's poisoned tool result was sent again")
		}
	}
	if got := eventTypes(second); !slices.Equal(got, []string{"guardrail.secrets.test_key"}) {
		t.Errorf("turn 2 events = %v, want the cached finding for the history", got)
	}

	// The same request again costs nothing.
	p.reset()
	runRequest(t, f, turn(len(msgs)), app(1))
	if n := p.callCount(); n != 0 {
		t.Errorf("repeat: provider called %d times, want 0", n)
	}
}

// What a verdict depends on is in the key: another filter version, another
// filter with the same config, another tenant all miss.
func TestGuardrailScan_CacheKeyScope(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	body := chatBody(t, chatMsg{Role: "user", Content: "hello SECRET"})
	p := &scanFake{}
	f := scanFilter(t, p, map[string]any{"on_detect": guardrails.ActionLog})

	runRequest(t, f, body, app(1))
	runRequest(t, f, body, app(1))
	if n := p.callCount(); n != 1 {
		t.Fatalf("same filter, app and text: %d calls, want 1", n)
	}

	runRequest(t, f, body, app(2))
	if n := p.callCount(); n != 2 {
		t.Errorf("another app: %d calls, want a miss", n)
	}

	edited := *f
	edited.UpdatedAt = f.UpdatedAt.Add(time.Second)
	edited.Config = models.JSONMap{}
	for k, v := range f.Config {
		edited.Config[k] = v
	}
	edited.Config["detectors"] = []any{map[string]any{"name": "secrets", "threshold": 0.9}}
	runRequest(t, &edited, body, app(1))
	if n := p.callCount(); n != 3 {
		t.Errorf("edited filter: %d calls, want a miss", n)
	}

	other := *f
	other.ID++
	runRequest(t, &other, body, app(1))
	if n := p.callCount(); n != 4 {
		t.Errorf("another filter with the same config: %d calls, want a miss", n)
	}

	// No tenant: the cache is not used at all.
	runRequest(t, f, body, nil)
	runRequest(t, f, body, nil)
	if n := p.callCount(); n != 6 {
		t.Errorf("no tenant: %d calls, want every run to reach the provider", n)
	}

	// Chat sessions are scoped by user.
	runRequest(t, f, body, map[string]interface{}{"user_id": int64(7), "session_id": "s1"})
	runRequest(t, f, body, map[string]interface{}{"user_id": int64(7), "session_id": "s2"})
	runRequest(t, f, body, map[string]interface{}{"user_id": int64(8), "session_id": "s1"})
	if n := p.callCount(); n != 8 {
		t.Errorf("chat users: %d calls, want one per user", n)
	}
}

// A connection value that changes under the same version stamp (an edge's
// next snapshot after the hub rotated a secret) reaches the provider on the
// next request, whatever its length (midsommar-clean-01t).
func TestGuardrailScan_RotatedConnectionReachesProvider(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	var built []string
	var mu sync.Mutex
	id := scanFilterSeq.Add(1)
	name := fmt.Sprintf("fake_rotate_%d", id)
	guardrails.RegisterSpec(guardrails.Spec{
		Name: name, DisplayName: name,
		Detectors:        []guardrails.DetectorSpec{{Name: "secrets", Category: "secrets"}},
		ConnectionFields: []guardrails.ConnectionField{{Name: "api_key", Secret: true}},
	})
	guardrails.RegisterFactory(name, func(cfg guardrails.Config) (guardrails.Provider, error) {
		mu.Lock()
		built = append(built, cfg.Connection["api_key"])
		mu.Unlock()
		return &scanFake{}, nil
	})
	f := &models.Filter{Name: name, Kind: models.FilterKindGuardrail}
	f.ID, f.UpdatedAt = uint(10000+id), time.Now()
	withKey := func(key string) models.JSONMap {
		return models.JSONMap{
			"provider": name, "detectors": []any{map[string]any{"name": "secrets"}}, "on_detect": guardrails.ActionLog,
			"connection": map[string]any{"api_key": key},
		}
	}
	body := chatBody(t, chatMsg{Role: "user", Content: "hello"})

	f.Config = withKey("lk-OLDOLDOLD")
	runRequest(t, f, body, app(1))
	f.Config = withKey("lk-NEWNEWNEW")
	runRequest(t, f, body, app(1))

	mu.Lock()
	defer mu.Unlock()
	if len(built) != 2 || built[1] != "lk-NEWNEWNEW" {
		t.Errorf("providers built with %q, want the rotated key used at once", built)
	}
}

func TestGuardrailScan_CacheSwitchedOff(t *testing.T) {
	body := chatBody(t, chatMsg{Role: "user", Content: "hello"})
	cases := map[string]func(*scanFake) *models.Filter{
		"cache false": func(p *scanFake) *models.Filter {
			return scanFilter(t, p, map[string]any{"cache": false})
		},
		"contextual provider": func(p *scanFake) *models.Filter {
			p.caps.Contextual = true
			return scanFilter(t, p, nil)
		},
		"test endpoint filter": func(p *scanFake) *models.Filter {
			f := scanFilter(t, p, nil)
			f.ID, f.UpdatedAt = 0, time.Time{}
			return f
		},
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			freshVerdictCache(t, time.Minute)
			p := &scanFake{}
			f := mk(p)
			runRequest(t, f, body, app(1))
			runRequest(t, f, body, app(1))
			if n := p.callCount(); n != 2 {
				t.Errorf("%d calls, want no caching", n)
			}
		})
	}

	t.Run("ttl 0", func(t *testing.T) {
		verdictCacheMu.Lock()
		prev := sharedVerdictCache
		sharedVerdictCache = nil
		verdictCacheMu.Unlock()
		t.Cleanup(func() {
			verdictCacheMu.Lock()
			sharedVerdictCache = prev
			verdictCacheMu.Unlock()
		})
		t.Setenv(EnvVerdictCacheTTL, "0")
		p := &scanFake{}
		f := scanFilter(t, p, nil)
		runRequest(t, f, body, app(1))
		runRequest(t, f, body, app(1))
		if n := p.callCount(); n != 2 {
			t.Errorf("%d calls, want the cache off", n)
		}
	})
}

// A failed call stores nothing: the next request asks the provider again,
// and the fail mode applies exactly as before.
func TestGuardrailScan_ErrorsAreNotCached(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	var failing atomic.Bool
	failing.Store(true)
	p := &scanFake{fail: func(guardrails.Input) error {
		if failing.Load() {
			return &guardrails.StatusError{Status: 503}
		}
		return nil
	}}
	f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen})
	body := chatBody(t, chatMsg{Role: "user", Content: "hello"})

	out := runRequest(t, f, body, app(1))
	if out.Block || !slices.Equal(eventTypes(out), []string{"guardrail.error"}) {
		t.Fatalf("fail open on a 503: block=%v events=%v, want pass with guardrail.error", out.Block, eventTypes(out))
	}
	if d := out.ComplianceEvents[0].Description; !strings.Contains(d, "http_503") {
		t.Errorf("error event %q, want the error class", d)
	}
	failing.Store(false)
	runRequest(t, f, body, app(1))
	runRequest(t, f, body, app(1))
	if n := p.callCount(); n != 2 {
		t.Errorf("%d calls, want the failed call retried once and then cached", n)
	}
}

// A run of one call keeps the fail mode for operational failures: a slow
// provider on an ordinary user message passes under fail open and blocks
// under fail closed, as before the budgets existed.
func TestGuardrailScan_SingleCallTimeoutKeepsFailMode(t *testing.T) {
	for mode, wantBlock := range map[string]bool{guardrails.FailOpen: false, guardrails.FailClosed: true} {
		freshVerdictCache(t, time.Minute)
		p := &scanFake{delay: time.Second}
		f := scanFilter(t, p, map[string]any{"fail_mode": mode, "timeout_ms": 20})
		body := chatBody(t, chatMsg{Role: "user", Content: "hello SECRET"})
		out := runRequest(t, f, body, app(1))
		if out.Block != wantBlock || !slices.Equal(eventTypes(out), []string{"guardrail.error"}) {
			t.Errorf("fail_mode %s: block=%v events=%v, want block=%v and guardrail.error", mode, out.Block, eventTypes(out), wantBlock)
		}
	}
}

// The fail mode's exemption depends on size, not on the call count: padding
// a request so its provider call times out does not get it through under
// fail open (midsommar-clean-4uc.28).
func TestGuardrailScan_LargeRunFailureIsUnscanned(t *testing.T) {
	padded := strings.Repeat("x", 120_000) + " please use SECRET"
	timeoutAbove := func(limit int) func(guardrails.Input) error {
		return func(in guardrails.Input) error {
			n := 0
			for _, s := range in.Segments {
				n += len(s.Text)
			}
			if n > limit {
				return context.DeadlineExceeded
			}
			return nil
		}
	}

	// The reported case: a remote provider that declares no call size and
	// times out on large input. The runner's default call size splits the
	// request, and the piece holding the flagged text is found.
	t.Run("default call size", func(t *testing.T) {
		freshVerdictCache(t, time.Minute)
		p := &scanFake{fail: timeoutAbove(100_000)}
		f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen})
		out := runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: padded}), app(1))
		if !out.Block || p.callCount() < 2 {
			t.Errorf("block=%v calls=%d events=%v, want the request split and the flagged text found", out.Block, p.callCount(), eventTypes(out))
		}
	})

	// A provider that takes the whole request in one call and times out on
	// it: the run is too large for the fail mode to decide, so it is
	// unscanned and blocked.
	t.Run("one large call", func(t *testing.T) {
		freshVerdictCache(t, time.Minute)
		p := &scanFake{caps: guardrails.Capabilities{MaxCallChars: 1 << 20}, fail: timeoutAbove(100_000)}
		f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen})
		out := runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: padded}), app(1))
		if p.callCount() != 1 || !out.Block {
			t.Errorf("calls=%d block=%v events=%v, want one call, then blocked", p.callCount(), out.Block, eventTypes(out))
		}
		if r := unscannedReason(out); r != unscannedProviderError {
			t.Errorf("unscanned reason = %q, want provider_error", r)
		}
		// The event says why the fail mode did not let it through.
		m := unscannedMeta(t, out)
		if m["error_class"] != "timeout" || m["fail_mode"] != guardrails.FailOpen || m["fail_mode_skipped"] != notSmallChars ||
			m["run_chars"].(int) <= m["single_call_chars"].(int) {
			t.Errorf("metadata = %v, want error_class timeout, fail_mode open, skipped for run_chars", m)
		}
		if seg := m["segments"].([]map[string]any)[0]; seg["excerpt_status"] != excerptNotScanned {
			t.Errorf("segment = %v, want excerpt_status not_scanned", seg)
		}
	})

	// An HTTP failure names its status.
	t.Run("http status", func(t *testing.T) {
		freshVerdictCache(t, time.Minute)
		p := &scanFake{caps: guardrails.Capabilities{MaxCallChars: 1 << 20}, fail: func(guardrails.Input) error { return &guardrails.StatusError{Status: 502} }}
		f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen})
		out := runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: padded}), app(1))
		if m := unscannedMeta(t, out); !out.Block || m["error_class"] != "http_502" {
			t.Errorf("block=%v metadata=%v, want blocked with error_class http_502", out.Block, m)
		}
	})

	// A tool result padded so its call times out is quarantined, not passed,
	// and the user's prompt in the same request goes through.
	t.Run("padded tool result", func(t *testing.T) {
		freshVerdictCache(t, time.Minute)
		p := &scanFake{fail: func(in guardrails.Input) error {
			for _, s := range in.Segments {
				if strings.HasPrefix(s.Text, "xxxx") {
					return context.DeadlineExceeded
				}
			}
			return nil
		}}
		f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen, "on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages})
		out := runRequest(t, f, chatBody(t,
			chatMsg{Role: "user", Content: "summarise the page"},
			chatMsg{Role: "tool", Content: padded, ToolCallID: "call_1"},
		), app(1))
		if out.Block || strings.Contains(out.Payload, "please use SECRET") || !strings.Contains(out.Payload, "summarise the page") {
			t.Errorf("block=%v, want the padded tool result quarantined and the prompt kept", out.Block)
		}
	})
}

// Padding a segment past the scan limit does not get it through under fail
// open: it is unscanned and the filter's action applies, before any call.
func TestGuardrailScan_OversizedSegmentIsNotPassed(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &scanFake{}
	f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen, "budget": map[string]any{"max_scan_chars": 100}})
	padded := strings.Repeat("x", 250) + " SECRET"
	body := chatBody(t, chatMsg{Role: "user", Content: "short"}, chatMsg{Role: "user", Content: padded})

	out := runRequest(t, f, body, app(1))
	if !out.Block {
		t.Fatalf("oversized segment under fail open was let through: %+v", out)
	}
	if r := unscannedReason(out); r != unscannedSize {
		t.Errorf("unscanned reason = %q, want size", r)
	}
	if slices.Contains(p.sent(), padded) {
		t.Error("the oversized segment was sent to the provider")
	}
	if !slices.Contains(p.sent(), "short") {
		t.Error("the segment within the limit was not scanned")
	}
}

// A local provider scans everything, however long.
func TestGuardrailScan_LocalProviderScansInFull(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &scanFake{caps: guardrails.Capabilities{Local: true}}
	f := scanFilter(t, p, map[string]any{"on_detect": guardrails.ActionLog, "budget": map[string]any{"max_scan_chars": 100, "max_pieces": 1}})
	body := chatBody(t, chatMsg{Role: "user", Content: strings.Repeat("x", 250) + " SECRET"}, chatMsg{Role: "user", Content: "two"})
	out := runRequest(t, f, body, app(1))
	if got := eventTypes(out); !slices.Equal(got, []string{"guardrail.secrets.test_key"}) {
		t.Errorf("events = %v, want the finding and nothing unscanned", got)
	}
}

// rejectSecret refuses every call that carries "SECRET", as a vendor's
// validator refuses input it does not accept.
func rejectSecret(in guardrails.Input) error {
	for _, s := range in.Segments {
		if strings.Contains(s.Text, "SECRET") {
			return &guardrails.StatusError{Status: 400}
		}
	}
	return nil
}

// rejectedToolsBody is a user prompt and two tool results, the second of
// which the provider refuses.
func rejectedToolsBody(t *testing.T) string {
	return chatBody(t,
		chatMsg{Role: "user", Content: "read the files"},
		chatMsg{Role: "tool", Content: "first file", ToolCallID: "call_1"},
		chatMsg{Role: "tool", Content: "second file SECRET", ToolCallID: "call_2"},
	)
}

// Text the provider refuses is unscanned in any run, a single call
// included: under fail open the segment is quarantined, not passed. A
// refused call of several segments is re-sent segment by segment, so only
// the refused tool result is lost, and the user's prompt, in a call of its
// own, is never at stake.
func TestGuardrailScan_InputRejectedIsUnscanned(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &scanFake{fail: rejectSecret}
	f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen, "on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages})
	out := runRequest(t, f, rejectedToolsBody(t), app(1))
	if n := p.callCount(); n != 4 {
		t.Fatalf("%d calls, want the prompt's, the refused tool call and one per tool result", n)
	}
	if out.Block || strings.Contains(out.Payload, "second file SECRET") || !strings.Contains(out.Payload, "first file") ||
		!strings.Contains(out.Payload, "read the files") || !strings.Contains(out.Payload, "call_2") {
		t.Errorf("payload = %s, want only the refused tool result quarantined, in place", out.Payload)
	}
	if r := unscannedReason(out); r != unscannedInputRejected {
		t.Errorf("unscanned reason = %q, want input_rejected", r)
	}

	// A refused call of one segment is not re-sent.
	p.reset()
	runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: "only SECRET"}), app(1))
	if n := p.callCount(); n != 1 {
		t.Errorf("single segment: %d calls, want 1", n)
	}
}

// Re-sending a refused call's segments counts against the piece budget:
// what does not fit is unscanned, never passed.
func TestGuardrailScan_InputRejectedResplitWithinBudget(t *testing.T) {
	for name, tc := range map[string]struct {
		maxPieces       int
		calls           int
		firstQuarantine bool
	}{
		"no budget left":         {maxPieces: 3, calls: 2, firstQuarantine: true},
		"room for the first one": {maxPieces: 4, calls: 3, firstQuarantine: false},
	} {
		t.Run(name, func(t *testing.T) {
			freshVerdictCache(t, time.Minute)
			p := &scanFake{fail: rejectSecret}
			f := scanFilter(t, p, map[string]any{
				"fail_mode": guardrails.FailOpen, "on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages,
				"budget": map[string]any{"max_pieces": tc.maxPieces},
			})
			out := runRequest(t, f, rejectedToolsBody(t), app(1))
			if n := p.callCount(); n != tc.calls {
				t.Errorf("%d calls, want %d", n, tc.calls)
			}
			if out.Block || strings.Contains(out.Payload, "second file SECRET") || !strings.Contains(out.Payload, "read the files") {
				t.Errorf("payload = %s, want the refused tool result quarantined and the prompt kept", out.Payload)
			}
			if got := !strings.Contains(out.Payload, "first file"); got != tc.firstQuarantine {
				t.Errorf("first tool result quarantined = %v, want %v (payload %s)", got, tc.firstQuarantine, out.Payload)
			}
		})
	}
}

// A tool result that makes its call fail costs only the tool results in that
// call: the user's prompt is never packed with them (midsommar-clean-4uc.30).
func TestGuardrailScan_ToolCallFailureSparesThePrompt(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &scanFake{fail: func(in guardrails.Input) error {
		for _, s := range in.Segments {
			if strings.Contains(s.Text, "SLOW") {
				return context.DeadlineExceeded
			}
		}
		return nil
	}}
	f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen, "on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages})
	out := runRequest(t, f, chatBody(t,
		chatMsg{Role: "system", Content: "be helpful"},
		chatMsg{Role: "user", Content: "summarise"},
		chatMsg{Role: "tool", Content: strings.Repeat("p", 40_000) + " SLOW", ToolCallID: "call_1"},
	), app(1))
	if out.Block || !strings.Contains(out.Payload, "summarise") || !strings.Contains(out.Payload, "be helpful") || strings.Contains(out.Payload, "SLOW") {
		t.Errorf("block=%v, want the tool result quarantined and the prompts kept", out.Block)
	}
	for _, in := range p.calls {
		var user, tool bool
		for _, s := range in.Segments {
			user = user || s.Role != guardrails.RoleTool
			tool = tool || s.Role == guardrails.RoleTool
		}
		if user && tool {
			t.Error("a call mixed the client's text with a tool result")
		}
	}
}

// Under fail closed, a small request whose tool results' call fails while the
// client's own text is scanned keeps going with the tool results
// quarantined, whatever the filter's action; if the client's call fails too,
// or alone, the request is blocked as before. Fail open passes, as before
// (midsommar-clean-4uc.32).
func TestGuardrailScan_FailClosedToolCallFailureQuarantines(t *testing.T) {
	failWhen := func(marker string) func(guardrails.Input) error {
		return func(in guardrails.Input) error {
			for _, s := range in.Segments {
				if strings.Contains(s.Text, marker) {
					return &guardrails.StatusError{Status: 503}
				}
			}
			return nil
		}
	}
	body := chatBody(t,
		chatMsg{Role: "user", Content: "summarise TOOLONLY"},
		chatMsg{Role: "tool", Content: "page text BADTOOL", ToolCallID: "call_1"},
	)
	cases := []struct {
		name, failMode, action, failOn string
		block, toolKept, userKept      bool
	}{
		{"closed block filter, tool call fails", guardrails.FailClosed, guardrails.ActionBlock, "BADTOOL", false, false, true},
		{"closed log filter, tool call fails", guardrails.FailClosed, guardrails.ActionLog, "BADTOOL", false, false, true},
		{"closed, the client's call fails", guardrails.FailClosed, guardrails.ActionBlock, "TOOLONLY", true, false, false},
		{"closed, both calls fail", guardrails.FailClosed, guardrails.ActionLog, "", true, false, false},
		{"open, tool call fails", guardrails.FailOpen, guardrails.ActionBlock, "BADTOOL", false, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			freshVerdictCache(t, time.Minute)
			fail := func(guardrails.Input) error { return &guardrails.StatusError{Status: 503} }
			if tc.failOn != "" {
				fail = failWhen(tc.failOn)
			}
			p := &scanFake{fail: fail}
			f := scanFilter(t, p, map[string]any{"fail_mode": tc.failMode, "on_detect": tc.action})
			out := runRequest(t, f, body, app(1))
			if out.Block != tc.block {
				t.Fatalf("block = %v, want %v (events %v)", out.Block, tc.block, eventTypes(out))
			}
			if tc.block {
				return
			}
			if got := strings.Contains(out.Payload, "page text BADTOOL"); got != tc.toolKept {
				t.Errorf("tool result kept = %v, want %v (payload %s)", got, tc.toolKept, out.Payload)
			}
			if got := strings.Contains(out.Payload, "summarise TOOLONLY"); got != tc.userKept {
				t.Errorf("prompt kept = %v, want %v", got, tc.userKept)
			}
			if !tc.toolKept {
				e := out.ComplianceEvents[0]
				if e.EventType != "guardrail.error" || e.Metadata["action"] != guardrails.ActionQuarantine || e.Metadata["error_class"] != "http_503" || strings.Contains(e.Description, "BADTOOL") {
					t.Errorf("event = %+v, want guardrail.error with the class and the quarantine", e)
				}
			}
		})
	}
}

// A provider that panics fails its call, in the call's own goroutine; the
// text may have provoked it, so even under fail open it is not let through
// (midsommar-clean-4uc.40).
func TestGuardrailScan_ProviderPanicIsUnscanned(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &scanFake{fail: func(in guardrails.Input) error {
		for _, s := range in.Segments {
			if strings.Contains(s.Text, "BOOM") {
				var m map[string]int
				m["x"] = 1 // a provider bug
			}
		}
		return nil
	}}
	f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen, "on_detect": guardrails.ActionQuarantine})
	out := runRequest(t, f, chatBody(t,
		chatMsg{Role: "user", Content: "summarise"},
		chatMsg{Role: "tool", Content: "crafted BOOM", ToolCallID: "call_1"},
	), app(1))
	if out.Block || strings.Contains(out.Payload, "crafted BOOM") || !strings.Contains(out.Payload, "summarise") {
		t.Errorf("block=%v payload=%s, want the tool result quarantined and the prompt kept", out.Block, out.Payload)
	}
	if r := unscannedReason(out); r != unscannedProviderPanic {
		t.Errorf("unscanned reason = %q, want provider_panic", r)
	}
}

// A provider that makes a request per segment gets each segment in a call
// of its own, so a cold cache's many tool results are scanned in parallel,
// each under its own timeout, not one after another under one
// (midsommar-clean-4uc.44).
func TestGuardrailScan_SegmentPerCallRunsInParallel(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &scanFake{caps: guardrails.Capabilities{SegmentPerCall: true, ToolSegments: true}, delay: 150 * time.Millisecond}
	f := scanFilter(t, p, map[string]any{"on_detect": guardrails.ActionQuarantine, "timeout_ms": 2000})
	msgs := []chatMsg{{Role: "user", Content: "summarise the results"}}
	for i := range 25 {
		msgs = append(msgs, chatMsg{Role: "tool", Content: fmt.Sprintf("result %d", i), ToolCallID: fmt.Sprintf("call_%d", i)})
	}
	start := time.Now()
	out := runRequest(t, f, chatBody(t, msgs...), app(1))
	took := time.Since(start)
	if r := unscannedReason(out); r != "" || out.Block {
		t.Errorf("unscanned=%q block=%v, want every segment scanned", r, out.Block)
	}
	if n := p.callCount(); n != 26 {
		t.Errorf("%d calls, want one per segment", n)
	}
	if took > 2*time.Second {
		t.Errorf("took %v, want the calls run %d at a time", took, scanParallel)
	}
}

// For a provider that makes a request per segment, a small run is one
// parallel wave per trust class: an ordinary request (system and user
// prompt) keeps fail open through an outage, and a sender who adds a fifth
// tool result to provoke a throttle gets it quarantined, not passed
// (midsommar-clean-4uc.50).
func TestGuardrailScan_SegmentPerCallSmallRunIsOneWave(t *testing.T) {
	failOn := func(marker string) func(guardrails.Input) error {
		return func(in guardrails.Input) error {
			for _, s := range in.Segments {
				if marker == "" || strings.Contains(s.Text, marker) {
					return &guardrails.StatusError{Status: 503}
				}
			}
			return nil
		}
	}
	tools := func(n int) []chatMsg {
		msgs := []chatMsg{{Role: "user", Content: "go"}}
		for i := range n {
			content := fmt.Sprintf("result %d", i)
			if i == 0 {
				content = "result DOWN"
			}
			msgs = append(msgs, chatMsg{Role: "tool", Content: content, ToolCallID: fmt.Sprintf("call_%d", i)})
		}
		return msgs
	}
	for _, tc := range []struct {
		name      string
		msgs      []chatMsg
		failOn    string
		unscanned bool
	}{
		{"system and user prompt, vendor down", []chatMsg{{Role: "system", Content: "be brief"}, {Role: "user", Content: "hello"}}, "", false},
		{"four tool results, one fails", tools(4), "DOWN", false},
		{"five tool results, one fails", tools(5), "DOWN", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			freshVerdictCache(t, time.Minute)
			p := &scanFake{caps: guardrails.Capabilities{SegmentPerCall: true, ToolSegments: true}, fail: failOn(tc.failOn)}
			f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen, "on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeSystemAndUser})
			body := chatBody(t, tc.msgs...)
			out := runRequest(t, f, body, app(1))
			if got := unscannedReason(out) != ""; got != tc.unscanned {
				t.Fatalf("unscanned = %v, want %v (events %v)", got, tc.unscanned, eventTypes(out))
			}
			if !tc.unscanned && (out.Block || out.Payload != body) {
				t.Errorf("fail open did not pass a small request: block=%v", out.Block)
			}
			if tc.unscanned && strings.Contains(out.Payload, "result DOWN") {
				t.Error("the failed tool result was passed")
			}
		})
	}
}

// Under fail closed, a small run of a per-segment provider whose tool call
// fails quarantines that tool result only (midsommar-clean-4uc.32 with .50).
func TestGuardrailScan_SegmentPerCallFailClosedQuarantinesTheFailedTool(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &scanFake{caps: guardrails.Capabilities{SegmentPerCall: true, ToolSegments: true}, fail: func(in guardrails.Input) error {
		if strings.Contains(in.Segments[0].Text, "DOWN") {
			return &guardrails.StatusError{Status: 503}
		}
		return nil
	}}
	f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailClosed, "on_detect": guardrails.ActionBlock})
	out := runRequest(t, f, chatBody(t,
		chatMsg{Role: "user", Content: "go"},
		chatMsg{Role: "tool", Content: "result DOWN", ToolCallID: "c0"},
		chatMsg{Role: "tool", Content: "result fine", ToolCallID: "c1"},
	), app(1))
	if out.Block || strings.Contains(out.Payload, "result DOWN") || !strings.Contains(out.Payload, "result fine") {
		t.Errorf("block=%v payload=%s, want only the failed tool result quarantined", out.Block, out.Payload)
	}
}

// A caller's call budget (a native batch upload) is shared by every run
// under its context: remote calls spend it, cached verdicts and local
// providers do not, and spending it is an error for the caller, not a fail
// mode outcome (midsommar-clean-4uc.9).
func TestGuardrailScan_CallBudget(t *testing.T) {
	run := func(f *models.Filter, ctx context.Context, text string) (*ScriptOutput, error) {
		return NewFilterRunner(f).RunScript(&ScriptInput{
			RawInput: chatBody(t, chatMsg{Role: "user", Content: text}), Format: "openai", VendorName: "openai",
			Context: app(1), Ctx: ctx,
		}, nil)
	}

	freshVerdictCache(t, time.Minute)
	p := &scanFake{}
	f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen})
	ctx := guardrails.WithCallBudget(context.Background(), 2)
	for _, text := range []string{"one", "two", "one"} { // the repeat is cached
		if _, err := run(f, ctx, text); err != nil {
			t.Fatalf("%q within the budget: %v", text, err)
		}
	}
	if _, err := run(f, ctx, "three"); !errors.Is(err, guardrails.ErrCallBudget) {
		t.Errorf("third remote call: err %v, want ErrCallBudget whatever the fail mode", err)
	}

	local := &scanFake{caps: guardrails.Capabilities{Local: true}}
	lf := scanFilter(t, local, nil)
	lctx := guardrails.WithCallBudget(context.Background(), 1)
	for _, text := range []string{"a", "b", "c"} {
		if _, err := run(lf, lctx, text); err != nil {
			t.Errorf("a local provider spent the budget: %v", err)
		}
	}
}

// hangingProvider sleeps through its context.
type hangingProvider struct{ d time.Duration }

func (hangingProvider) Capabilities() guardrails.Capabilities { return guardrails.Capabilities{} }

func (h hangingProvider) Classify(context.Context, guardrails.Input) (guardrails.Verdict, error) {
	time.Sleep(h.d)
	return guardrails.Verdict{}, nil
}

// A provider that ignores its context does not hold the request past the
// time budget: the run gives up on it shortly after (midsommar-clean-4uc.41).
func TestGuardrailScan_HangingProviderIsBounded(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	id := scanFilterSeq.Add(1)
	name := fmt.Sprintf("fake_hang_%d", id)
	guardrails.RegisterSpec(guardrails.Spec{Name: name, DisplayName: name, Detectors: []guardrails.DetectorSpec{{Name: "secrets", Category: "secrets"}}})
	guardrails.RegisterFactory(name, func(guardrails.Config) (guardrails.Provider, error) { return hangingProvider{d: 3 * time.Second}, nil })
	f := &models.Filter{Name: name, Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": name, "detectors": []any{map[string]any{"name": "secrets"}}, "on_detect": guardrails.ActionBlock,
		"fail_mode": guardrails.FailClosed, "timeout_ms": 100, "budget": map[string]any{"time_ms": 100},
	}}
	f.ID, f.UpdatedAt = uint(10000+id), time.Now()

	start := time.Now()
	out := runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: "hello"}), app(1))
	if took := time.Since(start); took > time.Second {
		t.Errorf("the run took %v; it waited out a provider that ignores its context", took)
	}
	if !out.Block {
		t.Error("fail closed let the request through when the provider never answered")
	}
}

// Splitting by trust alone does not take the fail mode from an ordinary
// request: a small prompt with a small tool result is still decided by it
// when the provider times out.
func TestGuardrailScan_SmallMixedRunKeepsFailMode(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &scanFake{delay: time.Second}
	f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen, "timeout_ms": 20, "scope": guardrails.ScopeAllMessages})
	body := chatBody(t, chatMsg{Role: "user", Content: "hi"}, chatMsg{Role: "tool", Content: "ok", ToolCallID: "c"})
	out := runRequest(t, f, body, app(1))
	if out.Block || out.Payload != body || unscannedReason(out) != "" {
		t.Errorf("block=%v events=%v, want fail open to pass a small request", out.Block, eventTypes(out))
	}
}

// In a run of several calls a throttled call cannot pass the request under
// fail open: its segment is quarantined, the others are untouched.
func TestGuardrailScan_MultiCallFailureIsUnscanned(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &scanFake{
		caps: guardrails.Capabilities{MaxChars: 40, MaxCallChars: 40},
		fail: func(in guardrails.Input) error {
			for _, s := range in.Segments {
				if strings.Contains(s.Text, "zzz") {
					return &guardrails.StatusError{Status: 429}
				}
			}
			return nil
		},
	}
	f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen, "on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages})
	poisoned := strings.Repeat("a", 60) + "zzz SECRET " + strings.Repeat("b", 60)
	body := chatBody(t,
		chatMsg{Role: "user", Content: "fetch it"},
		chatMsg{Role: "tool", Content: poisoned, ToolCallID: "call_1"},
	)
	out := runRequest(t, f, body, app(1))
	if out.Block {
		t.Fatalf("blocked; want the tool result quarantined and the request through")
	}
	if strings.Contains(out.Payload, "zzz SECRET") || !strings.Contains(out.Payload, "fetch it") {
		t.Errorf("payload = %s, want only the tool result replaced", out.Payload)
	}
	if r := unscannedReason(out); r != unscannedProviderError {
		t.Errorf("unscanned reason = %q, want provider_error", r)
	}
}

// The piece cap is applied before any call: the segments past it are never
// sent and are unscanned.
func TestGuardrailScan_PieceCap(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &scanFake{}
	f := scanFilter(t, p, map[string]any{"on_detect": guardrails.ActionLog, "budget": map[string]any{"max_pieces": 2}})
	body := chatBody(t, chatMsg{Role: "user", Content: "one"}, chatMsg{Role: "user", Content: "two"}, chatMsg{Role: "user", Content: "three"})
	out := runRequest(t, f, body, app(1))
	if got := p.sent(); !slices.Equal(got, []string{"one", "two"}) {
		t.Errorf("provider saw %q, want the first two only", got)
	}
	if r := unscannedReason(out); r != unscannedPieces {
		t.Errorf("unscanned reason = %q, want pieces", r)
	}
	if out.Block {
		t.Error("a log filter blocked")
	}
}

// When the time budget runs out in a run of several calls, what was not
// scanned is unscanned, not passed.
func TestGuardrailScan_TimeBudget(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &scanFake{caps: guardrails.Capabilities{MaxChars: 10, MaxCallChars: 10}, delay: 100 * time.Millisecond}
	f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen, "timeout_ms": 150, "budget": map[string]any{"time_ms": 150}})
	var msgs []chatMsg
	for i := range 10 {
		msgs = append(msgs, chatMsg{Role: "user", Content: fmt.Sprintf("message %d", i)})
	}
	out := runRequest(t, f, chatBody(t, msgs...), app(1))
	if !out.Block {
		t.Fatal("a run out of time passed under fail open")
	}
	if r := unscannedReason(out); r != unscannedTimeBudget {
		t.Errorf("unscanned reason = %q, want time_budget", r)
	}
}

// Text placed across a cut between two pieces is still found.
func TestGuardrailScan_SeamAcrossCut(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &scanFake{caps: guardrails.Capabilities{MaxChars: 40}}
	f := scanFilter(t, p, nil)
	text := strings.Repeat("a", 35) + "SECRET" + strings.Repeat("b", 20) // the cut at 40 splits SECRET
	out := runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: text}), app(1))
	if !out.Block {
		t.Fatal("text across a cut was not found")
	}
	if got := eventTypes(out); !slices.Equal(got, []string{"guardrail.secrets.test_key"}) {
		t.Errorf("events = %v", got)
	}
}

// A finding the provider does not pin on a segment applies to every
// segment of its call, and is not remembered for the clean ones.
func TestGuardrailScan_UnpinnedFinding(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &scanFake{unpinned: true}
	f := scanFilter(t, p, map[string]any{"on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages})
	body := chatBody(t,
		chatMsg{Role: "user", Content: "clean question"},
		chatMsg{Role: "tool", Content: "clean tool output", ToolCallID: "call_1"},
		chatMsg{Role: "tool", Content: "tool says SECRET", ToolCallID: "call_2"},
	)
	out := runRequest(t, f, body, app(1))
	if strings.Contains(out.Payload, "tool says SECRET") || strings.Contains(out.Payload, "clean tool output") || !strings.Contains(out.Payload, "clean question") {
		t.Errorf("payload = %s, want both tool results of the call quarantined and the prompt, in its own call, kept", out.Payload)
	}
	runRequest(t, f, body, app(1))
	if n := p.callCount(); n != 3 {
		t.Errorf("%d calls, want the prompt's clean verdict cached and the unpinned ones not", n)
	}
}

// Benchmarks a 1 MB agent body: a cold run (every segment to the provider)
// and a warm one (the whole history from the cache), with a provider that
// costs nothing, so the numbers are the runner's own overhead.
func BenchmarkGuardrailScan_AgentBody1MB(b *testing.B) {
	var msgs []chatMsg
	for i := 0; len(msgs) < 400; i++ {
		msgs = append(msgs,
			chatMsg{Role: "user", Content: fmt.Sprintf("step %d", i)},
			chatMsg{Role: "tool", Content: strings.Repeat(fmt.Sprintf("line %d of a long tool result\n", i), 170), ToolCallID: fmt.Sprintf("call_%d", i)},
		)
	}
	raw, _ := json.Marshal(map[string]any{"model": "m", "messages": msgs})
	body := string(raw)
	b.Logf("body: %d bytes, %d messages", len(body), len(msgs))

	run := func(b *testing.B, warm bool) {
		verdictCacheMu.Lock()
		prev := sharedVerdictCache
		sharedVerdictCache = newVerdictCache(defaultVerdictCacheEntries, defaultVerdictCacheMaxBytes, time.Hour)
		verdictCacheMu.Unlock()
		defer func() {
			verdictCacheMu.Lock()
			sharedVerdictCache = prev
			verdictCacheMu.Unlock()
		}()

		f := scanFilter(b, &scanFake{}, map[string]any{"on_detect": guardrails.ActionLog, "scope": guardrails.ScopeAllMessages, "budget": map[string]any{"max_pieces": 1000}})
		runner := NewFilterRunner(f)
		in := func() *ScriptInput {
			return &ScriptInput{RawInput: body, Format: "openai", Context: app(1)}
		}
		if warm {
			if _, err := runner.RunScript(in(), nil); err != nil {
				b.Fatal(err)
			}
		}
		b.SetBytes(int64(len(body)))
		b.ResetTimer()
		for range b.N {
			if !warm {
				sharedVerdictCache.lru.Reset()
			}
			if _, err := runner.RunScript(in(), nil); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.Run("cold", func(b *testing.B) { run(b, false) })
	b.Run("warm", func(b *testing.B) { run(b, true) })
}

// The local scan bounds read from the environment are kept within range:
// an allowance of nothing would make every large local scan unscanned, and
// an unbounded one would let one call run as long as its text allows.
func TestBoundedTunable(t *testing.T) {
	for _, c := range []struct {
		value string
		want  int
	}{
		{"", defaultLocalScanMsPerMiB},
		{"junk", defaultLocalScanMsPerMiB},
		{"-5", defaultLocalScanMsPerMiB},
		{"1", 100},
		{"500", 500},
		{"999999", 60000},
	} {
		t.Setenv(EnvLocalScanMsPerMiB, c.value)
		if got := boundedTunable(EnvLocalScanMsPerMiB, defaultLocalScanMsPerMiB, 100, 60000); got != c.want {
			t.Errorf("%q: got %d, want %d", c.value, got, c.want)
		}
	}
}
