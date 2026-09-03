//go:build vendorlive

package vendorconformance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	vc "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/vendorconformance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gatewaySurfaces are the ingress surfaces the data plane serves. The chat
// surface is Studio-only and lives in the root module's suite.
var gatewaySurfaces = []vc.Surface{vc.SurfaceShim, vc.SurfaceSDK, vc.SurfaceUniversal}

// TestConformance is the matrix run: every configured vendor, on every gateway
// surface, for every model slot and scenario the matrix allows.
func TestConformance(t *testing.T) {
	h := setup(t)

	cases := h.cfg.Cases(gatewaySurfaces...)
	if len(cases) == 0 {
		t.Skip("no cases to run for the configured vendors and surfaces")
	}
	t.Logf("running %d case(s)", len(cases))

	for _, c := range cases {
		t.Run(c.Name(), func(t *testing.T) {
			if c.Support == vc.Unsupported {
				if reason := vc.UnsupportedFeatureReason(c.Scenario); reason != "" {
					t.Skipf("%s is switched off platform-wide: %s", c.Scenario, reason)
				}
				t.Skipf("declared unsupported for %s (%s on %s)", c.Vendor.Key, c.Scenario, c.Surface)
			}
			t.Parallel()
			runCase(t, h, c)
		})
	}
}

func runCase(t *testing.T, h *harness, c vc.Case) {
	t.Helper()

	switch c.Scenario {
	case vc.ScenarioErrors:
		runErrorScenario(t, h, c)
		return
	case vc.ScenarioVision, vc.ScenarioJSONMode, vc.ScenarioTools, vc.ScenarioToolsStreaming:
		// The sdk surface takes vendor-native bodies; these scenarios build
		// OpenAI-format ones. Translating each scenario into three native
		// dialects would be testing our test code, not the product.
		if c.Surface == vc.SurfaceSDK {
			t.Skipf("%s is an OpenAI-format scenario; the sdk surface takes native bodies", c.Scenario)
		}
	}

	req, ok := vc.BuildRequest(c, h.cfg)
	require.True(t, ok, "no request builder for scenario %s", c.Scenario)
	req = req.WithModel(h.ModelString(c.Surface, c.Vendor, c.Model))

	url := h.URLFor(c.Surface, c.Vendor)
	if c.Surface == vc.SurfaceSDK {
		var skip bool
		url, req, skip = adaptForNativeSurface(url, req, c)
		if skip {
			t.Skipf("no native request shape defined for vendor %s", c.Vendor.Vendor)
		}
	}

	if req.IsStreaming() {
		runStreamingCase(t, h, c, url, req)
		return
	}
	runBufferedCase(t, h, c, url, req)
}

// runBufferedCase covers every non-streaming scenario: send, validate against
// the canonical schema for the surface's wire format, then assert the invariants
// a schema cannot express.
func runBufferedCase(t *testing.T, h *harness, c vc.Case, url string, req vc.ChatRequest) {
	t.Helper()

	ctx, cancel := h.CallContext(context.Background())
	defer cancel()

	resp, err := h.post(ctx, url, req.JSON(), vendorHeaders(c.Surface, c.Vendor))
	require.NoError(t, err, "POST %s", url)

	if !assertUpstreamOK(t, h, c, resp) {
		return
	}
	requireJSON(t, resp.Body)

	format := responseFormat(c.Surface, c.Vendor)
	res, err := vc.Validate(format, resp.Body)
	require.NoError(t, err, "validating response")

	h.recorder.RecordDrift(driftScope(c, format), res.Drift)
	if !res.OK() {
		h.recorder.RecordFailure(c.Name(), res.Error(), resp.Body)
		t.Errorf("%s\n\nresponse:\n%s", res.Error(), bodyExcerpt(resp.Body))
	}
	if h.cfg.StrictUnknownFields && len(res.Drift) > 0 {
		t.Errorf("strict mode: %d undocumented field(s): %v", len(res.Drift), res.Drift)
	}

	env, err := vc.Normalize(format, resp.Body)
	require.NoError(t, err, "normalizing response:\n%s", bodyExcerpt(resp.Body))

	// Save the body whenever this case fails for any reason, not only on a
	// schema violation. A semantic failure — a tool call whose arguments came
	// back empty, say — is schema-valid by construction, so without this the
	// most interesting failures are the ones that leave no evidence behind.
	t.Cleanup(func() {
		if t.Failed() {
			h.recorder.RecordFailure(c.Name(), "assertion failed", resp.Body)
		}
	})

	assertEnvelope(t, c, env)
	assertScenarioSpecifics(t, c, env, resp.Body)
}

// runStreamingCase covers the streaming scenarios. Every frame is validated
// individually, then the cross-chunk invariants run over the whole stream.
func runStreamingCase(t *testing.T, h *harness, c vc.Case, url string, req vc.ChatRequest) {
	t.Helper()

	ctx, cancel := h.CallContext(context.Background())
	defer cancel()

	stream, meta, err := h.postStream(ctx, url, req.JSON(), vendorHeaders(c.Surface, c.Vendor))
	require.NoError(t, err, "POST %s (stream)", url)

	if !assertUpstreamOK(t, h, c, meta) {
		return
	}

	// A 200 with no stream means the gateway answered a streaming request with a
	// buffered one. Report that plainly rather than as a cascade of missing-frame
	// assertions: an SDK reading this gets a Content-Type it does not expect and
	// never sees a single token.
	if stream == nil {
		h.recorder.RecordFailure(c.Name(), "streaming request answered with a buffered response", meta.Body)
		t.Fatalf("stream:true returned Content-Type %q instead of text/event-stream, with no SSE frames:\n%s",
			meta.Header.Get("Content-Type"), bodyExcerpt(meta.Body))
	}

	// A 200 whose only frame is an error envelope is an upstream failure that
	// survived the retry loop, not a schema problem. Reporting it as a pile of
	// missing-field violations would bury the actual cause.
	if errBody, isErr := streamCarriedUpstreamError(stream); isErr {
		h.recorder.RecordFailure(c.Name(), "upstream error delivered in-stream", errBody)
		if c.Support == vc.ModelDependent {
			t.Skipf("declared model-dependent and the stream carried an upstream error:\n%s", bodyExcerpt(errBody))
		}
		t.Fatalf("stream carried an upstream error instead of content:\n%s", bodyExcerpt(errBody))
	}

	ct := meta.Header.Get("Content-Type")
	assert.Containsf(t, ct, "text/event-stream",
		"a streaming response must be text/event-stream, got %q; clients dispatch on this header", ct)

	format, openAIConventions := streamFormat(c.Surface, c.Vendor)
	scope := driftScope(c, format)
	for _, frame := range stream.DataFrames() {
		res, err := vc.Validate(format, frame.Raw)
		if err != nil {
			t.Errorf("frame %d is not valid JSON: %v\n%s", frame.Index, err, bodyExcerpt(frame.Raw))
			continue
		}
		h.recorder.RecordDrift(scope, res.Drift)
		if !res.OK() {
			h.recorder.RecordFailure(fmt.Sprintf("%s-frame%d", c.Name(), frame.Index), res.Error(), frame.Raw)
			t.Errorf("frame %d: %s\n%s", frame.Index, res.Error(), bodyExcerpt(frame.Raw))
		}
		if h.cfg.StrictUnknownFields && len(res.Drift) > 0 {
			t.Errorf("strict mode: frame %d has undocumented field(s): %v", frame.Index, res.Drift)
		}
	}

	expectTools := c.Scenario == vc.ScenarioToolsStreaming

	// The two dialects have genuinely different contracts, so they get different
	// invariant checks. Holding Anthropic's typed event stream to OpenAI's
	// [DONE]-and-delta.role rules would report a correct response as broken.
	var env vc.Envelope
	if openAIConventions {
		for _, problem := range stream.StreamInvariants(expectTools) {
			t.Errorf("stream invariant: %s", problem)
		}
		env = stream.Envelope()
	} else {
		native := vc.AsAnthropicStream(stream)
		for _, problem := range native.StreamInvariants(expectTools) {
			t.Errorf("stream invariant: %s", problem)
		}
		env = native.Envelope()
	}

	assertEnvelope(t, c, env)

	if c.Scenario == vc.ScenarioUsage {
		// The reason this scenario exists: clients that ask for usage and do not
		// get it cannot bill, and the omission is invisible until they try.
		assert.True(t, env.HasUsage,
			"stream_options.include_usage was set but no chunk carried usage")
		if env.HasUsage {
			assertUsageSane(t, env.Usage)
		}
	}
}

// runErrorScenario asserts the error contract. A wrong error shape breaks
// clients exactly as hard as a wrong success shape, and is far less tested.
func runErrorScenario(t *testing.T, h *harness, c vc.Case) {
	t.Helper()
	if c.Surface == vc.SurfaceSDK {
		t.Skip("error shape on the native surface is the vendor's own contract, not ours")
	}
	if c.ModelSlot != vc.ModelLatest {
		t.Skip("error shape does not vary by model")
	}

	ctx, cancel := h.CallContext(context.Background())
	defer cancel()

	url := h.URLFor(c.Surface, c.Vendor)

	t.Run("unknown_model", func(t *testing.T) {
		body := vc.ChatRequest{
			"model":      h.ModelString(c.Surface, c.Vendor, "definitely-not-a-real-model-xyz"),
			"max_tokens": 16,
			"messages":   []any{map[string]any{"role": "user", "content": "ping"}},
		}.JSON()

		resp, err := h.post(ctx, url, body, nil)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, resp.Status, 400, "an unknown model must not return success")
		assert.Less(t, resp.Status, 500,
			"an unknown model is a client error; a 5xx tells the caller to retry something that can never succeed")
		assertErrorEnvelope(t, h, c, resp)
	})

	t.Run("malformed_body", func(t *testing.T) {
		resp, err := h.post(ctx, url, []byte(`{"model":`), nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.Status, "malformed JSON must be a 400")
		assertErrorEnvelope(t, h, c, resp)
	})

	t.Run("no_messages", func(t *testing.T) {
		body := vc.ChatRequest{
			"model":      h.ModelString(c.Surface, c.Vendor, c.Model),
			"max_tokens": 16,
			"messages":   []any{},
		}.JSON()

		resp, err := h.post(ctx, url, body, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, resp.Status, 400, "an empty message list must not return success")
		assertErrorEnvelope(t, h, c, resp)
	})
}

func assertErrorEnvelope(t *testing.T, h *harness, c vc.Case, resp *response) {
	t.Helper()
	if len(strings.TrimSpace(string(resp.Body))) == 0 {
		t.Errorf("error response %d had an empty body; clients have nothing to surface", resp.Status)
		return
	}
	res, err := vc.Validate(vc.FormatOpenAIError, resp.Body)
	if err != nil {
		t.Errorf("error body is not valid JSON (status %d):\n%s", resp.Status, bodyExcerpt(resp.Body))
		return
	}
	h.recorder.RecordDrift(driftScope(c, vc.FormatOpenAIError), res.Drift)
	if !res.OK() {
		h.recorder.RecordFailure(c.Name()+"-error", res.Error(), resp.Body)
		t.Errorf("error envelope (status %d): %s\n%s", resp.Status, res.Error(), bodyExcerpt(resp.Body))
	}
}

// assertUpstreamOK reports whether the exchange succeeded, distinguishing our
// bugs from the vendor's. A ModelDependent case that fails upstream is logged
// and skipped rather than failed: we declared we could not predict it.
func assertUpstreamOK(t *testing.T, h *harness, c vc.Case, resp *response) bool {
	t.Helper()
	if resp.Status == http.StatusOK {
		return true
	}

	h.recorder.RecordFailure(c.Name(), fmt.Sprintf("HTTP %d", resp.Status), resp.Body)

	if skipIfThrottled(t, resp) {
		return false
	}

	// A model that cannot do what the scenario asked tells us nothing about
	// whether our translation is correct. Skipped with the vendor's own words so
	// the gap is visible rather than silently absent.
	if gap, ok := modelCapabilityGap(resp.Body); ok {
		t.Skipf("model %q does not support this scenario; upstream said:\n%s", c.Model, gap)
		return false
	}

	if c.Support == vc.ModelDependent {
		t.Skipf("declared model-dependent and upstream returned %d:\n%s", resp.Status, bodyExcerpt(resp.Body))
		return false
	}
	t.Errorf("expected 200, got %d:\n%s", resp.Status, bodyExcerpt(resp.Body))
	return false
}

// assertEnvelope checks the invariants every successful turn must satisfy,
// whatever the vendor, surface or scenario.
func assertEnvelope(t *testing.T, c vc.Case, env vc.Envelope) {
	t.Helper()

	assert.Equal(t, "assistant", env.Role, "assistant turns must carry role=assistant")
	assert.NotEmpty(t, env.FinishReason, "a completed turn must say why it ended")
	assert.NotEqual(t, vc.ContentEmpty, env.ContentKind,
		"the turn produced neither text nor tool calls")

	if want := vc.ExpectedFinishReason(c.Scenario); want != "" {
		assert.Equalf(t, want, env.FinishReason,
			"scenario %s expects finish_reason=%s", c.Scenario, want)
	}

	// max_tokens is set on every request, so a "length" finish means the cap is
	// too low for this scenario and the result says nothing about translation.
	if env.FinishReason == "length" && c.Scenario != vc.ScenarioLongContext {
		t.Logf("note: hit max_tokens (%s); raise VENDOR_TESTS_MAX_TOKENS if this recurs", c.Scenario)
	}

	if env.HasUsage {
		assertUsageSane(t, env.Usage)
	}
}

// assertUsageSane checks the token counters. Usage arithmetic has been the
// single largest source of schema bugs in this codebase.
func assertUsageSane(t *testing.T, u vc.Usage) {
	t.Helper()
	assert.GreaterOrEqual(t, u.Prompt, 0, "prompt_tokens must not be negative")
	assert.GreaterOrEqual(t, u.Completion, 0, "completion_tokens must not be negative")
	assert.Greater(t, u.Prompt, 0, "a request with messages must report prompt tokens")
	assert.Greater(t, u.Total, 0, "total_tokens must be reported")

	if !u.Consistent() {
		// Reported, not failed: some vendors fold reasoning or cache tokens into
		// the total. A mismatch is worth eyes, not an automatic red build.
		t.Logf("note: usage does not sum (%s); expected prompt+completion == total", u)
	}
}

// assertScenarioSpecifics runs the checks that only make sense for one scenario.
func assertScenarioSpecifics(t *testing.T, c vc.Case, env vc.Envelope, body []byte) {
	t.Helper()

	switch c.Scenario {
	case vc.ScenarioTools:
		require.NotEmpty(t, env.ToolCallNames, "expected a tool call")
		assert.Contains(t, env.ToolCallNames, "get_weather")
		assertToolArgumentsAreEncodedJSON(t, c, body)

	case vc.ScenarioJSONMode:
		assertResponseContentIsJSON(t, body)

	case vc.ScenarioBasic, vc.ScenarioSystem, vc.ScenarioMultiturn, vc.ScenarioLongContext:
		assert.Equal(t, vc.ContentText, env.ContentKind, "expected a text answer")
	}
}

// assertToolArgumentsAreEncodedJSON is the check for the bug this suite exists
// to kill: on an OpenAI-format surface, tool arguments must be a JSON-encoded
// STRING whose contents parse. The schema pins the type; this pins the contents.
func assertToolArgumentsAreEncodedJSON(t *testing.T, c vc.Case, body []byte) {
	t.Helper()
	if c.Surface == vc.SurfaceSDK {
		return // native dialects encode arguments as objects by design
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(body, &parsed))
	require.NotEmpty(t, parsed.Choices)

	for i, tc := range parsed.Choices[0].Message.ToolCalls {
		assert.NotEmptyf(t, tc.ID, "tool call %d has no id; a tool result cannot be correlated back to it", i)
		assert.Truef(t, json.Valid([]byte(tc.Function.Arguments)),
			"tool call %d arguments are not valid JSON: %q", i, tc.Function.Arguments)

		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil {
			assert.Containsf(t, args, "city",
				"tool call %d arguments lost the declared parameter: %v", i, args)
		}
	}
}

func assertResponseContentIsJSON(t *testing.T, body []byte) {
	t.Helper()
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Choices) == 0 {
		return
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	assert.Truef(t, json.Valid([]byte(content)),
		"json_object mode returned content that is not JSON: %s", bodyExcerpt([]byte(content)))
}

// adaptForNativeSurface rewrites an OpenAI-format request into the vendor's own
// dialect for the sdk surface, and substitutes the model into the URL where the
// vendor puts it there. Returns skip=true for vendors with no native shape here.
func adaptForNativeSurface(url string, req vc.ChatRequest, c vc.Case) (string, vc.ChatRequest, bool) {
	switch c.Vendor.Vendor {
	case "anthropic":
		// Anthropic requires max_tokens and takes system as a top-level field.
		out := vc.ChatRequest{
			"model":      c.Model,
			"max_tokens": req["max_tokens"],
			"messages":   req["messages"],
		}
		if msgs, ok := req["messages"].([]any); ok {
			var kept []any
			for _, m := range msgs {
				msg, _ := m.(map[string]any)
				if msg["role"] == "system" {
					out["system"] = msg["content"]
					continue
				}
				kept = append(kept, m)
			}
			out["messages"] = kept
		}
		if req.IsStreaming() {
			out["stream"] = true
		}
		return url, out, false

	case "google_ai":
		// Gemini puts the model in the path and uses contents/parts, not messages.
		url = strings.ReplaceAll(url, "{model}", c.Model)
		contents := []any{}
		if msgs, ok := req["messages"].([]any); ok {
			for _, m := range msgs {
				msg, _ := m.(map[string]any)
				text, isText := msg["content"].(string)
				if !isText {
					continue
				}
				role := "user"
				switch msg["role"] {
				case "assistant":
					role = "model"
				case "system":
					continue // systemInstruction is a separate field; not exercised here
				}
				contents = append(contents, map[string]any{
					"role":  role,
					"parts": []any{map[string]any{"text": text}},
				})
			}
		}
		if len(contents) == 0 {
			return url, nil, true
		}
		return url, vc.ChatRequest{
			"contents": contents,
			"generationConfig": map[string]any{
				"maxOutputTokens": req["max_tokens"],
			},
		}, false

	case "openai", "ollama", "huggingface":
		out := req.WithModel(c.Model)
		if c.Vendor.Key == "openai" {
			// Current OpenAI models reject max_tokens outright and require
			// max_completion_tokens. The shim translates this for us, which is
			// exactly why the sdk surface must not: it forwards bodies verbatim,
			// so the test has to send what the vendor actually accepts. A 400
			// here would be the harness's fault, not the product's.
			if v, ok := out["max_tokens"]; ok {
				delete(out, "max_tokens")
				out["max_completion_tokens"] = v
			}
		}
		return url, out, false

	default:
		return url, nil, true
	}
}

func driftScope(c vc.Case, format vc.Format) string {
	return fmt.Sprintf("%s/%s/%s", c.Vendor.Key, c.Surface, format)
}
