package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
	"gorm.io/gorm"
)

// --- unit: classification -------------------------------------------------

func TestShouldFailover(t *testing.T) {
	defaults := models.LLMFailover{}.EffectiveTriggers()
	off := false
	noTimeout := models.LLMFailover{Triggers: &models.LLMFailoverTriggers{OnTimeout: &off}}.EffectiveTriggers()
	noConn := models.LLMFailover{Triggers: &models.LLMFailoverTriggers{OnConnectionError: &off}}.EffectiveTriggers()
	only503 := models.LLMFailover{Triggers: &models.LLMFailoverTriggers{StatusCodes: []int{503}}}.EffectiveTriggers()

	withStatus := func(code int) attemptFailure {
		return attemptFailure{err: fmt.Errorf("API returned unexpected status code: %d: nope", code), status: code, hasStatus: true}
	}
	noStatus := func(err error) attemptFailure {
		return attemptFailure{err: err, status: http.StatusBadGateway}
	}

	cases := []struct {
		name   string
		fail   attemptFailure
		trig   models.ResolvedFailoverTriggers
		want   bool
		reason string
	}{
		{"503 in defaults", withStatus(503), defaults, true, "status_503"},
		{"502 in defaults", withStatus(502), defaults, true, "status_502"},
		{"429 in defaults", withStatus(429), defaults, true, "status_429"},
		{"408 in defaults", withStatus(408), defaults, true, "status_408"},
		{"500 when only 503 configured", withStatus(500), only503, false, ""},
		{"400 never", withStatus(400), defaults, false, ""},
		{"401 never", withStatus(401), defaults, false, ""},
		{"403 never", withStatus(403), defaults, false, ""},
		{"404 never", withStatus(404), defaults, false, ""},
		{"422 never", withStatus(422), defaults, false, ""},
		{"deadline exceeded", noStatus(context.DeadlineExceeded), defaults, true, "timeout"},
		{"deadline exceeded with timeouts off", noStatus(context.DeadlineExceeded), noTimeout, false, "timeout"},
		{"typed timeout", noStatus(llms.NewError(llms.ErrCodeTimeout, "openai", "slow")), defaults, true, "timeout"},
		{"connection refused", noStatus(&url.Error{Op: "Post", URL: "http://x", Err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}}), defaults, true, "connection_error"},
		{"connection refused with connection errors off", noStatus(&url.Error{Op: "Post", URL: "http://x", Err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}}), noConn, false, "connection_error"},
		{"url timeout is a timeout, not a connection error", noStatus(&url.Error{Op: "Post", URL: "http://x", Err: timeoutErr{}}), noConn, true, "timeout"},
		{"unexpected EOF", noStatus(io.ErrUnexpectedEOF), defaults, true, "connection_error"},
		{"caller cancelled", noStatus(context.Canceled), defaults, false, ""},
		{"opaque error", noStatus(errors.New("something odd")), defaults, false, ""},
		{"nil error", attemptFailure{}, defaults, false, ""},
		{"attempt deadline with a sanitised driver error", attemptFailure{err: errors.New("request timeout: API call exceeded deadline"), status: 504, timedOut: true}, defaults, true, "timeout"},
		{"attempt deadline with timeouts off", attemptFailure{err: errors.New("request timeout"), status: 504, timedOut: true}, noTimeout, false, "timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := shouldFailover(tc.fail, tc.trig)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.reason, reason)
		})
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

// --- unit: planning --------------------------------------------------------

func TestPlanFailover_SkipsUnusableRungs(t *testing.T) {
	primary := &models.LLM{ID: 1, Name: "Primary", Vendor: models.OPENAI, Active: true}
	fallback := &models.LLM{ID: 2, Name: "Fallback", Vendor: models.OPENAI, Active: true, AllowedModels: []string{"^gpt-4o$"}}
	bedrock := &models.LLM{ID: 3, Name: "Bedrock", Vendor: models.BEDROCK, Active: true}
	primary.Failover = models.LLMFailover{Targets: []models.LLMFailoverTarget{
		{LLMID: 2, Model: "gpt-3.5-turbo"}, // no longer allowed by the target
		{LLMID: 1, Model: "gpt-4o"},        // self
		{LLMID: 9, Model: "gpt-4o"},        // not loaded
		{LLMID: 3, Model: "anthropic.x"},   // bedrock is a rung like any other
		{LLMID: 2, Model: "gpt-4o"},        // usable
	}}
	p := &Proxy{
		llms:     map[string]*models.LLM{"primary": primary, "fallback": fallback, "bedrock": bedrock},
		llmsByID: map[uint]*models.LLM{1: primary, 2: fallback, 3: bedrock},
		config:   &Config{},
	}
	r := httptest.NewRequest(http.MethodPost, "/ai/primary/v1/chat/completions", nil)

	plan := p.planFailover(primary, "gpt-4o", r)
	require.Len(t, plan.attempts, 3)
	assert.Equal(t, "primary", plan.attempts[0].slug)
	assert.Equal(t, "gpt-4o", plan.attempts[0].model)
	assert.Nil(t, plan.attempts[0].origin)
	assert.Equal(t, "bedrock", plan.attempts[1].slug)
	assert.Equal(t, "anthropic.x", plan.attempts[1].model)
	assert.Equal(t, "fallback", plan.attempts[2].slug)
	assert.Equal(t, 2, plan.attempts[2].index)
	assert.Same(t, primary, plan.attempts[2].origin)
	assert.Equal(t, 5*time.Minute, plan.attemptTimeout)

	plain := &models.LLM{ID: 4, Name: "Plain", Vendor: models.OPENAI}
	assert.Len(t, p.planFailover(plain, "x", r).attempts, 1, "no waterfall is exactly one attempt")
}

func TestFailoverMarker_RejectedWithoutTheProcessToken(t *testing.T) {
	primary := &models.LLM{ID: 1, Name: "Primary", Failover: models.LLMFailover{Targets: []models.LLMFailoverTarget{{LLMID: 2, Model: "m"}}}}
	fallback := &models.LLM{ID: 2, Name: "Fallback"}
	p := &Proxy{
		llms:          map[string]*models.LLM{"primary": primary, "fallback": fallback},
		llmsByID:      map[uint]*models.LLM{1: primary, 2: fallback},
		failoverToken: newFailoverToken(),
	}
	app := &models.App{LLMs: []models.LLM{*primary}}

	mk := func(token string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/llm/call/fallback/v1/chat/completions", nil)
		r.Header.Set(hdrFailoverOrigin, "primary")
		r.Header.Set(hdrFailoverAttempt, "1")
		if token != "" {
			r.Header.Set(hdrFailoverToken, token)
		}
		return r
	}

	_, ok := p.parseFailoverMarker(mk(""))
	assert.False(t, ok)
	assert.False(t, p.failoverGrantsAccess(mk(""), app, fallback))

	_, ok = p.parseFailoverMarker(mk("not-the-token"))
	assert.False(t, ok)
	assert.False(t, p.failoverGrantsAccess(mk("not-the-token"), app, fallback))

	m, ok := p.parseFailoverMarker(mk(p.failoverToken))
	require.True(t, ok)
	assert.Equal(t, failoverMarker{FromLLMID: 1, Attempt: 1}, m)
	assert.True(t, p.failoverGrantsAccess(mk(p.failoverToken), app, fallback))

	// A valid token still does not help an app that was never granted the primary.
	assert.False(t, p.failoverGrantsAccess(mk(p.failoverToken), &models.App{}, fallback))
	// ...nor a target the primary's waterfall does not list.
	assert.False(t, p.failoverGrantsAccess(mk(p.failoverToken), app, &models.LLM{ID: 3}))
}

// --- two-hop harness -------------------------------------------------------

type vendorCall struct {
	Path    string
	Headers http.Header
	Body    []byte
}

type fakeVendor struct {
	mu       sync.Mutex
	requests []vendorCall
	server   *httptest.Server
}

func newFakeVendor(t *testing.T, serve http.HandlerFunc) *fakeVendor {
	v := &fakeVendor{}
	v.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		v.mu.Lock()
		v.requests = append(v.requests, vendorCall{Path: r.URL.Path, Headers: r.Header.Clone(), Body: body})
		v.mu.Unlock()
		serve(w, r)
	}))
	t.Cleanup(v.server.Close)
	return v
}

func (v *fakeVendor) calls() []vendorCall {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]vendorCall(nil), v.requests...)
}

// failoverHarness is one live proxy on a real port, two OpenAI-shaped fake
// vendors, a primary LLM whose waterfall points at a fallback LLM, and an app
// that was granted the PRIMARY ONLY -- so every fallback that serves proves
// inherited access works.
type failoverHarness struct {
	t                 *testing.T
	db                *gorm.DB
	port              int
	apiKey            string
	app               *models.App
	primary, fallback *models.LLM
	primaryVendor     *fakeVendor
	fallbackVendor    *fakeVendor
}

func newFailoverHarness(t *testing.T, servePrimary, serveFallback http.HandlerFunc, mutate func(primary, fallback *models.LLM)) *failoverHarness {
	t.Helper()
	db, cancel := setupTest(t)
	t.Cleanup(func() { tearDownTest(db, cancel) })

	h := &failoverHarness{t: t, db: db}
	h.primaryVendor = newFakeVendor(t, servePrimary)
	h.fallbackVendor = newFakeVendor(t, serveFallback)

	service := services.NewService(db)
	budgetSvc := budget.NewService(db, services.NewTestNotificationService(db))

	user, err := service.CreateUser(services.UserDTO{Email: "failover@example.com", Name: "failover", Password: "password123"})
	require.NoError(t, err)

	h.fallback = &models.LLM{
		Name: "Fallback", Vendor: models.OPENAI, Active: true,
		APIEndpoint: h.fallbackVendor.server.URL, APIKey: "fallback-vendor-key",
		DefaultModel: "gpt-4o",
	}
	require.NoError(t, db.Create(h.fallback).Error)

	h.primary = &models.LLM{
		Name: "Primary", Vendor: models.OPENAI, Active: true,
		APIEndpoint: h.primaryVendor.server.URL, APIKey: "primary-vendor-key",
		DefaultModel: "gpt-4o",
		Failover:     models.LLMFailover{Targets: []models.LLMFailoverTarget{{LLMID: h.fallback.ID, Model: "gpt-4o"}}},
	}
	if mutate != nil {
		mutate(h.primary, h.fallback)
		require.NoError(t, db.Save(h.fallback).Error)
	}
	require.NoError(t, db.Create(h.primary).Error)

	app, err := service.CreateApp("Failover App", "", user.ID, nil, []uint{h.primary.ID}, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, service.ActivateAppCredential(app.ID))
	app, err = service.GetAppByID(app.ID)
	require.NoError(t, err)
	require.NotNil(t, app.Credential)
	h.app = app
	h.apiKey = app.Credential.Secret

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	h.port = ln.Addr().(*net.TCPAddr).Port

	p := NewProxy(service, &Config{Port: h.port}, budgetSvc)
	require.NoError(t, p.loadResources())
	srv := &http.Server{Handler: p.createHandler()}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		p.waitForAnalyzers()
	})
	return h
}

func (h *failoverHarness) post(path, body string, headers ...string) (*http.Response, []byte) {
	h.t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(h.port)+path, bytes.NewBufferString(body))
	require.NoError(h.t, err)
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(h.t, err)
	return resp, respBody
}

func (h *failoverHarness) proxyLogs() []models.ProxyLog {
	var logs []models.ProxyLog
	require.NoError(h.t, h.db.Where("app_id = ?", h.app.ID).Order("id asc").Find(&logs).Error)
	return logs
}

const failoverChatBody = `{"model":"gpt-4o","messages":[{"role":"user","content":"Say this is a test!"}]}`
const failoverStreamBody = `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"Say this is a test!"}]}`

func serveStatus(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(`{"error":{"message":"vendor says no","type":"server_error"}}`))
	}
}

func serveOpenAIText(text string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream bool `json:"stream"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		if !req.Stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-4o",` +
				`"choices":[{"index":0,"message":{"role":"assistant","content":"` + text + `"},"finish_reason":"stop"}],` +
				`"usage":{"prompt_tokens":6,"completion_tokens":5,"total_tokens":11}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, piece := range []string{text[:len(text)/2], text[len(text)/2:]} {
			fmt.Fprintf(w, "data: %s\n\n", openAIChunk(piece, nil))
			flusher.Flush()
		}
		stop := "stop"
		fmt.Fprintf(w, "data: %s\n\n", openAIChunk("", &stop))
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
}

func openAIChunk(content string, finish *string) string {
	chunk := map[string]interface{}{
		"id": "chatcmpl-1", "object": "chat.completion.chunk", "created": 1, "model": "gpt-4o",
		"choices": []map[string]interface{}{{
			"index": 0, "delta": map[string]string{"content": content}, "finish_reason": finish,
		}},
	}
	b, _ := json.Marshal(chunk)
	return string(b)
}

func streamedText(t *testing.T, body []byte) (string, []ChatCompletionChunk) {
	t.Helper()
	var text strings.Builder
	var chunks []ChatCompletionChunk
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "data: ") || strings.HasSuffix(line, "[DONE]") {
			continue
		}
		var chunk ChatCompletionChunk
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk), "frame: %s", line)
		chunks = append(chunks, chunk)
		for _, c := range chunk.Choices {
			text.WriteString(c.Delta.Content)
		}
	}
	return text.String(), chunks
}

// --- two-hop cases ---------------------------------------------------------

func TestFailover_PrimaryFailsFallbackServes(t *testing.T) {
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("from the fallback"), nil)

	resp, body := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	var out ChatCompletionResponse
	require.NoError(t, json.Unmarshal(body, &out))
	require.Len(t, out.Choices, 1)
	assert.Equal(t, "from the fallback", out.Choices[0].Message.Content)
	assert.Equal(t, "gpt-4o", out.Model, "the served model is echoed")

	assert.Equal(t, "true", resp.Header.Get(hdrFailover))
	assert.Equal(t, "fallback", resp.Header.Get(hdrServedLLM))
	assert.Equal(t, "gpt-4o", resp.Header.Get(hdrServedModel))

	require.Len(t, h.primaryVendor.calls(), 1)
	fallbackCalls := h.fallbackVendor.calls()
	require.Len(t, fallbackCalls, 1)

	// The vendor never sees the loopback marker, and gets the fallback's own key.
	for _, hdr := range []string{hdrFailoverOrigin, hdrFailoverAttempt, hdrFailoverToken} {
		assert.Empty(t, fallbackCalls[0].Headers.Get(hdr), "leaked %s", hdr)
	}
	assert.Equal(t, "Bearer fallback-vendor-key", fallbackCalls[0].Headers.Get("Authorization"))

	// One ProxyLog per attempt: the failed primary and the fallback that served.
	waitForProxyLog(t, h.db, h.app.ID, http.StatusServiceUnavailable)
	waitForProxyLog(t, h.db, h.app.ID, http.StatusOK)
	logs := h.proxyLogs()
	require.Len(t, logs, 2)
	assert.Equal(t, h.primary.ID, logs[0].LLMID)
	assert.Equal(t, http.StatusServiceUnavailable, logs[0].ResponseCode)
	assert.Nil(t, logs[0].FailoverFromLLMID)
	assert.Equal(t, 0, logs[0].FailoverAttempt)
	assert.Equal(t, h.fallback.ID, logs[1].LLMID)
	assert.Equal(t, http.StatusOK, logs[1].ResponseCode)
	require.NotNil(t, logs[1].FailoverFromLLMID)
	assert.Equal(t, h.primary.ID, *logs[1].FailoverFromLLMID)
	assert.Equal(t, 1, logs[1].FailoverAttempt)
}

func TestFailover_CallerErrorsDoNotFailOver(t *testing.T) {
	for _, code := range []int{400, 401, 403, 404} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			h := newFailoverHarness(t, serveStatus(code), serveOpenAIText("never"), nil)

			resp, body := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
			assert.Equal(t, code, resp.StatusCode, "body: %s", body)
			var envelope struct {
				Error struct {
					Type string `json:"type"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(body, &envelope))
			assert.NotEmpty(t, envelope.Error.Type, "OpenAI error envelope")
			assert.Empty(t, resp.Header.Get(hdrFailover))
			assert.Empty(t, resp.Header.Get(hdrServedLLM))
			assert.Empty(t, h.fallbackVendor.calls(), "fallback must not be tried")
		})
	}
}

func TestFailover_EveryRungFailsReturnsTheLastError(t *testing.T) {
	h := newFailoverHarness(t, serveStatus(503), serveStatus(502), nil)

	resp, body := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode, "body: %s", body)
	assert.Empty(t, resp.Header.Get(hdrFailover))
	assert.Len(t, h.primaryVendor.calls(), 1)
	assert.Len(t, h.fallbackVendor.calls(), 1)
}

func TestFailover_StreamingBeforeFirstFrame(t *testing.T) {
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("streamed by the fallback"), nil)

	resp, body := h.post("/ai/primary/v1/chat/completions", failoverStreamBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
	assert.Equal(t, "true", resp.Header.Get(hdrFailover))
	assert.Equal(t, "fallback", resp.Header.Get(hdrServedLLM))

	text, chunks := streamedText(t, body)
	assert.Equal(t, "streamed by the fallback", text)
	require.NotEmpty(t, chunks)
	for _, c := range chunks {
		assert.Equal(t, "gpt-4o", c.Model)
	}
	assert.Len(t, h.fallbackVendor.calls(), 1)
}

func TestFailover_StreamingAfterFirstFrameIsCommitted(t *testing.T) {
	// The primary gets one frame out, then drops the connection.
	primary := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", openAIChunk("partial ", nil))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}
	h := newFailoverHarness(t, primary, serveOpenAIText("never"), nil)

	resp, body := h.post("/ai/primary/v1/chat/completions", failoverStreamBody)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), `"content":"partial "`, "the committed frame reached the client")
	assert.Contains(t, string(body), "[DONE]")
	assert.Empty(t, resp.Header.Get(hdrFailover))
	assert.Equal(t, "primary", resp.Header.Get(hdrServedLLM))
	assert.Empty(t, h.fallbackVendor.calls(), "a committed stream cannot fail over")
}

func TestFailover_RungSkippedWhenTargetNoLongerAllowsTheModel(t *testing.T) {
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("never"), func(primary, fallback *models.LLM) {
		fallback.AllowedModels = []string{"^gpt-3.5-turbo$"}
	})

	resp, _ := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Empty(t, h.fallbackVendor.calls())
}

func TestFailover_RungSkippedWhenTargetInactive(t *testing.T) {
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("never"), func(primary, fallback *models.LLM) {
		fallback.Active = false
	})

	resp, _ := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Empty(t, h.fallbackVendor.calls())
}

// stubBudget rejects one LLM and allows everything else. The community budget
// service never rejects, so the budget pre-check is exercised at unit level.
type stubBudget struct{ deny uint }

func (s stubBudget) CheckBudget(app *models.App, llm *models.LLM) (float64, float64, error) {
	if llm.ID == s.deny {
		return 0, 0, errors.New("monthly budget exceeded")
	}
	return 0, 0, nil
}
func (stubBudget) AnalyzeBudgetUsage(*models.App, *models.LLM) {}

func TestPlanFailover_RungSkippedWhenFallbackBudgetExhausted(t *testing.T) {
	primary := &models.LLM{ID: 1, Name: "Primary", Vendor: models.OPENAI, Active: true}
	broke := &models.LLM{ID: 2, Name: "Broke", Vendor: models.OPENAI, Active: true}
	solvent := &models.LLM{ID: 3, Name: "Solvent", Vendor: models.OPENAI, Active: true}
	primary.Failover = models.LLMFailover{Targets: []models.LLMFailoverTarget{
		{LLMID: 2, Model: "gpt-4o"}, {LLMID: 3, Model: "gpt-4o"},
	}}
	p := &Proxy{
		llms:          map[string]*models.LLM{"primary": primary, "broke": broke, "solvent": solvent},
		llmsByID:      map[uint]*models.LLM{1: primary, 2: broke, 3: solvent},
		config:        &Config{},
		budgetService: stubBudget{deny: 2},
	}
	r := httptest.NewRequest(http.MethodPost, "/ai/primary/v1/chat/completions", nil)
	r = r.WithContext(context.WithValue(r.Context(), "app", &models.App{ID: 7}))

	plan := p.planFailover(primary, "gpt-4o", r)
	require.Len(t, plan.attempts, 2, "a fallback over budget is not a fallback")
	assert.Equal(t, "solvent", plan.attempts[1].slug)

	// Without an app in context there is nothing to check a budget against,
	// and the rung is kept for the inner hop to decide.
	plan = p.planFailover(primary, "gpt-4o", httptest.NewRequest(http.MethodPost, "/", nil))
	assert.Len(t, plan.attempts, 3)
}

func TestFailover_SpoofedMarkerCannotTaintAnalytics(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("direct"), serveOpenAIText("never"), nil)

	// Straight at the inner hop for an LLM the app IS allowed, carrying a
	// forged marker: served as a plain request, and recorded as one.
	resp, _ := h.post("/llm/call/primary/v1/chat/completions", failoverChatBody,
		hdrFailoverOrigin, "fallback", hdrFailoverAttempt, "1", hdrFailoverToken, "guess")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	waitForProxyLog(t, h.db, h.app.ID, http.StatusOK)
	logs := h.proxyLogs()
	require.Len(t, logs, 1)
	assert.Equal(t, h.primary.ID, logs[0].LLMID)
	assert.Nil(t, logs[0].FailoverFromLLMID, "a forged marker must not be recorded")
	assert.Equal(t, 0, logs[0].FailoverAttempt)
}

func TestFailover_UnifiedRouterInheritsTheWaterfall(t *testing.T) {
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("via unified"), nil)

	resp, body := h.post("/v1/chat/completions",
		`{"model":"primary/gpt-4o","messages":[{"role":"user","content":"Say this is a test!"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, "true", resp.Header.Get(hdrFailover))
	var out ChatCompletionResponse
	require.NoError(t, json.Unmarshal(body, &out))
	assert.Equal(t, "via unified", out.Choices[0].Message.Content)
}

func TestFailover_TimeoutTrigger(t *testing.T) {
	slow := func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		serveStatus(500)(w, r)
	}
	t.Run("fails over when on", func(t *testing.T) {
		h := newFailoverHarness(t, slow, serveOpenAIText("after timeout"), func(primary, fallback *models.LLM) {
			primary.Failover.Triggers = &models.LLMFailoverTriggers{AttemptTimeoutSecond: 1}
		})
		start := time.Now()
		resp, body := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
		assert.Equal(t, "true", resp.Header.Get(hdrFailover))
		assert.Less(t, time.Since(start), 4*time.Second, "the primary was abandoned at the attempt timeout")
	})
	t.Run("does not when off", func(t *testing.T) {
		off := false
		h := newFailoverHarness(t, slow, serveOpenAIText("never"), func(primary, fallback *models.LLM) {
			primary.Failover.Triggers = &models.LLMFailoverTriggers{AttemptTimeoutSecond: 1, OnTimeout: &off}
		})
		resp, _ := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
		assert.NotEqual(t, http.StatusOK, resp.StatusCode)
		assert.Empty(t, h.fallbackVendor.calls())
	})
}

func TestFailover_MetricCounted(t *testing.T) {
	handler := metrics.Init()
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("counted"), nil)

	resp, _ := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	scrape := rec.Body.String()
	var counted bool
	for _, line := range strings.Split(scrape, "\n") {
		if strings.HasPrefix(line, "aistudio_llm_failover_total{") &&
			strings.Contains(line, `from_llm="primary"`) &&
			strings.Contains(line, `reason="status_503"`) &&
			strings.Contains(line, `to_llm="fallback"`) &&
			strings.HasSuffix(line, " 1") {
			counted = true
		}
	}
	assert.True(t, counted, "scrape:\n%s", scrape)
}
