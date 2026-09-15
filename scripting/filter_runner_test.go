package scripting

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/tmc/langchaingo/llms"
)

// fakeProvider flags every segment containing "SECRET" and, when asked,
// rewrites it. It records the input it was given.
type fakeProvider struct {
	caps    guardrails.Capabilities
	err     error
	lastIn  guardrails.Input
	calls   int
	verdict func(in guardrails.Input) guardrails.Verdict
}

func (f *fakeProvider) Capabilities() guardrails.Capabilities { return f.caps }

func (f *fakeProvider) Classify(ctx context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	f.calls++
	f.lastIn = in
	if f.err != nil {
		return guardrails.Verdict{}, f.err
	}
	if f.verdict != nil {
		return f.verdict(in), nil
	}
	v := guardrails.Verdict{}
	for _, s := range in.Segments {
		if strings.Contains(s.Text, "SECRET") {
			v.Flagged = true
			v.Findings = append(v.Findings, guardrails.Finding{
				Detector: "secrets.test_key", Category: "secrets", Label: "Test key",
				Score: 1, Severity: "critical", SegmentIndex: s.Index,
			})
			if in.Redact != nil {
				if v.Rewritten == nil {
					v.Rewritten = map[int]string{}
				}
				v.Rewritten[s.Index] = strings.ReplaceAll(s.Text, "SECRET", "[REDACTED]")
			}
		}
	}
	return v, nil
}

// installFake registers a provider spec + factory under a unique name and
// returns the config map that selects it.
func installFake(t *testing.T, name string, p *fakeProvider, onDetect string) models.JSONMap {
	t.Helper()
	guardrails.RegisterSpec(guardrails.Spec{
		Name: name, DisplayName: name, Redacts: true,
		Detectors: []guardrails.DetectorSpec{{Name: "secrets", Category: "secrets"}},
	})
	guardrails.RegisterFactory(name, func(cfg guardrails.Config) (guardrails.Provider, error) { return p, nil })
	return models.JSONMap{
		"provider":  name,
		"detectors": []any{map[string]any{"name": "secrets"}},
		"on_detect": onDetect,
	}
}

func guardrailFilter(name string, response bool, cfg models.JSONMap) *models.Filter {
	return &models.Filter{Name: name, Kind: models.FilterKindGuardrail, ResponseFilter: response, Config: cfg}
}

func userMsg(text string) llms.MessageContent {
	return llms.MessageContent{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextPart(text)}}
}

func systemMsg(text string) llms.MessageContent {
	return llms.MessageContent{Role: llms.ChatMessageTypeSystem, Parts: []llms.ContentPart{llms.TextPart(text)}}
}

func TestNewFilterRunner_DispatchesByKind(t *testing.T) {
	if _, ok := NewFilterRunner(&models.Filter{Kind: models.FilterKindScript}).(*ScriptRunner); !ok {
		t.Error("script kind should run the script runner")
	}
	if _, ok := NewFilterRunner(&models.Filter{}).(*ScriptRunner); !ok {
		t.Error("an empty kind is a script filter (rows created before the kind existed)")
	}
	if _, ok := NewFilterRunner(&models.Filter{Kind: models.FilterKindGuardrail}).(*guardrailRunner); !ok {
		t.Error("guardrail kind should run the guardrail runner")
	}
}

func TestGuardrailRunner_BlocksProxyRequest(t *testing.T) {
	p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true}}
	f := guardrailFilter("block-secrets", false, installFake(t, "fake_block", p, guardrails.ActionBlock))

	input := &ScriptInput{
		RawInput: `{"messages":[{"role":"system","content":"be nice"},{"role":"user","content":"my key is SECRET"}]}`,
		Messages: []llms.MessageContent{systemMsg("be nice"), userMsg("my key is SECRET")},
	}
	out, err := NewFilterRunner(f).RunScript(input, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if !out.Block || out.Message != guardrails.DefaultRequestBlockMessage {
		t.Errorf("RunScript() block=%v message=%q, want a block with the default message", out.Block, out.Message)
	}
	if len(out.ComplianceEvents) != 1 || out.ComplianceEvents[0].EventType != "guardrail.secrets.test_key" || out.ComplianceEvents[0].Severity != "critical" {
		t.Errorf("RunScript() compliance events = %+v", out.ComplianceEvents)
	}
	if strings.Contains(out.ComplianceEvents[0].Description, "SECRET") {
		t.Error("compliance event must not carry the matched text")
	}
	// Default scope is all_user: the system message is not sent.
	if len(p.lastIn.Segments) != 1 || p.lastIn.Segments[0].Role != "user" {
		t.Errorf("provider saw segments %+v, want only the user message", p.lastIn.Segments)
	}
}

func TestGuardrailRunner_RedactsProxyRequestAsMessages(t *testing.T) {
	p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true}}
	f := guardrailFilter("redact-secrets", false, installFake(t, "fake_redact", p, guardrails.ActionRedact))

	input := &ScriptInput{
		RawInput: `{"messages":[{"role":"system","content":"be nice"},{"role":"user","content":"my key is SECRET"}]}`,
		Messages: []llms.MessageContent{systemMsg("be nice"), userMsg("my key is SECRET")},
	}
	out, err := NewFilterRunner(f).RunScript(input, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if out.Block {
		t.Fatal("redact must not block")
	}
	if p.lastIn.Redact == nil {
		t.Error("provider was not asked to redact")
	}
	if len(out.Messages) != 2 || out.Messages[0]["content"] != "be nice" || out.Messages[1]["content"] != "my key is [REDACTED]" {
		t.Errorf("RunScript() Messages = %v, want the full conversation with the user message rewritten", out.Messages)
	}
	if out.Payload != "" {
		t.Errorf("RunScript() Payload = %q, want empty when Messages carry the rewrite", out.Payload)
	}
	if len(out.ComplianceEvents) != 1 || out.ComplianceEvents[0].Metadata["action"] != guardrails.ActionRedact {
		t.Errorf("RunScript() compliance events = %+v", out.ComplianceEvents)
	}
}

func TestGuardrailRunner_RedactsChatMessageAsPayload(t *testing.T) {
	p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true}}
	f := guardrailFilter("redact-chat", false, installFake(t, "fake_redact_chat", p, guardrails.ActionRedact))

	// Chat and tool paths hand over one message whose content is the raw input.
	input := &ScriptInput{RawInput: "my key is SECRET", Messages: []llms.MessageContent{userMsg("my key is SECRET")}, IsChat: true}
	out, err := NewFilterRunner(f).RunScript(input, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if out.Payload != "my key is [REDACTED]" || len(out.Messages) != 0 {
		t.Errorf("RunScript() Payload=%q Messages=%v, want the payload rewritten", out.Payload, out.Messages)
	}
}

func TestGuardrailRunner_RedactOnResponseDegradesToLog(t *testing.T) {
	p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true}}
	f := guardrailFilter("redact-response", true, installFake(t, "fake_redact_resp", p, guardrails.ActionRedact))

	input := &ScriptInput{RawInput: "here is SECRET", IsResponse: true}
	out, err := NewFilterRunner(f).RunScript(input, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if out.Block || out.Payload != "here is SECRET" {
		t.Errorf("RunScript() block=%v payload=%q, want the response untouched", out.Block, out.Payload)
	}
	if p.lastIn.Redact != nil {
		t.Error("provider must not be asked to rewrite a response")
	}
	if len(out.ComplianceEvents) != 1 || out.ComplianceEvents[0].Metadata["action"] != guardrails.ActionLog {
		t.Errorf("RunScript() compliance events = %+v, want the finding logged with action log", out.ComplianceEvents)
	}
}

func TestGuardrailRunner_LogOnlyPassesThrough(t *testing.T) {
	p := &fakeProvider{caps: guardrails.Capabilities{}}
	f := guardrailFilter("log-secrets", false, installFake(t, "fake_log", p, guardrails.ActionLog))

	input := &ScriptInput{RawInput: "SECRET", Messages: []llms.MessageContent{userMsg("SECRET")}}
	out, err := NewFilterRunner(f).RunScript(input, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if out.Block || out.Payload != "SECRET" || len(out.ComplianceEvents) != 1 {
		t.Errorf("RunScript() = %+v, want pass-through with one event", out)
	}
}

func TestGuardrailRunner_CleanInputIsSilent(t *testing.T) {
	p := &fakeProvider{}
	f := guardrailFilter("block-secrets", false, installFake(t, "fake_clean", p, guardrails.ActionBlock))

	out, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: "hello", Messages: []llms.MessageContent{userMsg("hello")}}, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if out.Block || len(out.ComplianceEvents) != 0 || out.Payload != "hello" {
		t.Errorf("RunScript() = %+v, want an untouched pass-through", out)
	}
}

func TestGuardrailRunner_FailModes(t *testing.T) {
	t.Run("request side defaults to closed", func(t *testing.T) {
		p := &fakeProvider{err: errors.New("boom")}
		f := guardrailFilter("closed", false, installFake(t, "fake_fail_closed", p, guardrails.ActionBlock))
		out, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: "x", Messages: []llms.MessageContent{userMsg("x")}}, nil)
		if err != nil {
			t.Fatalf("RunScript() error = %v, fail modes must not surface as errors", err)
		}
		if !out.Block || out.Message != guardrails.DefaultRequestBlockMessage {
			t.Errorf("RunScript() block=%v message=%q, want a block", out.Block, out.Message)
		}
		if strings.Contains(out.Message, "boom") {
			t.Error("provider error must not reach the client")
		}
		if len(out.ComplianceEvents) != 1 || out.ComplianceEvents[0].EventType != "guardrail.error" || out.ComplianceEvents[0].Severity != "critical" {
			t.Errorf("RunScript() compliance events = %+v", out.ComplianceEvents)
		}
	})

	t.Run("response side defaults to open", func(t *testing.T) {
		p := &fakeProvider{err: errors.New("boom")}
		f := guardrailFilter("open", true, installFake(t, "fake_fail_open", p, guardrails.ActionBlock))
		out, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: "x", IsResponse: true}, nil)
		if err != nil {
			t.Fatalf("RunScript() error = %v", err)
		}
		if out.Block || out.Payload != "x" {
			t.Errorf("RunScript() = %+v, want pass-through", out)
		}
		if len(out.ComplianceEvents) != 1 || out.ComplianceEvents[0].Severity != "warning" {
			t.Errorf("RunScript() compliance events = %+v", out.ComplianceEvents)
		}
	})

	t.Run("explicit fail_mode wins", func(t *testing.T) {
		p := &fakeProvider{err: errors.New("boom")}
		cfg := installFake(t, "fake_fail_explicit", p, guardrails.ActionBlock)
		cfg["fail_mode"] = guardrails.FailOpen
		f := guardrailFilter("open-request", false, cfg)
		out, _ := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: "x", Messages: []llms.MessageContent{userMsg("x")}}, nil)
		if out.Block {
			t.Error("fail_mode open must let the request through")
		}
	})
}

func TestGuardrailRunner_StreamingCadence(t *testing.T) {
	p := &fakeProvider{caps: guardrails.Capabilities{}}
	cfg := installFake(t, "fake_stream", p, guardrails.ActionBlock)
	cfg["stream"] = map[string]any{"evaluate_every_chars": 10}
	f := guardrailFilter("stream", true, cfg)

	run := func(buffer, delta string) *ScriptOutput {
		out, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: delta, CurrentBuffer: buffer, IsResponse: true, IsChunk: true, ChunkIndex: 1}, nil)
		if err != nil {
			t.Fatalf("RunScript() error = %v", err)
		}
		return out
	}

	run("abcde", "abcde") // 0 -> 5: no boundary crossed
	if p.calls != 0 {
		t.Fatalf("provider called %d times before the first boundary", p.calls)
	}
	run("abcdefghijkl", "fghijkl") // 5 -> 12 crosses 10
	if p.calls != 1 {
		t.Fatalf("provider called %d times, want 1 after crossing the boundary", p.calls)
	}
	run("abcdefghijklmno", "mno") // 12 -> 15: same window
	if p.calls != 1 {
		t.Fatalf("provider called %d times, want no call inside the window", p.calls)
	}
	out := run("abcdefghijklmno SECRET", " SECRET") // 15 -> 22 crosses 20
	if p.calls != 2 || !out.Block {
		t.Fatalf("calls=%d block=%v, want the buffer evaluated and blocked at the next boundary", p.calls, out.Block)
	}
	if p.lastIn.Segments[0].Text != "abcdefghijklmno SECRET" {
		t.Errorf("provider saw %q, want the accumulated buffer", p.lastIn.Segments[0].Text)
	}

	// End of stream: a response shorter than the cadence is still evaluated.
	p.calls = 0
	out, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: "", CurrentBuffer: "tiny SECRET", IsResponse: true, IsChunk: true, IsFinal: true, ChunkIndex: 3}, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if p.calls != 1 || !out.Block {
		t.Errorf("calls=%d block=%v, want the final evaluation to run and block", p.calls, out.Block)
	}
	out, _ = NewFilterRunner(f).RunScript(&ScriptInput{RawInput: "", CurrentBuffer: "   ", IsResponse: true, IsChunk: true, IsFinal: true}, nil)
	if p.calls != 1 || out.Block {
		t.Error("an empty response must not be sent to the provider at the end of the stream")
	}
}

func TestGuardrailRunner_SplitsLongSegmentsAndMergesRewrites(t *testing.T) {
	p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true, MaxChars: 8}}
	f := guardrailFilter("split", false, installFake(t, "fake_split", p, guardrails.ActionRedact))

	text := "aaaaaaaaSECRETbbbbbbbb" // 22 chars -> pieces of 8: "aaaaaaaa", "SECRETbb", "bbbbbb"
	out, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: text, Messages: []llms.MessageContent{userMsg(text)}}, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if len(p.lastIn.Segments) != 3 {
		t.Fatalf("provider saw %d pieces, want 3", len(p.lastIn.Segments))
	}
	if out.Payload != "aaaaaaaa[REDACTED]bbbbbbbb" {
		t.Errorf("RunScript() Payload = %q, want the pieces reassembled with the rewrite", out.Payload)
	}
}

func TestGuardrailRunner_ScopeSelectsMessages(t *testing.T) {
	msgs := []llms.MessageContent{systemMsg("sys"), userMsg("u1"), {Role: llms.ChatMessageTypeAI, Parts: []llms.ContentPart{llms.TextPart("a1")}}, userMsg("u2")}
	cases := map[string][]string{
		guardrails.ScopeLastUser:      {"u2"},
		guardrails.ScopeAllUser:       {"u1", "u2"},
		guardrails.ScopeSystemAndUser: {"sys", "u1", "u2"},
		guardrails.ScopeAllMessages:   {"sys", "u1", "a1", "u2"},
	}
	for scope, want := range cases {
		p := &fakeProvider{}
		cfg := installFake(t, "fake_scope_"+scope, p, guardrails.ActionLog)
		cfg["scope"] = scope
		f := guardrailFilter("scope", false, cfg)
		if _, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: "{}", Messages: msgs}, nil); err != nil {
			t.Fatalf("%s: RunScript() error = %v", scope, err)
		}
		got := make([]string, 0, len(p.lastIn.Segments))
		for _, s := range p.lastIn.Segments {
			got = append(got, s.Text)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: provider saw %v, want %v", scope, got, want)
		}
	}
}

// Connection references are resolved where the filter runs, so a provider
// never sees "$ENV/NAME" as its credential.
func TestGuardrailRunner_ResolvesConnectionReferences(t *testing.T) {
	t.Setenv("GUARDRAIL_TEST_KEY", "resolved-key")
	var seen guardrails.Config
	guardrails.RegisterSpec(guardrails.Spec{
		Name: "fake_conn", DisplayName: "fake_conn",
		Detectors:        []guardrails.DetectorSpec{{Name: "secrets"}},
		ConnectionFields: []guardrails.ConnectionField{{Name: "api_key", Secret: true}},
	})
	guardrails.RegisterFactory("fake_conn", func(cfg guardrails.Config) (guardrails.Provider, error) {
		seen = cfg
		return &fakeProvider{}, nil
	})
	f := guardrailFilter("conn", false, models.JSONMap{
		"provider":   "fake_conn",
		"detectors":  []any{map[string]any{"name": "secrets"}},
		"on_detect":  guardrails.ActionLog,
		"connection": map[string]any{"api_key": "$ENV/GUARDRAIL_TEST_KEY", "endpoint": "http://x"},
	})
	if _, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: "x", Messages: []llms.MessageContent{userMsg("x")}}, nil); err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if seen.Connection["api_key"] != "resolved-key" || seen.Connection["endpoint"] != "http://x" {
		t.Errorf("provider saw connection %v, want the $ENV reference resolved and the endpoint untouched", seen.Connection)
	}
}

func TestGuardrailRunner_MisconfiguredFilterErrors(t *testing.T) {
	f := guardrailFilter("bad", false, models.JSONMap{"provider": "no_such_provider", "detectors": []any{map[string]any{"name": "x"}}, "on_detect": "block"})
	if _, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: "x"}, nil); err == nil {
		t.Fatal("RunScript() returned no error for an unknown provider")
	}
}
