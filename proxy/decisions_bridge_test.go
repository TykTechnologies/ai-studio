package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/decisions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The unified ingress request: OpenAI Decisions schema, "route/model".
func unifiedDecisionBody(t *testing.T, model string) string {
	t.Helper()
	var req map[string]any
	require.NoError(t, json.Unmarshal([]byte(decisionRequestOpenAI), &req))
	req["model"] = model
	b, err := json.Marshal(req)
	require.NoError(t, err)
	return string(b)
}

func decodeDecision(t *testing.T, body []byte) decisions.Response {
	t.Helper()
	var resp decisions.Response
	require.NoError(t, json.Unmarshal(body, &resp), "body: %s", body)
	return resp
}

func TestDecisionBridge_OpenAIRouteForwardsTheBodyAsSent(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveOpenAIDecision(), serveStatus(500), noFailover)

	body := unifiedDecisionBody(t, "primary/gpt-6-luna")
	// An unknown field the gateway does not model must still reach OpenAI.
	body = body[:len(body)-1] + `,"metadata":{"ticket":"T-1"}}`
	resp, out := h.post("/v1/decisions", body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	assert.Equal(t, "primary", resp.Header.Get(hdrServedLLM))

	calls := h.primaryVendor.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "/v1/decisions", calls[0].Path)
	assert.Equal(t, "Bearer primary-vendor-key", calls[0].Headers.Get("Authorization"))
	var sent map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(calls[0].Body, &sent))
	assert.JSONEq(t, `"gpt-6-luna"`, string(sent["model"]), "route prefix stripped")
	assert.JSONEq(t, `{"ticket":"T-1"}`, string(sent["metadata"]))

	dec := decodeDecision(t, out)
	assert.Equal(t, "dec_1", dec.ID)
	require.Len(t, dec.Answers, 3)

	waitForAnalytics(t, h.db, 1)
	h.proxy.waitForAnalyzers()
	recs := h.chatRecords()
	require.Len(t, recs, 1, "governance runs once, on the loopback hop")
	assert.Equal(t, models.DecisionRequest, recs[0].RequestKind)
	assert.Equal(t, 3, recs[0].DecisionQuestions)
}

func TestDecisionBridge_SystemOneRouteIsTranslatedBothWays(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), asSystemOne)
	h.addPrice("jev-1.13.0", models.SYSTEMONE, 0.000000042)

	resp, out := h.post("/v1/decisions", unifiedDecisionBody(t, "primary/jev-latest"))
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

	calls := h.primaryVendor.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "/v1/systemone", calls[0].Path)
	assert.JSONEq(t, `{"model":"jev-latest","state":"I was charged twice for my order.",
	  "questions":{"angry":{"type":"noul","instructions":"Is the customer angry?"},
	    "topic":{"type":"choice","instructions":"Which team?","criteria":{"shipping":"shipping","billing":"billing"}},
	    "urgency":{"type":"score","instructions":"How urgent?","criteria":["low","high"]}}}`, string(calls[0].Body))

	assert.JSONEq(t, `{"object":"decision","model":"jev-1.13.0","answers":[
	  {"type":"predicate","name":"angry","probability":0.8},
	  {"type":"choice","name":"topic","choice":"billing","confidence":0.9,
	   "probabilities":[{"value":"shipping","probability":0.1},{"value":"billing","probability":0.9}]},
	  {"type":"refusal","name":"urgency"}],
	  "usage":{"input_tokens":200,"output_tokens":0}}`, string(out))

	waitForAnalytics(t, h.db, 1)
	h.proxy.waitForAnalyzers()
	recs := h.chatRecords()
	require.Len(t, recs, 1)
	assert.Equal(t, models.DecisionRequest, recs[0].RequestKind)
	assert.Equal(t, string(models.SYSTEMONE), recs[0].Vendor)
	assert.Equal(t, 3, recs[0].DecisionQuestions)
	assert.Equal(t, 1, recs[0].DecisionRefusals)
	assert.InDelta(t, 200*0.000000042*10000, recs[0].Cost, 1e-9)
}

func TestDecisionBridge_RejectsBeforeTheUpstream(t *testing.T) {
	t.Parallel()
	images := `{"model":"primary/jev-latest","input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]}],` +
		`"questions":[{"type":"predicate","name":"q","instructions":"?"}]}`
	cases := []struct {
		name   string
		mutate func(primary, fallback *models.LLM)
		body   string
		status int
		param  string
	}{
		{"stream", asSystemOne, `{"model":"primary/jev-latest","stream":true,"input":"x","questions":[{"type":"predicate","name":"q"}]}`, http.StatusBadRequest, "stream"},
		{"duplicate names", asSystemOne, `{"model":"primary/jev-latest","input":"x","questions":[{"type":"predicate","name":"q"},{"type":"predicate","name":"q"}]}`, http.StatusBadRequest, "questions[1].name"},
		{"images on a text-only model", asSystemOne, images, http.StatusBadRequest, "input"},
		{"vendor without decision models", asVendor(models.ANTHROPIC), unifiedDecisionBody(t, "primary/claude"), http.StatusBadRequest, "model"},
		{"model not allowed", func(p, f *models.LLM) { asSystemOne(p, f); p.AllowedModels = []string{"jev-1.13.0"} }, unifiedDecisionBody(t, "primary/jev-latest"), http.StatusForbidden, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), tc.mutate)
			resp, out := h.post("/v1/decisions", tc.body)
			require.Equal(t, tc.status, resp.StatusCode, string(out))
			apiErr := decodeOAIError(t, out)
			if tc.param != "" {
				require.NotNil(t, apiErr.Param, string(out))
				assert.Equal(t, tc.param, *apiErr.Param)
				assert.Equal(t, "invalid_request_error", apiErr.Type)
			}
			assert.Empty(t, h.primaryVendor.calls())
		})
	}
}

func TestDecisionBridge_Unauthenticated(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), asSystemOne)
	h.apiKey = "wrong"
	resp, _ := h.post("/v1/decisions", unifiedDecisionBody(t, "primary/jev-latest"))
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Empty(t, h.primaryVendor.calls())
}

func TestDecisionBridge_UpstreamErrorsUseTheOpenAIContract(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		upstream int
		body     string
		status   int
		errType  string
		message  string
	}{
		{"validation", http.StatusUnprocessableEntity, `{"detail":"questions.topic.criteria: at least 2 options"}`, http.StatusBadRequest, "invalid_request_error", "questions.topic.criteria: at least 2 options"},
		{"overloaded", statusSiteOverloaded, `{"detail":"overloaded"}`, http.StatusServiceUnavailable, "api_error", "overloaded"},
		{"rate limited", http.StatusTooManyRequests, `{"detail":"slow down"}`, http.StatusTooManyRequests, "rate_limit_error", "slow down"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			serve := func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.upstream)
				_, _ = w.Write([]byte(tc.body))
			}
			h := newFailoverHarness(t, serve, serveStatus(500), asSystemOne)
			resp, out := h.post("/v1/decisions", unifiedDecisionBody(t, "primary/jev-latest"))
			require.Equal(t, tc.status, resp.StatusCode, string(out))
			apiErr := decodeOAIError(t, out)
			assert.Equal(t, tc.errType, apiErr.Type)
			assert.Contains(t, apiErr.Message, tc.message)
		})
	}
}

func TestDecisionBridge_FailsOverAcrossDecisionVendors(t *testing.T) {
	t.Parallel()
	// A System One primary that is down falls over to an OpenAI fallback; the
	// body is re-shaped per rung.
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIDecision(), func(primary, fallback *models.LLM) {
		primary.Vendor = models.SYSTEMONE
		primary.Failover = models.LLMFailover{Targets: []models.LLMFailoverTarget{{LLMID: fallback.ID, Model: "gpt-6-luna"}}}
	})
	resp, out := h.post("/v1/decisions", unifiedDecisionBody(t, "primary/jev-latest"))
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	assert.Equal(t, "fallback", resp.Header.Get(hdrServedLLM))
	assert.Equal(t, "gpt-6-luna", resp.Header.Get(hdrServedModel))

	require.Len(t, h.primaryVendor.calls(), 1)
	assert.Equal(t, "/v1/systemone", h.primaryVendor.calls()[0].Path)
	fb := h.fallbackVendor.calls()
	require.Len(t, fb, 1)
	assert.Equal(t, "/v1/decisions", fb[0].Path)
	var sent decisions.Request
	require.NoError(t, json.Unmarshal(fb[0].Body, &sent))
	assert.Equal(t, "gpt-6-luna", sent.Model)
	assert.Len(t, sent.Questions, 3)
}

func TestDecisionBridge_SkipsFailoverRungsWithoutDecisionModels(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("chat"), func(primary, fallback *models.LLM) {
		primary.Vendor = models.SYSTEMONE
		fallback.Vendor = models.ANTHROPIC
		primary.Failover = models.LLMFailover{Targets: []models.LLMFailoverTarget{{LLMID: fallback.ID, Model: "claude"}}}
	})
	resp, out := h.post("/v1/decisions", unifiedDecisionBody(t, "primary/jev-latest"))
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "the primary's own failure, not the skipped rung's: %s", out)
	assert.Empty(t, h.fallbackVendor.calls(), "a chat-only rung is never called with a decision request")
	assert.Len(t, h.primaryVendor.calls(), 1)
}

// closedURL is an address nothing listens on.
func closedURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	return url
}

func TestDecisionBridge_RequestErrors(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), asSystemOne)

	cases := []struct {
		name   string
		path   string
		body   string
		status int
		param  string
	}{
		{"not JSON", "/v1/decisions", `{"model":`, http.StatusBadRequest, ""},
		{"model not vendor/model", "/v1/decisions", `{"model":"jev-latest","input":"x","questions":[{"type":"predicate","name":"q"}]}`, http.StatusBadRequest, ""},
		{"model not a string", "/v1/decisions", `{"model":7}`, http.StatusBadRequest, ""},
		{"empty model on the route", "/ai/primary/v1/decisions", `{"model":"","input":"x","questions":[{"type":"predicate","name":"q"}]}`, http.StatusBadRequest, "model"},
		{"no input", "/ai/primary/v1/decisions", `{"model":"jev-latest","questions":[{"type":"predicate","name":"q"}]}`, http.StatusBadRequest, "input"},
		{"no questions", "/ai/primary/v1/decisions", `{"model":"jev-latest","input":"x","questions":[]}`, http.StatusBadRequest, "questions"},
		{"unknown question type", "/ai/primary/v1/decisions", `{"model":"jev-latest","input":"x","questions":[{"type":"noul","name":"q"}]}`, http.StatusBadRequest, "questions[0].type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, out := h.post(tc.path, tc.body)
			require.Equal(t, tc.status, resp.StatusCode, string(out))
			apiErr := decodeOAIError(t, out)
			assert.NotEmpty(t, apiErr.Message)
			if tc.param != "" {
				require.NotNil(t, apiErr.Param, string(out))
				assert.Equal(t, tc.param, *apiErr.Param)
			}
		})
	}
	assert.Empty(t, h.primaryVendor.calls(), "no request error reaches the vendor")

	// An App naming a route it was not granted (including one that does not
	// exist) is refused by the credential check with 403, exactly as on the
	// chat ingress (docs/site/docs/proxy.md).
	t.Run("ungranted route matches the chat ingress", func(t *testing.T) {
		dec, decBody := h.post("/v1/decisions", `{"model":"nope/jev-latest","input":"x","questions":[{"type":"predicate","name":"q"}]}`)
		chat, chatBody := h.post("/v1/chat/completions", `{"model":"nope/gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
		assert.Equal(t, http.StatusForbidden, dec.StatusCode, string(decBody))
		assert.Equal(t, chat.StatusCode, dec.StatusCode, "chat: %s", chatBody)
	})

	t.Run("oversized body", func(t *testing.T) {
		big := `{"model":"jev-latest","input":"` + strings.Repeat("x", maxUnifiedRouterBodyBytes+1) + `","questions":[{"type":"predicate","name":"q"}]}`
		resp, out := h.post("/ai/primary/v1/decisions", big)
		assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode, bodyExcerptForTest(out))
	})

	t.Run("anonymous callers cannot probe for routes", func(t *testing.T) {
		h.apiKey = "wrong"
		defer func() { h.apiKey = h.app.Credential.Secret }()
		resp, _ := h.post("/v1/decisions", `{"model":"nope/x","input":"x","questions":[{"type":"predicate","name":"q"}]}`)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "401 before route resolution, never 404")
	})

	t.Run("GET is not a decision call", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(h.port)+"/v1/decisions", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.GreaterOrEqual(t, resp.StatusCode, 400)
		assert.Empty(t, h.primaryVendor.calls())
	})
}

func bodyExcerptForTest(b []byte) string {
	if len(b) > 300 {
		return string(b[:300]) + "..."
	}
	return string(b)
}

func TestDecisionBridge_UpstreamFailures(t *testing.T) {
	t.Parallel()
	t.Run("a System One 200 without answers is a 502", func(t *testing.T) {
		serve := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"jev-1.13.0","usage":{"input_tokens":5}}`))
		}
		h := newFailoverHarness(t, serve, serveStatus(500), asSystemOne)
		resp, out := h.post("/v1/decisions", unifiedDecisionBody(t, "primary/jev-latest"))
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, string(out))
		decodeOAIError(t, out)
	})

	t.Run("a non-JSON upstream error keeps its status in an envelope", func(t *testing.T) {
		serve := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("<html>internal error</html>"))
		}
		h := newFailoverHarness(t, serve, serveStatus(500), asSystemOne)
		resp, out := h.post("/v1/decisions", unifiedDecisionBody(t, "primary/jev-latest"))
		require.Equal(t, http.StatusInternalServerError, resp.StatusCode, string(out))
		decodeOAIError(t, out)
	})

	t.Run("an unreachable primary fails over", func(t *testing.T) {
		h := newFailoverHarness(t, serveStatus(500), serveSystemOneDecision(), func(primary, fallback *models.LLM) {
			primary.Vendor = models.SYSTEMONE
			primary.APIEndpoint = closedURL(t)
			fallback.Vendor = models.SYSTEMONE
			primary.Failover = models.LLMFailover{Targets: []models.LLMFailoverTarget{{LLMID: fallback.ID, Model: "jev-latest"}}}
		})
		resp, out := h.post("/v1/decisions", unifiedDecisionBody(t, "primary/jev-latest"))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
		assert.Equal(t, "fallback", resp.Header.Get(hdrServedLLM))
		require.Len(t, h.fallbackVendor.calls(), 1)
	})

	t.Run("an unreachable primary without a waterfall is a 502 envelope", func(t *testing.T) {
		h := newFailoverHarness(t, serveStatus(500), serveStatus(500), func(primary, fallback *models.LLM) {
			asSystemOne(primary, fallback)
			primary.APIEndpoint = closedURL(t)
		})
		resp, out := h.post("/v1/decisions", unifiedDecisionBody(t, "primary/jev-latest"))
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, string(out))
		decodeOAIError(t, out)
	})

	t.Run("a fallback of another format that cannot take the request is skipped", func(t *testing.T) {
		// OpenAI primary takes images; the System One fallback cannot, so the
		// waterfall ends at the primary's own failure.
		h := newFailoverHarness(t, serveStatus(503), serveSystemOneDecision(), func(primary, fallback *models.LLM) {
			fallback.Vendor = models.SYSTEMONE
			primary.Failover = models.LLMFailover{Targets: []models.LLMFailoverTarget{{LLMID: fallback.ID, Model: "jev-latest"}}}
		})
		body := `{"model":"primary/gpt-6-luna","input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]}],` +
			`"questions":[{"type":"predicate","name":"q","instructions":"?"}]}`
		resp, out := h.post("/v1/decisions", body)
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, string(out))
		assert.Empty(t, h.fallbackVendor.calls())
	})
}

// The reverse of the decision waterfall: a chat request whose waterfall has a
// decision-only fallback must skip it, and the caller must see the primary's
// own failure rather than "vendor not supported on /ai", which named a
// workaround (/llm/rest) that cannot serve chat either.
func TestChatBridge_SkipsDecisionOnlyFailoverRungs(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{"buffered": failoverChatBody, "streaming": failoverStreamBody} {
		t.Run(name, func(t *testing.T) {
			h := newFailoverHarness(t, serveStatus(503), serveSystemOneDecision(), func(primary, fallback *models.LLM) {
				fallback.Vendor = models.SYSTEMONE
				primary.Failover = models.LLMFailover{Targets: []models.LLMFailoverTarget{{LLMID: fallback.ID, Model: "jev-latest"}}}
			})
			resp, out := h.post("/ai/primary/v1/chat/completions", body)
			assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "the primary's own failure: %s", out)
			assert.NotContains(t, string(out), "not supported on the /ai")
			assert.Empty(t, h.fallbackVendor.calls(), "a decision-only rung is never sent a chat request")
			assert.Len(t, h.primaryVendor.calls(), 1)
		})
	}
}

func TestChatBridge_RejectsDecisionOnlyRoutes(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), asSystemOne)
	for _, body := range []string{
		`{"model":"primary/jev-latest","messages":[{"role":"user","content":"hi"}]}`,
		`{"model":"primary/jev-latest","stream":true,"messages":[{"role":"user","content":"hi"}]}`,
	} {
		resp, out := h.post("/v1/chat/completions", body)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
		assert.Contains(t, decodeOAIError(t, out).Message, "serves decision models only")
	}
	assert.Empty(t, h.primaryVendor.calls())
}

// Decisions are budgeted like chat: a spent budget refuses the call on the
// loopback hop, the vendor is never called, and the reason reaches the
// caller in the OpenAI envelope.
func TestDecisionBridge_BudgetRefusal(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		mutate func(primary, fallback *models.LLM)
		model  string
	}{
		"systemone": {asSystemOne, "primary/jev-latest"},
		"openai":    {noFailover, "primary/gpt-6-luna"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), tc.mutate)
			h.proxy.budgetService = denyBudget{err: fmt.Errorf("budget exceeded: app monthly budget is 0")}

			resp, out := h.post("/v1/decisions", unifiedDecisionBody(t, tc.model))
			require.Equal(t, http.StatusForbidden, resp.StatusCode, string(out))
			apiErr := decodeOAIError(t, out)
			assert.Equal(t, "Budget limit exceeded: app monthly budget is 0", apiErr.Message)
			assert.Equal(t, "budget_exceeded", apiErr.Code)
			assert.Empty(t, h.primaryVendor.calls())
		})
	}
}

func TestUnifiedListModels_ReportsCapabilities(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), asSystemOne)
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(h.port)+"/v1/models", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)

	var list unifiedModelList
	require.NoError(t, json.Unmarshal(body, &list))
	require.Len(t, list.Data, 1, string(body))
	assert.Equal(t, "primary/jev-latest", list.Data[0].ID)
	assert.Equal(t, []string{"decisions"}, list.Data[0].Capabilities)
}
