package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/generative-ai-go/genai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// tool_choice was ignored for Anthropic, Google and Bedrock on the
// OpenAI-shaped bridges (/v1, /ai/): a forced function, "required" and
// "none" never reached the vendor, which breaks agent frameworks that force
// a tool (midsommar-clean-6bg5.22). It is now translated per vendor, and a
// value that cannot be translated is refused with 400, never ignored.

const bridgeTools = `"tools":[` +
	`{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}},` +
	`{"type":"function","function":{"name":"get_time","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]`

func bridgeToolBody(model, toolChoice string, stream bool) string {
	body := `{"model":"` + model + `","max_tokens":200,"messages":[{"role":"user","content":"What is the weather in Paris?"}],` + bridgeTools
	if toolChoice != "" {
		body += `,"tool_choice":` + toolChoice
	}
	if stream {
		body += `,"stream":true`
	}
	return body + `}`
}

const forcedGetTime = `{"type":"function","function":{"name":"get_time"}}`

// capturingVendor records the request bodies a fake vendor receives.
type capturingVendor struct {
	mu     sync.Mutex
	bodies [][]byte
	serve  http.HandlerFunc
}

func (c *capturingVendor) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	c.bodies = append(c.bodies, body)
	c.mu.Unlock()
	r.Body = io.NopCloser(&sliceReader{b: body})
	c.serve(w, r)
}

type sliceReader struct {
	b []byte
	i int
}

func (s *sliceReader) Read(p []byte) (int, error) {
	if s.i >= len(s.b) {
		return 0, io.EOF
	}
	n := copy(p, s.b[s.i:])
	s.i += n
	return n, nil
}

func (c *capturingVendor) last(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	require.NotEmpty(t, c.bodies, "the vendor was not called")
	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(c.bodies[len(c.bodies)-1], &out), string(c.bodies[len(c.bodies)-1]))
	return out
}

// serveGeminiAnyShape answers generateContent, and streamGenerateContent as
// server-sent events or as a JSON array, whichever the driver asks for.
func serveGeminiAnyShape(w http.ResponseWriter, r *http.Request) {
	const chunk = `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],` +
		`"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1,"totalTokenCount":6}}`
	switch {
	case !strings.Contains(r.URL.Path, "streamGenerateContent"):
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chunk))
	case r.URL.Query().Get("alt") == "sse":
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
	default:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[" + chunk + "]"))
	}
}

func TestBridgeToolChoice_Bedrock(t *testing.T) {
	for _, tc := range []struct {
		name       string
		toolChoice string
		want       string // toolConfig.toolChoice as sent; "" = absent
		noTools    bool   // no toolConfig at all
	}{
		{name: "no tool_choice: the request is what it was", want: ""},
		{name: "auto", toolChoice: `"auto"`, want: `{"auto":{}}`},
		{name: "required", toolChoice: `"required"`, want: `{"any":{}}`},
		{name: "a function", toolChoice: forcedGetTime, want: `{"tool":{"name":"get_time"}}`},
		{name: "none: no tools are sent", toolChoice: `"none"`, noTools: true},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "", true: ", streamed"}[stream], func(t *testing.T) {
				h, v := converseHarness(t, false)
				resp, out := h.post("/ai/primary/v1/chat/completions", bridgeToolBody("anthropic.claude-3", tc.toolChoice, stream))
				require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
				calls := v.calls()
				require.Len(t, calls, 1)
				var sent struct {
					ToolConfig *struct {
						Tools      []json.RawMessage `json:"tools"`
						ToolChoice json.RawMessage   `json:"toolChoice"`
					} `json:"toolConfig"`
				}
				require.NoError(t, json.Unmarshal(calls[0].Body, &sent), string(calls[0].Body))
				if tc.noTools {
					assert.Nil(t, sent.ToolConfig, string(calls[0].Body))
					return
				}
				require.NotNil(t, sent.ToolConfig, string(calls[0].Body))
				assert.Len(t, sent.ToolConfig.Tools, 2)
				if tc.want == "" {
					assert.Empty(t, sent.ToolConfig.ToolChoice, "no toolChoice without tool_choice")
				} else {
					assert.JSONEq(t, tc.want, string(sent.ToolConfig.ToolChoice))
				}
			})
		}
	}
}

func TestBridgeToolChoice_RefusedWith400(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"a function that is not in tools", bridgeToolBody("anthropic.claude-3", `{"type":"function","function":{"name":"get_stock"}}`, false), `which is not in tools`},
		{"an unknown word", bridgeToolBody("anthropic.claude-3", `"sometimes"`, false), "unsupported tool_choice"},
		{"an object that names no function", bridgeToolBody("anthropic.claude-3", `{"type":"allowed_tools"}`, false), "unsupported tool_choice"},
		{"required without tools", `{"model":"anthropic.claude-3","messages":[{"role":"user","content":"hi"}],"tool_choice":"required"}`, "needs at least one tool"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, v := converseHarness(t, false)
			resp, out := h.post("/ai/primary/v1/chat/completions", tc.body)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
			assert.Contains(t, string(out), tc.want)
			assert.Empty(t, v.calls(), "nothing reaches the vendor")
		})
	}
}

// Bedrock cannot be told to call no tool, and refuses a conversation that
// holds tool calls without the tool definitions. Agent frameworks send
// exactly that on the turn that should only summarise, and it worked before
// (with "none" ignored), so it still does: the tools are sent, no toolChoice.
func TestBridgeToolChoice_BedrockNoneWithToolHistoryStillWorks(t *testing.T) {
	body := `{"model":"anthropic.claude-3","messages":[{"role":"user","content":"time?"},` +
		`{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"get_time","arguments":"{}"}}]},` +
		`{"role":"tool","tool_call_id":"c1","content":"12:00"}],` + bridgeTools + `,"tool_choice":"none"}`
	h, v := converseHarness(t, false)
	resp, out := h.post("/ai/primary/v1/chat/completions", body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	calls := v.calls()
	require.Len(t, calls, 1)
	var sent struct {
		ToolConfig *struct {
			Tools      []json.RawMessage `json:"tools"`
			ToolChoice json.RawMessage   `json:"toolChoice"`
		} `json:"toolConfig"`
	}
	require.NoError(t, json.Unmarshal(calls[0].Body, &sent), string(calls[0].Body))
	require.NotNil(t, sent.ToolConfig, "the tool definitions the conversation needs are sent")
	assert.Len(t, sent.ToolConfig.Tools, 2)
	assert.Empty(t, sent.ToolConfig.ToolChoice)
}

// Anthropic and Google go through the in-tree langchaingo drivers, which
// dropped tool_choice. The vendor now receives it in its own shape, and a
// request without tool_choice carries none.
func TestBridgeToolChoice_AnthropicAndGoogle(t *testing.T) {
	for _, vendor := range []struct {
		name   string
		vendor models.Vendor
		model  string
		serve  func(t *testing.T) http.HandlerFunc
		field  string
		want   map[string]string
	}{
		{"anthropic", models.ANTHROPIC, "claude-sonnet-5", func(t *testing.T) http.HandlerFunc { return serveAnthropic(t) }, "tool_choice", map[string]string{
			`"auto"`:      `{"type":"auto"}`,
			`"required"`:  `{"type":"any"}`,
			`"none"`:      `{"type":"none"}`,
			forcedGetTime: `{"type":"tool","name":"get_time"}`,
		}},
		// The SDK sends the mode as its enum number.
		{"google", models.GOOGLEAI, "gemini-2.5-flash", func(*testing.T) http.HandlerFunc { return serveGeminiAnyShape }, "toolConfig", map[string]string{
			`"auto"`:      fmt.Sprintf(`{"functionCallingConfig":{"mode":%d}}`, genai.FunctionCallingAuto),
			`"required"`:  fmt.Sprintf(`{"functionCallingConfig":{"mode":%d}}`, genai.FunctionCallingAny),
			`"none"`:      fmt.Sprintf(`{"functionCallingConfig":{"mode":%d}}`, genai.FunctionCallingNone),
			forcedGetTime: fmt.Sprintf(`{"functionCallingConfig":{"mode":%d,"allowedFunctionNames":["get_time"]}}`, genai.FunctionCallingAny),
		}},
	} {
		t.Run(vendor.name, func(t *testing.T) {
			capture := &capturingVendor{serve: vendor.serve(t)}
			h := newFailoverHarness(t, capture.handler, serveOpenAIText("never"), func(primary, _ *models.LLM) {
				primary.Vendor = vendor.vendor
				primary.Failover = models.LLMFailover{}
				primary.DefaultModel = vendor.model
			})

			resp, out := h.post("/ai/primary/v1/chat/completions", bridgeToolBody(vendor.model, "", false))
			require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
			sent := capture.last(t)
			assert.NotContains(t, sent, vendor.field, "no tool_choice in, none out")
			assert.Contains(t, sent, "tools")

			for choice, want := range vendor.want {
				resp, out := h.post("/ai/primary/v1/chat/completions", bridgeToolBody(vendor.model, choice, false))
				require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", choice, out)
				sent := capture.last(t)
				require.Contains(t, sent, vendor.field, "tool_choice %s was dropped: %v", choice, sent)
				assert.JSONEq(t, want, string(sent[vendor.field]), choice)
			}

			before := len(capture.bodies)
			resp, out = h.post("/ai/primary/v1/chat/completions", bridgeToolBody(vendor.model, `{"type":"function","function":{"name":"get_stock"}}`, false))
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
			assert.Len(t, capture.bodies, before, "nothing reaches the vendor")
		})
	}
}

// OpenAI-compatible vendors take tool_choice as the client sent it, a shape
// this gateway does not know included: nothing is validated or rewritten.
func TestBridgeToolChoice_OpenAIIsForwardedUntouched(t *testing.T) {
	capture := &capturingVendor{serve: serveOpenAIText("ok")}
	h := newFailoverHarness(t, capture.handler, serveOpenAIText("never"), noFailover)
	for _, choice := range []string{forcedGetTime, `"required"`, `{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[]}}`} {
		resp, out := h.post("/ai/primary/v1/chat/completions", bridgeToolBody("gpt-4o", choice, false))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
		assert.JSONEq(t, choice, string(capture.last(t)["tool_choice"]))
	}
}
