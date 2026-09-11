package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// endpointShapeHarness is one live proxy (both hops served on a real port), one
// app with a credential, and one LLM route pointed at a fake vendor that records
// every path it is called on. It exists so the same request can be replayed
// against every endpoint shape an operator might configure.
type endpointShapeHarness struct {
	t        *testing.T
	port     int
	apiKey   string
	mu       sync.Mutex
	paths    []string
	upstream *httptest.Server
}

func newEndpointShapeHarness(t *testing.T, vendor models.Vendor, endpointSuffix string, serve func(w http.ResponseWriter, r *http.Request)) *endpointShapeHarness {
	t.Helper()
	db, cancel := setupTest(t)
	t.Cleanup(func() { tearDownTest(db, cancel) })

	h := &endpointShapeHarness{t: t}
	h.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.paths = append(h.paths, r.URL.Path)
		h.mu.Unlock()
		serve(w, r)
	}))
	t.Cleanup(h.upstream.Close)

	service := services.NewService(db)
	budgetSvc := budget.NewService(db, services.NewTestNotificationService(db))

	user, err := service.CreateUser(services.UserDTO{Email: "shapes@example.com", Name: "shapes", Password: "password123"})
	require.NoError(t, err)

	llm := &models.LLM{
		Name:        "Shape Route",
		Vendor:      vendor,
		Active:      true,
		APIEndpoint: h.upstream.URL + endpointSuffix,
		APIKey:      "vendor-key",
	}
	require.NoError(t, db.Create(llm).Error)

	app, err := service.CreateApp("Shape App", "", user.ID, nil, []uint{llm.ID}, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, service.ActivateAppCredential(app.ID))
	app, err = service.GetAppByID(app.ID)
	require.NoError(t, err)
	require.NotNil(t, app.Credential)
	h.apiKey = app.Credential.Secret

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	h.port = ln.Addr().(*net.TCPAddr).Port

	p := NewProxy(service, &Config{Port: h.port}, budgetSvc)
	require.NoError(t, p.loadResources())
	srv := &http.Server{Handler: p.createHandler()}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return h
}

func (h *endpointShapeHarness) post(path, body string) (*http.Response, []byte) {
	h.t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(h.port)+path, bytes.NewBufferString(body))
	require.NoError(h.t, err)
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(h.t, err)
	return resp, respBody
}

func (h *endpointShapeHarness) upstreamPaths() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.paths...)
}

// Endpoint shapes operators configure for a vendor whose API lives under /v1,
// and the single path the vendor must receive for each.
var v1EndpointShapes = []struct {
	name     string
	suffix   string
	wantPath func(nativeSuffix string) string
}{
	{"bare host", "", func(s string) string { return "/v1" + s }},
	{"bare host, trailing slash", "/", func(s string) string { return "/v1" + s }},
	{"versioned root", "/v1", func(s string) string { return "/v1" + s }},
	{"versioned root, trailing slash", "/v1/", func(s string) string { return "/v1" + s }},
	{"proxy prefix without version", "/gw/vendor", func(s string) string { return "/gw/vendor/v1" + s }},
	{"proxy prefix with version", "/gw/vendor/v1", func(s string) string { return "/gw/vendor/v1" + s }},
	{"proxy prefix with version, trailing slash", "/gw/vendor/v1/", func(s string) string { return "/gw/vendor/v1" + s }},
	{"cloudflare-style: version first, vendor last", "/v1/acct/gw/vendor", func(s string) string { return "/v1/acct/gw/vendor/v1" + s }},
}

func anthropicMessageJSON(text string) string {
	return `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"` + text + `"}],` +
		`"model":"claude-sonnet-5","stop_reason":"end_turn","usage":{"input_tokens":6,"output_tokens":5}}`
}

func serveAnthropic(t *testing.T) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/messages") || strings.Contains(r.URL.Path, "/v1/v1/") {
			w.WriteHeader(http.StatusNotFound) // api.anthropic.com answers unknown paths with a bare 404
			return
		}
		var req struct {
			Stream bool `json:"stream"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		if !req.Stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(anthropicMessageJSON("This is a test!")))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		events := []string{
			`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5","content":[],"stop_reason":null,"usage":{"input_tokens":6,"output_tokens":1}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"This is "}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"a test!"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`,
			`{"type":"message_stop"}`,
		}
		for _, ev := range events {
			var typed struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal([]byte(ev), &typed)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", typed.Type, ev)
			flusher.Flush()
		}
	}
}

func serveOpenAI(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/v1/chat/completions") || strings.Contains(r.URL.Path, "/v1/v1/") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-4o",` +
		`"choices":[{"index":0,"message":{"role":"assistant","content":"This is a test!"},"finish_reason":"stop"}],` +
		`"usage":{"prompt_tokens":6,"completion_tokens":5,"total_tokens":11}}`))
}

const openAIChatBody = `{"model":"any","messages":[{"role":"user","content":"Say this is a test!"}]}`

// Every entry point that reaches an Anthropic route must land on the vendor's
// /v1/messages, whatever endpoint shape the operator configured: the OpenAI
// bridge (which the unified /v1/chat/completions router rewrites into), its
// streaming variant, and the native passthrough. A version segment configured
// on the endpoint must never be doubled, and one missing from it must be
// supplied.
func TestAnthropicRoute_AllEntryPointsForEveryEndpointShape(t *testing.T) {
	for _, shape := range v1EndpointShapes {
		t.Run(shape.name, func(t *testing.T) {
			h := newEndpointShapeHarness(t, models.ANTHROPIC, shape.suffix, serveAnthropic(t))
			want := shape.wantPath("/messages")

			t.Run("bridge", func(t *testing.T) {
				resp, body := h.post("/ai/shape-route/v1/chat/completions", openAIChatBody)
				require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
				var out ChatCompletionResponse
				require.NoError(t, json.Unmarshal(body, &out))
				require.Len(t, out.Choices, 1)
				assert.Equal(t, "This is a test!", out.Choices[0].Message.Content)
			})

			t.Run("unified router", func(t *testing.T) {
				resp, body := h.post("/v1/chat/completions",
					`{"model":"shape-route/claude-sonnet-5","messages":[{"role":"user","content":"Say this is a test!"}]}`)
				require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
			})

			t.Run("bridge streaming", func(t *testing.T) {
				resp, body := h.post("/ai/shape-route/v1/chat/completions",
					`{"model":"any","stream":true,"messages":[{"role":"user","content":"Say this is a test!"}]}`)
				require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
				assert.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
				var text strings.Builder
				for _, line := range strings.Split(string(body), "\n") {
					if !strings.HasPrefix(line, "data: ") || strings.HasSuffix(line, "[DONE]") {
						continue
					}
					var chunk ChatCompletionChunk
					require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk), "frame: %s", line)
					for _, c := range chunk.Choices {
						text.WriteString(c.Delta.Content)
					}
				}
				assert.Equal(t, "This is a test!", text.String())
			})

			t.Run("native passthrough", func(t *testing.T) {
				resp, body := h.post("/llm/call/shape-route/v1/messages",
					`{"model":"claude-sonnet-5","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)
				require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
			})

			t.Run("native passthrough streaming", func(t *testing.T) {
				resp, body := h.post("/llm/call/shape-route/v1/messages",
					`{"model":"claude-sonnet-5","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
				require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
				assert.Contains(t, string(body), `"type":"message_stop"`)
			})

			for i, got := range h.upstreamPaths() {
				assert.Equal(t, want, got, "upstream call #%d", i+1)
			}
			assert.Len(t, h.upstreamPaths(), 5, "each entry point must reach the vendor exactly once")
		})
	}
}

// The same guarantee for OpenAI-wire routes, including OpenAI-compatible
// providers that mount their API under a prefix (Fireworks: /inference/v1,
// Groq: /openai/v1). The bridge and the passthrough both send
// /v1/chat/completions, so a configured /inference/v1 used to become
// /inference/v1/v1/chat/completions.
func TestOpenAIRoute_BridgeAndPassthroughForEveryEndpointShape(t *testing.T) {
	shapes := append([]struct {
		name     string
		suffix   string
		wantPath func(string) string
	}{
		{"fireworks-style prefix", "/inference/v1", func(s string) string { return "/inference/v1" + s }},
		{"groq-style prefix", "/openai/v1", func(s string) string { return "/openai/v1" + s }},
	}, v1EndpointShapes...)

	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			h := newEndpointShapeHarness(t, models.OPENAI, shape.suffix, serveOpenAI)
			want := shape.wantPath("/chat/completions")

			resp, body := h.post("/ai/shape-route/v1/chat/completions", openAIChatBody)
			require.Equal(t, http.StatusOK, resp.StatusCode, "bridge body: %s", body)

			resp, body = h.post("/v1/chat/completions",
				`{"model":"shape-route/gpt-4o","messages":[{"role":"user","content":"Say this is a test!"}]}`)
			require.Equal(t, http.StatusOK, resp.StatusCode, "unified body: %s", body)

			resp, body = h.post("/llm/call/shape-route/v1/chat/completions", openAIChatBody)
			require.Equal(t, http.StatusOK, resp.StatusCode, "native body: %s", body)

			assert.Equal(t, []string{want, want, want}, h.upstreamPaths())
		})
	}
}

// A client's query string must survive the join on both hops (Gemini relies on
// ?alt=sse and ?key=), and a proxy prefix must not be mistaken for an overlap
// just because it shares text with the caller's path.
func TestPassthrough_QueryStringAndLookalikePrefixSurviveJoin(t *testing.T) {
	var gotQuery, gotPath string
	h := newEndpointShapeHarness(t, models.OPENAI, "/v1beta-compat", func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		serveOpenAI(w, r)
	})
	// /v1beta-compat shares a prefix with /v1 but is not the same segment.
	resp, body := h.post("/llm/call/shape-route/v1/chat/completions?alt=sse&key=abc", openAIChatBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, "/v1beta-compat/v1/chat/completions", gotPath)
	assert.Equal(t, "alt=sse&key=abc", gotQuery)
}
