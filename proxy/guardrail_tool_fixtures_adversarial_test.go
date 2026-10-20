//go:build enterprise

package proxy

// Adversarial tests for 4uc.2 (tests agent): the shipped built-in guardrails
// run through runRequestFilters over every vendor fixture in
// pkg/testinfra/toolfixtures. A redaction changes only the PII leaves and
// keeps every tool block, id, image, signature and number; a log filter
// never changes a byte; a block filter catches the tool-result canary under
// all_messages in every format; the next filter in the chain sees the
// rewritten body; a redaction that cannot land in place never lets the PII
// through silently.

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/toolfixtures"
)

// newAllFormatsFilterProxy has every extractor and reconstructor New
// registers, so a fixture runs exactly as on the gateway.
func newAllFormatsFilterProxy() *Proxy {
	p := &Proxy{}
	ex := NewMessageExtractorRegistry()
	ex.Register(&OpenAIMessageExtractor{})
	ex.Register(&AnthropicMessageExtractor{})
	ex.Register(&GoogleAIMessageExtractor{})
	ex.Register(&VertexMessageExtractor{})
	ex.Register(&OllamaMessageExtractor{})
	ex.Register(&OpenAIDecisionsMessageExtractor{})
	ex.Register(&SystemOneMessageExtractor{})
	ex.Register(&OpenAIResponsesMessageExtractor{})
	ex.Register(&OpenAIInputMessageExtractor{})
	p.messageExtractorRegistry = ex
	rc := NewMessageReconstructorRegistry()
	rc.Register(&OpenAIMessageReconstructor{})
	rc.Register(&AnthropicMessageReconstructor{})
	rc.Register(&GoogleAIMessageReconstructor{})
	rc.Register(&VertexMessageReconstructor{})
	rc.Register(&OllamaMessageReconstructor{})
	rc.Register(&OpenAIDecisionsMessageReconstructor{})
	rc.Register(&SystemOneMessageReconstructor{})
	rc.Register(&OpenAIResponsesMessageReconstructor{})
	rc.Register(&OpenAIInputMessageReconstructor{})
	p.messageReconstructorRegistry = rc
	return p
}

// complianceSink collects compliance events for the duration of a test.
type complianceSink struct {
	mu     sync.Mutex
	events []*models.ComplianceEvent
}

func (h *complianceSink) RecordProxyLog(context.Context, *models.ProxyLog)                {}
func (h *complianceSink) RecordChatLogEntry(context.Context, *models.LLMChatLogEntry)     {}
func (h *complianceSink) RecordChatRecord(context.Context, *models.LLMChatRecord)         {}
func (h *complianceSink) RecordToolCall(context.Context, string, time.Time, int, uint)    {}
func (h *complianceSink) RecordChatRecordsBatch(context.Context, []*models.LLMChatRecord) {}
func (h *complianceSink) RecordProxyLogsBatch(context.Context, []*models.ProxyLog)        {}
func (h *complianceSink) SetAsGlobalHandler()                                             { analytics.SetHandler(h) }
func (h *complianceSink) RecordComplianceEvents(_ context.Context, e []*models.ComplianceEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, e...)
}
func (*complianceSink) RecordMCPCalls(context.Context, []models.MCPCallRecord) {}

func (h *complianceSink) take() []*models.ComplianceEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := h.events
	h.events = nil
	return out
}

func installComplianceSink(t *testing.T) *complianceSink {
	t.Helper()
	old := analytics.GetHandler()
	h := &complianceSink{}
	analytics.SetHandler(h)
	t.Cleanup(func() { analytics.SetHandler(old) })
	return h
}

func builtinGuardrail(name, category, action, scope string) *models.Filter {
	return &models.Filter{Name: name, Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider":  "builtin",
		"detectors": []any{map[string]any{"name": category}},
		"on_detect": action,
		"scope":     scope,
	}}
}

// proxyFixtures are the fixtures a single request body carries, with the
// format the proxy hands the filters: the vendor on /llm/, or the native
// API's wire format.
func proxyFixtures() []toolfixtures.Fixture {
	return toolfixtures.ByFormat("anthropic", "openai", "ollama", "google_ai", "vertex", "openai_responses", "bedrock")
}

func runFixture(t *testing.T, p *Proxy, f toolfixtures.Fixture, filters ...*models.Filter) ([]byte, error) {
	t.Helper()
	llm := &models.LLM{ID: 7, Vendor: models.Vendor(f.Format), Filters: filters}
	r := httptest.NewRequest("POST", "/llm/rest/test/v1/x", strings.NewReader(string(f.Body)))
	return p.runRequestFilters(llm, r, f.Body, f.Format, "test-model")
}

func TestAdversarial_RedactPIIKeepsEveryToolBlock(t *testing.T) {
	p := newAllFormatsFilterProxy()
	installComplianceSink(t)
	for _, f := range proxyFixtures() {
		pii := f.PIISegments()
		if len(pii) == 0 {
			continue
		}
		t.Run(f.Name, func(t *testing.T) {
			out, err := runFixture(t, p, f, builtinGuardrail("pii", guardrails.CategoryPII, guardrails.ActionRedact, guardrails.ScopeAllMessages))
			if err != nil {
				t.Fatalf("runRequestFilters: %v", err)
			}
			if strings.Contains(string(out), toolfixtures.PIIEmail) {
				t.Error("the e-mail address reached upstream")
			}
			toolfixtures.AssertOnlyChanged(t, f.Body, out, toolfixtures.Paths(pii)...)
			toolfixtures.AssertPreserved(t, f, out)
		})
	}
}

// A guardrail that finds nothing, or only logs, forwards the client's bytes
// unchanged: no re-marshalling, no key reordering, no lost precision.
func TestAdversarial_NonRewritingGuardrailsKeepBytes(t *testing.T) {
	p := newAllFormatsFilterProxy()
	sink := installComplianceSink(t)
	for _, f := range proxyFixtures() {
		t.Run(f.Name, func(t *testing.T) {
			out, err := runFixture(t, p, f,
				builtinGuardrail("log-injection", guardrails.CategoryInjection, guardrails.ActionLog, guardrails.ScopeAllMessages),
				builtinGuardrail("redact-secrets", guardrails.CategorySecrets, guardrails.ActionRedact, guardrails.ScopeAllMessages),
			)
			if err != nil {
				t.Fatalf("runRequestFilters: %v", err)
			}
			if string(out) != string(f.Body) {
				diffs, _ := toolfixtures.DiffPaths(f.Body, out)
				t.Errorf("body changed although nothing was redacted (decoded diffs %v)", diffs)
			}
			if len(sink.take()) == 0 {
				t.Error("the log filter recorded no event for the tool-result canary under all_messages")
			}
		})
	}
}

// A block filter sees tool output in every format. With an explicit
// tool_results.on_detect block it refuses the request; without one the tool
// result is quarantined and the request continues (4uc.5).
func TestAdversarial_BlockOnToolCanaryAllMessages(t *testing.T) {
	p := newAllFormatsFilterProxy()
	installComplianceSink(t)
	for _, f := range proxyFixtures() {
		t.Run(f.Name, func(t *testing.T) {
			// Explicit tool_results.on_detect block: the request is refused.
			explicit := builtinGuardrail("block-injection", guardrails.CategoryInjection, guardrails.ActionBlock, guardrails.ScopeAllMessages)
			explicit.Config["tool_results"] = map[string]any{"on_detect": guardrails.ActionBlock}
			_, err := runFixture(t, p, f, explicit)
			if err == nil || !strings.Contains(err.Error(), "Policy error") {
				t.Errorf("explicit block: err = %v, want a policy block for the tool-result canary", err)
			}
			// No tool_results key (4uc.5): block maps to quarantine for tool
			// results, so the agent loop goes on without the canary.
			out, err := runFixture(t, p, f, builtinGuardrail("block-injection", guardrails.CategoryInjection, guardrails.ActionBlock, guardrails.ScopeAllMessages))
			if err != nil {
				t.Fatalf("default block filter refused a request whose only finding is in a tool result: %v", err)
			}
			if strings.Contains(string(out), toolfixtures.CanaryPrefix) {
				t.Error("the canary reached upstream")
			}
		})
	}
}

// The second filter reads the first one's output: after a redaction a
// script filter must not see the e-mail address, and a later guardrail
// still sees (and logs) the untouched canary.
func TestAdversarial_ChainSeesRewrittenBody(t *testing.T) {
	p := newAllFormatsFilterProxy()
	sink := installComplianceSink(t)
	tripwire := &models.Filter{Name: "tripwire", Script: []byte(`
text := import("text")
output := {block: text.contains(input.raw_input, "` + toolfixtures.PIIEmail + `"), message: "email leaked to the next filter"}
`)}
	for _, f := range proxyFixtures() {
		if len(f.PIISegments()) == 0 {
			continue
		}
		t.Run(f.Name, func(t *testing.T) {
			sink.take()
			out, err := runFixture(t, p, f,
				builtinGuardrail("pii", guardrails.CategoryPII, guardrails.ActionRedact, guardrails.ScopeAllMessages),
				tripwire,
				builtinGuardrail("log-injection", guardrails.CategoryInjection, guardrails.ActionLog, guardrails.ScopeAllMessages),
			)
			if err != nil {
				t.Fatalf("chain: %v", err)
			}
			var sawInjection bool
			for _, e := range sink.take() {
				if e.FilterName == "log-injection" && strings.HasPrefix(e.EventType, "guardrail.injection.") {
					sawInjection = true
				}
			}
			if !sawInjection {
				t.Error("the guardrail after the redaction did not see the canary")
			}
			toolfixtures.AssertPreserved(t, f, out)
		})
	}
}

// A redaction the runner cannot write back in place is never forwarded,
// whatever the fail mode (4uc.22): in a tool result (the e-mail is a Gemini
// functionResponse KEY, detection text but not a leaf) the result is
// quarantined and the request continues; in a user message (the address
// split across two text parts, found only by the joined view) the request
// is blocked. Both record guardrail.unredactable.
func TestAdversarial_UnwritableRedactionNeverSilent(t *testing.T) {
	p := newAllFormatsFilterProxy()
	sink := installComplianceSink(t)
	toolKey := toolfixtures.Fixture{Name: "email-key", Format: "google_ai", Body: []byte(`{"contents":[{"role":"user","parts":[{"text":"look up the plan"}]},{"role":"model","parts":[{"functionCall":{"name":"crm","args":{}}}]},{"role":"user","parts":[{"functionResponse":{"name":"crm","response":{"jane.doe@example.com":{"plan":"business"}}}}]}]}`)}
	userSplit := toolfixtures.Fixture{Name: "email-split", Format: "openai", Body: []byte(`{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"mail jane.doe@"},{"type":"text","text":"example.com today"}]}]}`)}
	for _, mode := range []string{guardrails.FailClosed, guardrails.FailOpen} {
		t.Run(mode+"/tool result", func(t *testing.T) {
			sink.take()
			filter := builtinGuardrail("pii", guardrails.CategoryPII, guardrails.ActionRedact, guardrails.ScopeAllMessages)
			filter.Config["fail_mode"] = mode
			out, err := runFixture(t, p, toolKey, filter)
			if err != nil {
				t.Fatalf("an unredactable tool result must be quarantined, not block the request: %v", err)
			}
			if strings.Contains(string(out), toolfixtures.PIIEmail) {
				t.Error("the address went upstream")
			}
			if !strings.Contains(string(out), "look up the plan") || !strings.Contains(string(out), `"name":"crm"`) {
				t.Errorf("the prompt or the functionResponse name was lost: %s", out)
			}
			if !hasEvent(sink.take(), "guardrail.unredactable") {
				t.Error("no guardrail.unredactable event")
			}
		})
		t.Run(mode+"/user message", func(t *testing.T) {
			sink.take()
			filter := builtinGuardrail("pii", guardrails.CategoryPII, guardrails.ActionRedact, guardrails.ScopeAllMessages)
			filter.Config["fail_mode"] = mode
			out, err := runFixture(t, p, userSplit, filter)
			events := sink.take()
			if err == nil {
				if strings.Contains(string(out), "jane.doe@") && strings.Contains(string(out), "example.com") {
					t.Errorf("both halves of the address went upstream; events %d", len(events))
				}
				return
			}
			if !hasEvent(events, "guardrail.unredactable") {
				t.Error("blocked without a guardrail.unredactable event")
			}
		})
	}
}

func hasEvent(events []*models.ComplianceEvent, eventType string) bool {
	for _, e := range events {
		if e.EventType == eventType {
			return true
		}
	}
	return false
}

// Runners built once serve many bodies from many goroutines (the batch
// path); the redactions never cross between bodies.
func TestAdversarial_SharedRunnersAcrossConcurrentBodies(t *testing.T) {
	p := newAllFormatsFilterProxy()
	installComplianceSink(t)
	fs := proxyFixtures()
	filters := []*models.Filter{
		builtinGuardrail("pii", guardrails.CategoryPII, guardrails.ActionRedact, guardrails.ScopeAllMessages),
		builtinGuardrail("log-injection", guardrails.CategoryInjection, guardrails.ActionLog, guardrails.ScopeAllMessages),
	}
	byFormat := map[string][]requestFilterRunner{}
	for _, f := range fs {
		llm := &models.LLM{ID: 7, Vendor: models.Vendor(f.Format), Filters: filters}
		byFormat[f.Format] = requestFilterRunners(llm)
	}
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				f := fs[(w+i)%len(fs)]
				llm := &models.LLM{ID: 7, Vendor: models.Vendor(f.Format), Filters: filters}
				r := httptest.NewRequest("POST", "/", strings.NewReader(string(f.Body)))
				out, err := p.runRequestFiltersWith(byFormat[f.Format], llm, r, f.Body, f.Format, "m", nil)
				if err != nil {
					t.Errorf("%s: %v", f.Name, err)
					return
				}
				if diffs, err := toolfixtures.DiffPaths(f.Body, out); err != nil {
					t.Errorf("%s: %v", f.Name, err)
				} else {
					allowed := map[string]bool{}
					for _, s := range f.PIISegments() {
						allowed[s.Path] = true
					}
					for _, d := range diffs {
						if !allowed[d] {
							t.Errorf("%s: changed at %s under concurrency", f.Name, d)
						}
					}
				}
			}
		}(w)
	}
	wg.Wait()
}
