package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
)

// converseVendor is a fake Bedrock runtime: Converse answers a JSON message
// with usage (cache tokens included), ConverseStream the stream given. It
// checks the SigV4 signature for us-east-1 with the LLM's own keys, as AWS
// does, and keeps every request it got.
type converseVendor struct {
	t      *testing.T
	stream []byte

	mu      sync.Mutex
	status  int    // answer every request with this status and body, when set
	errBody string // ...this body
	reqs    []vendorCall
}

const converseAnswer = `{"output":{"message":{"role":"assistant","content":[{"text":"Hello from Bedrock"}]}},"stopReason":"end_turn",` +
	`"usage":{"inputTokens":11,"outputTokens":7,"totalTokens":18,"cacheReadInputTokens":3,"cacheWriteInputTokens":2}}`

func (v *converseVendor) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	v.mu.Lock()
	v.reqs = append(v.reqs, vendorCall{Path: r.URL.EscapedPath(), Headers: r.Header.Clone(), Body: body})
	status, errBody := v.status, v.errBody
	v.mu.Unlock()
	if status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(errBody))
		return
	}
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	var signed http.Header
	check := sigV4VerifyHandler("AKIDTEST", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", &signed)
	if strings.HasSuffix(r.URL.Path, "/converse-stream") {
		sigV4StreamHandlerWith(v.stream, check)(w, r)
		return
	}
	rec := newStatusRecorder()
	check(rec, r)
	if rec.code != http.StatusOK {
		w.WriteHeader(rec.code)
		_, _ = w.Write([]byte(rec.body.String()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(converseAnswer))
}

func (v *converseVendor) calls() []vendorCall {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]vendorCall(nil), v.reqs...)
}

func (v *converseVendor) fail(status int, body string) {
	v.mu.Lock()
	v.status, v.errBody = status, body
	v.mu.Unlock()
}

// converseHarness is a Bedrock primary on converseVendor, with no failover
// unless failover is set (an OpenAI fallback that answers "fallback").
func converseHarness(t *testing.T, failover bool, filters ...*models.Filter) (*failoverHarness, *converseVendor) {
	v := &converseVendor{t: t, stream: bedrockStream(t, "Sure. ", `{"to":"x"}`)}
	h := newFailoverHarness(t, v.serve, serveOpenAIText("fallback"), func(primary, _ *models.LLM) {
		primary.Vendor = models.BEDROCK
		if !failover {
			primary.Failover = models.LLMFailover{}
		}
		primary.DefaultModel = "anthropic.claude-3"
		primary.Metadata = models.JSONMap{"aws_access_key_id": "AKIDTEST", "aws_secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "aws_region": "us-east-1"}
		primary.Filters = filters
	})
	bedrockVendor.InvalidateClientCache(h.primary.ID)
	t.Cleanup(func() { bedrockVendor.InvalidateClientCache(h.primary.ID) })
	return h, v
}

// bridgeRequest is one request to a Bedrock bridge, authenticated as its
// clients authenticate.
type bridgeRequest struct {
	name, path, body string
	stream           bool
	anthropic        bool
}

func (b bridgeRequest) post(h *failoverHarness, headers ...string) (*http.Response, []byte) {
	if b.anthropic {
		headers = append([]string{"x-api-key", h.apiKey}, headers...)
	}
	return h.post(b.path, b.body, headers...)
}

var bedrockBridges = []bridgeRequest{
	{"ai", "/ai/primary/v1/chat/completions", `{"model":"anthropic.claude-3","messages":[{"role":"user","content":"hi"}]}`, false, false},
	{"ai stream", "/ai/primary/v1/chat/completions", `{"model":"anthropic.claude-3","stream":true,"messages":[{"role":"user","content":"hi"}]}`, true, false},
	{"unified", "/v1/chat/completions", `{"model":"primary/anthropic.claude-3","messages":[{"role":"user","content":"hi"}]}`, false, false},
	{"unified stream", "/v1/chat/completions", `{"model":"primary/anthropic.claude-3","stream":true,"messages":[{"role":"user","content":"hi"}]}`, true, false},
	{"anthropic", "/anthropic/primary/v1/messages", `{"model":"claude","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`, false, true},
	{"anthropic stream", "/anthropic/primary/v1/messages", `{"model":"claude","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`, true, true},
}

// statusRecorder is a minimal ResponseWriter that keeps the status and body.
type statusRecorder struct {
	header http.Header
	code   int
	body   strings.Builder
}

func newStatusRecorder() *statusRecorder {
	return &statusRecorder{header: http.Header{}, code: http.StatusOK}
}

func (s *statusRecorder) Header() http.Header         { return s.header }
func (s *statusRecorder) Write(b []byte) (int, error) { return s.body.WriteString(string(b)) }
func (s *statusRecorder) WriteHeader(code int)        { s.code = code }

// sigV4StreamHandlerWith answers a request check accepts with stream.
func sigV4StreamHandlerWith(stream []byte, check http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec := newStatusRecorder()
		check(rec, r)
		if rec.code != http.StatusOK {
			w.WriteHeader(rec.code)
			_, _ = w.Write([]byte(rec.body.String()))
			return
		}
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		_, _ = w.Write(stream)
	}
}

// On the Bedrock bridges the body the guardrail screens is the one sent: the
// bridge decodes the client's body (encoding/json takes a case variant, or
// the last of a repeated key) and the inner hop screens the Converse body it
// was translated to, so what the decoder kept never reaches AWS unscreened.
// (They used to screen the client's body and send the decoded one, so such
// bodies were refused.)
func TestDuplicateKeys_ScreenedOnTheBedrockBridges(t *testing.T) {
	cases := []bridgeRequest{
		{"openai repeated key", "/ai/primary/v1/chat/completions", `{"model":"anthropic.claude-3","messages":[{"role":"user","content":"benign","content":"the SECRET"}]}`, false, false},
		{"openai case variant", "/ai/primary/v1/chat/completions", `{"model":"anthropic.claude-3","messages":[{"role":"user","Content":"the SECRET"}]}`, false, false},
		{"anthropic case variant", "/anthropic/primary/v1/messages", `{"model":"claude","max_tokens":10,"messages":[{"role":"user","content":"hi","Content":"the SECRET"}]}`, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, v := converseHarness(t, false, redactGuardrail(guardrails.ScopeAllUser))
			resp, out := tc.post(h)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
			calls := v.calls()
			require.Len(t, calls, 1)
			assert.NotContains(t, string(calls[0].Body), "SECRET")
			assert.Contains(t, string(calls[0].Body), "[REDACTED]")
		})
	}
}

// decodeJSONObject is body as a map, failing the test when it is not one.
func decodeJSONObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m), string(body))
	return m
}

// chainRecorder stands in for the gateway's plugin chain: auth hooks that
// count the /llm/ requests they see (as the microgateway's do; they skip
// every other path) and a response hook that counts its phases and sets
// plugin headers (a cache's X-Cache, the Prompt Compressor's labels).
type chainRecorder struct {
	mu        sync.Mutex
	preAuth   []string // paths
	postAuth  []string
	auth      []string // paths custom auth was asked about
	postHdr   []string // X-Tyk-Compression as post_auth saw it
	headers   int
	writes    []*ResponseWriteRequest
	completes []*StreamCompleteRequest
	vendors   []string
	block     func(w http.ResponseWriter) bool // post_auth answers in place of the vendor
}

func (c *chainRecorder) install(h *failoverHarness) {
	h.proxy.SetAuthHooks(&AuthHooks{
		CustomAuth: func(r *http.Request, _ AuthTarget, _, _ string) (AuthResult, error) {
			c.mu.Lock()
			c.auth = append(c.auth, r.URL.Path)
			c.mu.Unlock()
			return AuthResult{Outcome: AuthNotHandled}, nil
		},
		PreAuth: func(_ http.ResponseWriter, r *http.Request) bool {
			if strings.HasPrefix(r.URL.Path, "/llm/") {
				c.mu.Lock()
				c.preAuth = append(c.preAuth, r.URL.EscapedPath())
				c.mu.Unlock()
			}
			return false
		},
		PostAuth: func(w http.ResponseWriter, r *http.Request, _ uint) bool {
			if !strings.HasPrefix(r.URL.Path, "/llm/") {
				return false
			}
			c.mu.Lock()
			c.postAuth = append(c.postAuth, r.URL.EscapedPath())
			c.postHdr = append(c.postHdr, r.Header.Get(hdrCompression))
			block := c.block
			c.mu.Unlock()
			return block != nil && block(w)
		},
	})
	h.proxy.AddResponseHook(c)
}

func (c *chainRecorder) GetName() string { return "chain-recorder" }

func (c *chainRecorder) OnBeforeWriteHeaders(_ context.Context, req *HeadersRequest) (*HeadersResponse, error) {
	c.mu.Lock()
	c.headers++
	c.vendors = append(c.vendors, req.Context.Vendor)
	c.mu.Unlock()
	out := map[string]string{"X-Cache": "MISS", "X-Tyk-Compression": "applied"}
	for k, v := range req.Headers {
		out[k] = v
	}
	return &HeadersResponse{Modified: true, Headers: out}, nil
}

func (c *chainRecorder) OnBeforeWrite(_ context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error) {
	c.mu.Lock()
	c.writes = append(c.writes, req)
	c.mu.Unlock()
	return &ResponseWriteResponse{Body: req.Body, Headers: req.Headers}, nil
}

func (c *chainRecorder) OnStreamComplete(_ context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error) {
	c.mu.Lock()
	c.completes = append(c.completes, req)
	c.mu.Unlock()
	return &StreamCompleteResponse{}, nil
}

func (c *chainRecorder) snapshot() chainRecorder {
	c.mu.Lock()
	defer c.mu.Unlock()
	return chainRecorder{preAuth: c.preAuth, postAuth: c.postAuth, auth: c.auth, postHdr: c.postHdr, headers: c.headers,
		writes: c.writes, completes: c.completes, vendors: c.vendors}
}

// Every Bedrock bridge loops back through /llm/call/ (8xf.12): pre_auth,
// post_auth and the response hooks run once, on the inner hop, with
// Vendor=bedrock and the Converse body; the request reaches AWS once, signed
// with the LLM's keys; the plugin headers reach the client; the client's
// X-Tyk-Compression reaches the plugins and never AWS; and the exchange
// leaves one ProxyLog with the usage the old bridges recorded, cache tokens
// included.
func TestBedrockBridges_RunTheChainOnTheInnerHop(t *testing.T) {
	for _, b := range bedrockBridges {
		t.Run(b.name, func(t *testing.T) {
			h, v := converseHarness(t, false)
			chain := &chainRecorder{}
			chain.install(h)

			resp, body := b.post(h, hdrCompression, "off")
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

			op := "converse"
			if b.stream {
				op = "converse-stream"
			}
			calls := v.calls()
			require.Len(t, calls, 1, "AWS is called once")
			assert.Equal(t, "/model/anthropic.claude-3/"+op, calls[0].Path)
			assert.Empty(t, calls[0].Headers.Get(hdrCompression), "the client's instruction is for the plugins, never AWS")
			assert.True(t, strings.HasPrefix(calls[0].Headers.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AKIDTEST/"))

			inner := "/llm/call/primary/model/anthropic.claude-3/" + op
			require.Eventually(t, func() bool {
				s := chain.snapshot()
				return len(s.writes)+len(s.completes) > 0
			}, 5*time.Second, 10*time.Millisecond)
			time.Sleep(50 * time.Millisecond) // a second call would have come by now
			s := chain.snapshot()
			assert.Equal(t, []string{inner}, s.preAuth, "pre_auth runs once, on the inner hop")
			assert.Equal(t, []string{inner}, s.postAuth, "post_auth runs once, on the inner hop")
			assert.Equal(t, []string{"off"}, s.postHdr)
			require.Len(t, s.auth, 1, "custom auth runs once: %v", s.auth)
			assert.False(t, strings.HasPrefix(s.auth[0], "/llm/"), "on the outer hop; the inner hop takes the hand-off")
			assert.Equal(t, 1, s.headers, "the header phase runs once")
			assert.Equal(t, []string{"bedrock"}, s.vendors)
			if b.stream {
				assert.Empty(t, s.writes)
				require.Len(t, s.completes, 1)
				assert.Contains(t, string(s.completes[0].AccumulatedResponse), `data: {"metadata":{"usage":`)
				assert.Contains(t, string(s.completes[0].RequestBody), `"content":[{"text":"hi"}]`, "the Converse body")
			} else {
				assert.Empty(t, s.completes)
				require.Len(t, s.writes, 1)
				assert.Contains(t, string(s.writes[0].Body), `"output":{"message"`, "the Converse answer")
			}

			assert.Equal(t, "MISS", resp.Header.Get("X-Cache"), "plugin headers reach the client")
			assert.Equal(t, "applied", resp.Header.Get("X-Tyk-Compression"))

			logs := waitForLogs(t, h, 1)
			assert.Equal(t, http.StatusOK, logs[0].ResponseCode)
			assert.Equal(t, h.primary.ID, logs[0].LLMID)
			var recs []models.LLMChatRecord
			require.Eventually(t, func() bool { recs = h.chatRecords(); return len(recs) == 1 }, 5*time.Second, 20*time.Millisecond)
			if b.stream {
				assert.Equal(t, 10, recs[0].PromptTokens)
				assert.Equal(t, 5, recs[0].ResponseTokens)
			} else {
				assert.Equal(t, 11, recs[0].PromptTokens)
				assert.Equal(t, 7, recs[0].ResponseTokens)
				assert.Equal(t, 3, recs[0].CacheReadPromptTokens)
				assert.Equal(t, 2, recs[0].CacheWritePromptTokens)
			}
			assert.Equal(t, "anthropic.claude-3", recs[0].Name, "the chat record names the model")
		})
	}
}

// What the inner hop refuses comes back in the bridge's own envelope, with
// its status, and a 4xx never fails over: a request guardrail's block (400),
// a post_auth plugin's block (its status and message), a spent budget (403).
// A plugin that blocks with a 200 and its own body, which no SDK can read as
// a Converse answer, is a 502 that says so, never an empty answer.
func TestBedrockBridges_InnerHopRefusals(t *testing.T) {
	guard := forbiddenResponseGuardrail()
	guard.ResponseFilter = false
	cases := []struct {
		name   string
		setup  func(h *failoverHarness, chain *chainRecorder)
		forbid bool // the request carries FORBIDDEN
		status int
		says   string
	}{
		{"request guardrail", func(h *failoverHarness, _ *chainRecorder) {
			h.primary.Filters = []*models.Filter{guard}
			require.NoError(t, h.db.Save(h.primary).Error)
			require.NoError(t, h.db.Model(h.primary).Association("Filters").Replace([]*models.Filter{guard}))
			require.NoError(t, h.proxy.loadResources())
		}, true, http.StatusBadRequest, "blocked by policy"},
		{"plugin block", func(_ *failoverHarness, chain *chainRecorder) {
			chain.block = func(w http.ResponseWriter) bool {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":{"message":"the plugin says no","type":"permission_error"}}`))
				return true
			}
		}, false, http.StatusForbidden, "the plugin says no"},
		// The shape the edge gives a plugin's refusal of a Bedrock runtime
		// call (8xf.30), which the bridges read the message from too.
		{"AWS-shaped plugin block", func(_ *failoverHarness, chain *chainRecorder) {
			chain.block = func(w http.ResponseWriter) bool {
				w.Header().Set(bedrockVendor.ErrorTypeHeader, "AccessDeniedException")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write(bedrockVendor.ErrorBody([]byte(`{"error":{"message":"the firewall says no"}}`)))
				return true
			}
		}, false, http.StatusForbidden, "the firewall says no"},
		// The edge's refusal of a body over the plugins' size limit
		// (respondPluginChainError), in its own {"error": ...} shape (8xf.36).
		{"edge size limit", func(_ *failoverHarness, chain *chainRecorder) {
			chain.block = func(w http.ResponseWriter) bool {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				_, _ = w.Write([]byte(`{"error":"request too large for the gateway's plugins (68157440 bytes)"}`))
				return true
			}
		}, false, http.StatusRequestEntityTooLarge, "request too large for the gateway's plugins (68157440 bytes)"},
		{"budget", func(h *failoverHarness, _ *chainRecorder) {
			h.proxy.budgetService = denyBudget{err: fmt.Errorf("budget exceeded: app monthly budget is 0")}
		}, false, http.StatusForbidden, "Budget limit exceeded"},
		{"plugin block with a 200", func(_ *failoverHarness, chain *chainRecorder) {
			chain.block = func(w http.ResponseWriter) bool {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"error":{"message":"cached refusal"}}`))
				return true
			}
		}, false, http.StatusBadGateway, "cached refusal"},
	}
	for _, tc := range cases {
		for _, b := range bedrockBridges {
			if strings.HasPrefix(b.name, "unified") {
				continue // the /ai/ handler, as tested
			}
			t.Run(tc.name+"/"+b.name, func(t *testing.T) {
				// A 5xx may fail over (the 200 case); a refusal must not.
				h, v := converseHarness(t, tc.status < http.StatusInternalServerError)
				chain := &chainRecorder{}
				chain.install(h)
				tc.setup(h, chain)
				req := b
				if tc.forbid {
					req.body = strings.Replace(req.body, `"content":"hi"`, `"content":"say FORBIDDEN"`, 1)
				}
				resp, body := req.post(h)
				require.Equal(t, tc.status, resp.StatusCode, string(body))
				assert.Contains(t, string(body), tc.says)
				assert.NotContains(t, string(body), "operation error", "the gateway's message, not the SDK's")
				if tc.status == http.StatusRequestEntityTooLarge && !b.anthropic {
					assert.Equal(t, "request_too_large", decodeOAIError(t, body).Code)
				}
				if b.anthropic {
					assert.Equal(t, "error", decodeJSONObject(t, body)["type"], "the Anthropic envelope: %s", body)
				} else {
					assert.NotNil(t, decodeOAIError(t, body))
				}
				assert.Empty(t, v.calls(), "AWS is never called")
				assert.Empty(t, h.fallbackVendor.calls(), "a refusal never fails over")
			})
		}
	}
}

// A Bedrock rung's 5xx fails over to an OpenAI rung, and an OpenAI rung's to
// a Bedrock one, buffered and streamed.
func TestBedrockBridges_Failover(t *testing.T) {
	for _, stream := range []bool{false, true} {
		body := `{"model":"anthropic.claude-3","messages":[{"role":"user","content":"hi"}]}`
		if stream {
			body = `{"model":"anthropic.claude-3","stream":true,"messages":[{"role":"user","content":"hi"}]}`
		}
		t.Run(fmt.Sprintf("bedrock to openai/stream=%v", stream), func(t *testing.T) {
			h, v := converseHarness(t, true)
			v.fail(http.StatusInternalServerError, `{"message":"internal failure"}`)
			resp, out := h.post("/ai/primary/v1/chat/completions", body)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
			assert.Contains(t, string(out), `"model":"gpt-4o"`, "the fallback answered")
			assert.Len(t, v.calls(), 1)
			assert.Len(t, h.fallbackVendor.calls(), 1)
		})
		t.Run(fmt.Sprintf("openai to bedrock/stream=%v", stream), func(t *testing.T) {
			v := &converseVendor{t: t, stream: bedrockStream(t, "Sure. ", `{"to":"x"}`)}
			h := newFailoverHarness(t, serveStatus(503), v.serve, func(_, fallback *models.LLM) {
				fallback.Vendor = models.BEDROCK
				fallback.DefaultModel = "anthropic.claude-3"
				fallback.Metadata = models.JSONMap{"aws_access_key_id": "AKIDTEST", "aws_secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "aws_region": "us-east-1"}
			})
			h.primary.Failover.Targets[0].Model = "anthropic.claude-3"
			require.NoError(t, h.db.Save(h.primary).Error)
			require.NoError(t, h.proxy.loadResources())
			resp, out := h.post("/ai/primary/v1/chat/completions", strings.Replace(body, "anthropic.claude-3", "gpt-4o", 1))
			require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
			if stream {
				assert.Contains(t, string(out), "Sure. ")
			} else {
				assert.Contains(t, string(out), "Hello from Bedrock")
			}
			require.Len(t, v.calls(), 1)
			assert.Empty(t, v.calls()[0].Headers.Get(hdrFailoverToken), "the loopback's marker never reaches AWS")
		})
	}
}

// A router that picks a Bedrock LLM loops back too: the plugins run once,
// on the inner hop of the LLM it picked.
func TestBedrockBridges_Router(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			v := &converseVendor{t: t, stream: bedrockStream(t, "Sure. ", `{"to":"x"}`)}
			var mu sync.Mutex
			var posts []string
			h := newRouterHarness(t, routerHarnessOpts{grantRouter: true,
				authHooks: &AuthHooks{PostAuth: func(_ http.ResponseWriter, r *http.Request, _ uint) bool {
					if strings.HasPrefix(r.URL.Path, "/llm/") {
						mu.Lock()
						posts = append(posts, r.URL.Path)
						mu.Unlock()
					}
					return false
				}},
				mutate: func(h *routerHarness) {
					h.big.Vendor = models.BEDROCK
					h.big.APIEndpoint = newFakeVendor(t, v.serve).server.URL
					h.big.DefaultModel = "anthropic.claude-3"
					h.big.Metadata = models.JSONMap{"aws_access_key_id": "AKIDTEST", "aws_secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "aws_region": "us-east-1"}
					require.NoError(t, h.db.Save(h.big).Error)
					big := h.resolver.routes["big"]
					big.model = "anthropic.claude-3"
					h.resolver.routes["big"] = big
				}})
			resp, body := h.do(http.MethodPost, "/ai/smart/v1/chat/completions", chatBody("big", stream))
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
			if stream {
				assert.Contains(t, string(body), "Sure. ")
			} else {
				assert.Contains(t, string(body), "Hello from Bedrock")
			}
			require.Len(t, v.calls(), 1)
			op := "converse"
			if stream {
				op = "converse-stream"
			}
			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, []string{"/llm/call/big/model/anthropic.claude-3/" + op}, posts)
		})
	}
}

// An ARN or inference-profile model ID round-trips from the /ai/ request
// into the AWS path, escaped as the SDKs escape it.
func TestBedrockBridges_ARNModelID(t *testing.T) {
	for _, stream := range []string{"", `"stream":true,`} {
		h, v := converseHarness(t, false)
		resp, out := h.post("/ai/primary/v1/chat/completions", `{`+stream+`"model":"`+profileARN+`","messages":[{"role":"user","content":"hi"}]}`)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
		calls := v.calls()
		require.Len(t, calls, 1)
		assert.Equal(t, profileARN, ExtractBedrockModelIDFromPath(calls[0].Path), calls[0].Path)
		assert.Contains(t, calls[0].Path, url.PathEscape("inference-profile/us.anthropic.claude-3"))
	}
}
