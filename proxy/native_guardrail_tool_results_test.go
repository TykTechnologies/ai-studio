package proxy

import (
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/toolfixtures"
)

// Tool results in native API items (Anthropic Message Batches, OpenAI batch
// files) and in Responses API bodies are screened like any request's, with a
// remote provider whose calls one native create may make only so many of.

const remoteGuardProvider = "test_remote_guard"

var (
	registerRemoteGuard sync.Once
	remoteGuardCalls    atomic.Int64
)

// remoteGuard is a remote (not Local) provider that flags the tool fixture
// canaries and "SECRET", and counts its calls.
type remoteGuard struct{}

func (remoteGuard) Capabilities() guardrails.Capabilities {
	return guardrails.Capabilities{ToolSegments: true}
}

func (remoteGuard) Classify(_ context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	remoteGuardCalls.Add(1)
	v := guardrails.Verdict{}
	for _, s := range in.Segments {
		if strings.Contains(s.Text, toolfixtures.CanaryPrefix) || strings.Contains(s.Text, "SECRET") {
			v.Findings = append(v.Findings, guardrails.Finding{Detector: "injection.test", Category: "injection", Score: 1, Severity: "warning", SegmentIndex: s.Index})
		}
	}
	v.Flagged = len(v.Findings) > 0
	return v, nil
}

// remoteGuardFilter blocks, which quarantines a flagged tool result.
func remoteGuardFilter() *models.Filter {
	registerRemoteGuard.Do(func() {
		guardrails.RegisterSpec(guardrails.Spec{
			Name: remoteGuardProvider, DisplayName: "test remote guard",
			Detectors: []guardrails.DetectorSpec{{Name: "injection", Category: "injection"}},
		})
		guardrails.RegisterFactory(remoteGuardProvider, func(guardrails.Config) (guardrails.Provider, error) { return remoteGuard{}, nil })
	})
	return &models.Filter{
		Name: "remote guard", Kind: models.FilterKindGuardrail,
		Config: models.JSONMap{
			"provider":   remoteGuardProvider,
			"detectors":  []any{map[string]any{"name": "injection"}},
			"on_detect":  guardrails.ActionBlock,
			"quarantine": map[string]any{"placeholder": "[removed by policy]"},
		},
	}
}

func withGuard(allowed ...string) func(*models.LLM) {
	return func(l *models.LLM) {
		l.AllowedModels = allowed
		l.Filters = append(l.Filters, remoteGuardFilter())
	}
}

// A Message Batches item whose tool_result carries an injection reaches the
// vendor with that tool result quarantined; the other item, the custom_ids
// and the tool_use_id are untouched.
func TestNativeAnthropicBatch_ToolResultQuarantined(t *testing.T) {
	t.Parallel()
	fx := toolfixtures.Get("anthropic_batch_tool_result")
	h := newNativeHarness(t, nativeHarnessOpts{mutateLLM: withGuard("^claude-sonnet-4-5$")})
	resp, out := h.do(h.keyA, http.MethodPost, batchesPath, string(fx.Body), "anthropic-version", "2023-06-01")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

	calls := h.vendor.callsTo(http.MethodPost, "/v1/messages/batches")
	require.Len(t, calls, 1)
	sent := calls[0].Body
	assert.NotContains(t, string(sent), toolfixtures.CanaryPrefix)
	assert.Contains(t, string(sent), "[removed by policy]")
	toolfixtures.AssertOnlyChanged(t, fx.Body, sent, "/requests/1/params/messages/2/content/0/content")
	toolfixtures.AssertPreserved(t, fx, sent)
}

// One budget of remote guardrail calls covers all of a create's items:
// spending it refuses the create with a 400 that names the limit, before
// anything reaches the vendor.
func TestNativeCreate_GuardrailCallBudget(t *testing.T) {
	t.Parallel()
	body := batchCreateBody("claude-luna", "claude-luna", "claude-luna")
	for name, tc := range map[string]struct {
		limit  int
		status int
	}{
		"within the budget": {limit: 3, status: http.StatusOK},
		"over the budget":   {limit: 2, status: http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			h := newNativeHarness(t, nativeHarnessOpts{
				config:    &Config{NativeMaxGuardrailCalls: tc.limit},
				mutateLLM: withGuard("^claude-luna$"),
			})
			resp, out := h.do(h.keyA, http.MethodPost, batchesPath, body, "anthropic-version", "2023-06-01")
			require.Equal(t, tc.status, resp.StatusCode, string(out))
			if tc.status != http.StatusBadRequest {
				return
			}
			assert.Contains(t, string(out), "NATIVE_API_MAX_GUARDRAIL_CALLS")
			assert.Contains(t, string(out), "2 guardrail provider calls")
			assert.NotContains(t, string(out), "hello", "the error must not echo item content")
			assert.Empty(t, h.vendor.callsTo(http.MethodPost, "/v1/messages/batches"))
		})
	}
}

// An OpenAI batch file whose Responses API line carries an injection in a
// function_call_output is uploaded with only that line's tool output
// quarantined; the other line is byte for byte what was sent.
func TestOpenAIBatchUpload_QuarantinesOnlyTheToolOutput(t *testing.T) {
	t.Parallel()
	h := newResponsesHarness(t)
	h.primary.AllowedModels = []string{"^gpt-4o"}
	h.primary.Filters = []*models.Filter{remoteGuardFilter()}
	require.NoError(t, h.db.Save(h.primary).Error)
	require.NoError(t, h.proxy.loadResources())

	clean := chatLine("clean", "gpt-4o", "hi")
	poisoned := `{"custom_id":"poisoned","method":"POST","url":"/v1/responses","body":{"model":"gpt-4o","input":[` +
		`{"role":"user","content":"read it"},` +
		`{"type":"function_call","call_id":"call_9","name":"read_file","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"call_9","output":"file says SECRET"}]}}`
	resp, out := h.uploadFile(h.keyC, clean, poisoned)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)

	calls := h.api.callsTo(http.MethodPost, "/v1/files")
	require.Len(t, calls, 1)
	lines := uploadedLines(t, calls[0].Body, calls[0].Headers.Get("Content-Type"))
	require.Len(t, lines, 2)
	assert.Equal(t, clean, lines[0], "the clean line is forwarded as sent")
	toolfixtures.AssertOnlyChanged(t, []byte(poisoned), []byte(lines[1]), "/body/input/2/output")
	var line map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &line), "the line stays valid JSON")
	assert.Equal(t, "poisoned", line["custom_id"])
	assert.NotContains(t, lines[1], "SECRET")
}

// uploadedLines reads the JSONL file part of a multipart upload.
func uploadedLines(t *testing.T, body []byte, contentType string) []string {
	t.Helper()
	_, params, ok := strings.Cut(contentType, "boundary=")
	require.True(t, ok, contentType)
	r := multipart.NewReader(strings.NewReader(string(body)), strings.Trim(params, `"`))
	for {
		part, err := r.NextPart()
		require.NoError(t, err)
		if part.FormName() != "file" {
			continue
		}
		b, err := io.ReadAll(part)
		require.NoError(t, err)
		return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	}
}

// Responses API tool outputs (a function_call_output string or part array,
// custom tool and MCP outputs) are quarantined in place: item ids, call ids
// and encrypted reasoning pass through untouched.
func TestResponsesToolOutputs_QuarantinedInPlace(t *testing.T) {
	p := New(nil, nil, &Config{})
	for _, name := range []string{"openai_responses_function_call_output", "openai_responses_mixed_outputs"} {
		t.Run(name, func(t *testing.T) {
			fx := toolfixtures.Get(name)
			r, _ := http.NewRequest(http.MethodPost, "/llm/rest/primary/v1/responses", strings.NewReader(string(fx.Body)))
			out, err := p.runRequestFilters(&models.LLM{Vendor: models.OPENAI, Filters: []*models.Filter{remoteGuardFilter()}}, r, fx.Body, requestFilterFormatOpenAIResponses, "gpt-4.1")
			require.NoError(t, err)
			assert.NotContains(t, string(out), toolfixtures.CanaryPrefix)
			toolfixtures.AssertOnlyChanged(t, fx.Body, out, toolfixtures.Paths(fx.InjectionSegments())...)
			toolfixtures.AssertPreserved(t, fx, out)
		})
	}
}
