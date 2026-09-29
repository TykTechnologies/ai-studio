package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// denyBudget refuses every LLM with err, as a spent or unreadable budget does.
type denyBudget struct{ err error }

func (d denyBudget) CheckBudget(*models.App, *models.LLM) (float64, float64, error) {
	return 0, 0, d.err
}
func (denyBudget) AnalyzeBudgetUsage(*models.App, *models.LLM) {}

// asVendor makes the primary LLM a vendor other than OpenAI, with no
// waterfall, so the /ai/ bridge reads the inner hop's refusal through that
// vendor's driver.
func asVendor(v models.Vendor) func(primary, fallback *models.LLM) {
	return func(primary, _ *models.LLM) {
		primary.Vendor = v
		primary.Failover = models.LLMFailover{}
	}
}

func decodeOAIError(t *testing.T, body []byte) *APIError {
	t.Helper()
	var out OAIErrorResponse
	require.NoError(t, json.Unmarshal(body, &out), "body: %s", body)
	require.NotNil(t, out.Error, "body: %s", body)
	return out.Error
}

// A refusal the inner hop decides (here a spent budget) must reach the /ai/
// caller with its reason, whichever vendor's driver read the loopback
// response. The OpenAI driver kept only the envelope's message and wrapped
// it; Google's kept no message at all ("googleapi: Error 403:"), so the
// caller could not tell a spent budget from anything else.
func TestLoopbackRefusal_BudgetReasonReachesTheClient(t *testing.T) {
	for _, vendor := range []models.Vendor{models.OPENAI, models.GOOGLEAI, models.ANTHROPIC} {
		for name, body := range map[string]string{"non-streaming": failoverChatBody, "streaming": failoverStreamBody} {
			t.Run(fmt.Sprintf("%s/%s", vendor, name), func(t *testing.T) {
				h := newFailoverHarness(t, serveOpenAIText("never"), serveOpenAIText("never"), asVendor(vendor))
				h.proxy.budgetService = denyBudget{err: fmt.Errorf("budget exceeded: app monthly budget is 0")}

				resp, got := h.post("/ai/primary/v1/chat/completions", body)
				require.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", got)
				apiErr := decodeOAIError(t, got)
				assert.Equal(t, "Budget limit exceeded: app monthly budget is 0", apiErr.Message)
				assert.Equal(t, "budget_exceeded", apiErr.Code)
				assert.Equal(t, "permission_error", apiErr.Type)
				assert.NotContains(t, string(got), "[ERROR]")
				assert.NotContains(t, string(got), "googleapi")
				assert.NotContains(t, string(got), "unexpected status code")
				assert.Empty(t, h.primaryVendor.calls(), "a refused request never reaches the vendor")
			})
		}
	}
}

// A budget check that could not run is a 503 that says so, without the
// database error behind it.
func TestLoopbackRefusal_BudgetCheckUnavailable(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("never"), serveOpenAIText("never"), asVendor(models.GOOGLEAI))
	h.proxy.budgetService = denyBudget{err: fmt.Errorf("%w: reading app 3: disk I/O error", services.ErrBudgetCheckUnavailable)}

	resp, got := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "body: %s", got)
	apiErr := decodeOAIError(t, got)
	assert.Equal(t, "Budget check unavailable, retry shortly", apiErr.Message)
	assert.Equal(t, "budget_check_unavailable", apiErr.Code)
	assert.NotContains(t, string(got), "disk I/O")
}

// A direct caller of the pass-through keeps the pass-through's error body,
// and a streaming request says what a buffered one does.
func TestBudgetRefusal_PassThroughBodyUnchanged(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("never"), serveOpenAIText("never"), nil)
	h.proxy.budgetService = denyBudget{err: fmt.Errorf("budget exceeded: app monthly budget is 0")}

	for _, path := range []string{"/llm/rest/primary/v1/chat/completions", "/llm/stream/primary/v1/chat/completions"} {
		resp, got := h.post(path, failoverStreamBody)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, "%s body: %s", path, got)
		var out ErrorResponse
		require.NoError(t, json.Unmarshal(got, &out), "body: %s", got)
		assert.Equal(t, "Budget limit exceeded", out.Message, path)
		assert.Equal(t, "budget exceeded: app monthly budget is 0", out.Error, path)
	}
}

func TestParseLoopbackError(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		ok      bool
		message string
		errType string
		code    any
	}{
		{"openai envelope", 400, `{"error":{"message":"too many images","type":"invalid_request_error","code":"too_many_images"}}`, true, "too many images", "invalid_request_error", "too_many_images"},
		{"google envelope drops the numeric code", 400, `{"error":{"code":400,"message":"API key not valid","status":"INVALID_ARGUMENT"}}`, true, "API key not valid", "", nil},
		{"anthropic envelope", 400, `{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens: required"}}`, true, "max_tokens: required", "invalid_request_error", nil},
		{"pass-through client error keeps its reason", 403, `{"status":403,"message":"Budget limit exceeded","error":"budget exceeded: team monthly budget exceeded"}`, true, "Budget limit exceeded: budget exceeded: team monthly budget exceeded", "", nil},
		{"pass-through with no detail", 401, `{"status":401,"message":"invalid credential"}`, true, "invalid credential", "", nil},
		{"pass-through server error hides its internals", 502, `{"status":502,"message":"failed to make upstream request","error":"dial tcp 10.0.0.7:443: connection refused"}`, true, "failed to make upstream request", "", nil},
		{"plain text", 502, `Bad Gateway`, false, "", "", nil},
		{"envelope without a message", 500, `{"error":{"type":"server_error"}}`, false, "", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			apiErr, ok := parseLoopbackError(c.status, []byte(c.body))
			require.Equal(t, c.ok, ok)
			if !ok {
				return
			}
			assert.Equal(t, c.message, apiErr.Message)
			assert.Equal(t, c.errType, apiErr.Type)
			assert.Equal(t, c.code, apiErr.Code)
		})
	}
}

// proxyLogCount waits for the app's proxy logs to stop changing and returns
// how many there are with status.
func proxyLogCount(t *testing.T, h *failoverHarness, status int) int {
	t.Helper()
	waitForProxyLog(t, h.db, h.app.ID, status)
	h.proxy.waitForAnalyzers()
	var n int64
	require.NoError(t, h.db.Model(&models.ProxyLog{}).Where("app_id = ? AND response_code = ?", h.app.ID, status).Count(&n).Error)
	return int(n)
}

// A vendor that cannot be reached leaves one analytics row with the status
// the caller got, on every path. Only /llm/rest recorded one before: the
// streaming pass-through, which /ai/ and the unified /v1 route through,
// answered 502 and recorded nothing, so a vendor outage was invisible.
func TestUpstreamTransportFailure_IsRecorded(t *testing.T) {
	cases := []struct{ path, body string }{
		{"/ai/primary/v1/chat/completions", failoverChatBody},
		{"/ai/primary/v1/chat/completions", failoverStreamBody},
		{"/llm/stream/primary/v1/chat/completions", failoverStreamBody},
		{"/llm/rest/primary/v1/chat/completions", failoverChatBody},
	}
	for _, c := range cases {
		t.Run(c.path+" "+map[bool]string{true: "streaming", false: "buffered"}[c.body == failoverStreamBody], func(t *testing.T) {
			h := newFailoverHarness(t, serveOpenAIText("never"), serveOpenAIText("never"), asVendor(models.OPENAI))
			h.primaryVendor.server.Close() // connection refused

			resp, got := h.post(c.path, c.body)
			require.Equal(t, http.StatusBadGateway, resp.StatusCode, "body: %s", got)
			assert.Equal(t, 1, proxyLogCount(t, h, http.StatusBadGateway))
		})
	}
}

// A vendor that never answers: the /ai/ caller gets 504 from the outer hop's
// deadline, and the row the inner hop leaves says 504 too, not 502.
func TestUpstreamTimeout_OnTheLoopbackIsRecordedAs504(t *testing.T) {
	hang := func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}
	for name, body := range map[string]string{"buffered": failoverChatBody, "streaming": failoverStreamBody} {
		t.Run(name, func(t *testing.T) {
			h := newFailoverHarness(t, hang, serveOpenAIText("never"), asVendor(models.OPENAI))
			h.proxy.config.LLMTimeout = 300 * time.Millisecond

			resp, got := h.post("/ai/primary/v1/chat/completions", body)
			require.Equal(t, http.StatusGatewayTimeout, resp.StatusCode, "body: %s", got)
			assert.Equal(t, 1, proxyLogCount(t, h, http.StatusGatewayTimeout))
			assert.Zero(t, proxyLogCount502(t, h), "the inner hop's row must not say 502")
		})
	}
}

func proxyLogCount502(t *testing.T, h *failoverHarness) int {
	var n int64
	require.NoError(t, h.db.Model(&models.ProxyLog{}).Where("app_id = ? AND response_code = ?", h.app.ID, http.StatusBadGateway).Count(&n).Error)
	return int(n)
}

// Every proxied chat record carries its latency (total_time_ms). It was 0 on
// every row the gateway wrote.
func TestChatRecord_CarriesLatency(t *testing.T) {
	slow := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(40 * time.Millisecond)
		serveOpenAIText("hello")(w, r)
	}
	cases := []struct{ path, body string }{
		{"/ai/primary/v1/chat/completions", failoverChatBody},
		{"/ai/primary/v1/chat/completions", failoverStreamBody},
		{"/llm/rest/primary/v1/chat/completions", failoverChatBody},
		{"/llm/stream/primary/v1/chat/completions", failoverStreamBody},
	}
	for _, c := range cases {
		t.Run(c.path+" "+map[bool]string{true: "streaming", false: "buffered"}[c.body == failoverStreamBody], func(t *testing.T) {
			h := newFailoverHarness(t, slow, serveOpenAIText("never"), nil)
			resp, got := h.post(c.path, c.body)
			require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", got)
			waitForProxyLog(t, h.db, h.app.ID, http.StatusOK)
			h.proxy.waitForAnalyzers()
			// The analytics worker writes the chat record and the proxy log
			// from separate channels in no fixed order: wait for the record.
			waitForAnalytics(t, h.db, 1)

			var recs []models.LLMChatRecord
			require.NoError(t, h.db.Where("app_id = ?", h.app.ID).Find(&recs).Error)
			require.Len(t, recs, 1)
			assert.GreaterOrEqual(t, recs[0].TotalTimeMS, 40, "total_time_ms")
			assert.Less(t, recs[0].TotalTimeMS, 5000, "total_time_ms")
		})
	}
}
