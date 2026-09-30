package proxy

// TAS-45: reproduction for OpenAI /v1/responses traffic through the unified
// /llm/call/{slug}/ route: it reaches OpenAI, but the OpenAI analyzer only
// recognises /v1/chat/completions and /v1/embeddings, so no ProxyLog or
// LLMChatRecord is written and nothing counts toward a budget.
//
// The chat completions tests are controls: they pass today and prove the
// harness records what it should. The /v1/responses tests assert the correct
// behaviour and fail until the analyzer understands the Responses API.
// Budget enforcement is Enterprise-only; its reproduction is in
// openai_responses_budget_test.go (-tags enterprise).

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

const (
	openAITestSlug  = "openai-api"
	openAITestModel = "gpt-4o-mini"
)

const openAIChatCompletionBody = `{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "model": "gpt-4o-mini",
  "choices": [{"index": 0, "message": {"role": "assistant", "content": "Hello!"}, "finish_reason": "stop"}],
  "usage": {"prompt_tokens": 12, "completion_tokens": 30, "total_tokens": 42}
}`

// Shape of a non-streaming Responses API reply.
const openAIResponsesBody = `{
  "id": "resp_123",
  "object": "response",
  "created_at": 1741476542,
  "status": "completed",
  "model": "gpt-4o-mini",
  "output": [{
    "type": "message",
    "id": "msg_123",
    "status": "completed",
    "role": "assistant",
    "content": [{"type": "output_text", "text": "Hello!", "annotations": []}]
  }],
  "usage": {
    "input_tokens": 12,
    "input_tokens_details": {"cached_tokens": 0},
    "output_tokens": 30,
    "output_tokens_details": {"reasoning_tokens": 0},
    "total_tokens": 42
  }
}`

// Shape of a streaming Responses API reply: named SSE events, usage only on
// response.completed.
var openAIResponsesStream = []string{
	"event: response.created\ndata: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_123\",\"object\":\"response\",\"status\":\"in_progress\",\"model\":\"gpt-4o-mini\",\"output\":[],\"usage\":null}}\n\n",
	"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"msg_123\",\"output_index\":0,\"content_index\":0,\"delta\":\"Hello!\"}\n\n",
	"event: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"id\":\"resp_123\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"gpt-4o-mini\",\"output\":[{\"type\":\"message\",\"id\":\"msg_123\",\"status\":\"completed\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"Hello!\",\"annotations\":[]}]}],\"usage\":{\"input_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":30,\"output_tokens_details\":{\"reasoning_tokens\":0},\"total_tokens\":42}}}\n\n",
}

// newOpenAIUpstream stands in for api.openai.com.
func newOpenAIUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		streaming := bytes.Contains(body, []byte(`"stream":true`))

		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/chat/completions"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(openAIChatCompletionBody))
		case strings.HasSuffix(r.URL.Path, "/v1/responses") && streaming:
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			for _, ev := range openAIResponsesStream {
				_, _ = w.Write([]byte(ev))
				if flusher != nil {
					flusher.Flush()
				}
			}
		case strings.HasSuffix(r.URL.Path, "/v1/responses"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(openAIResponsesBody))
		default:
			http.NotFound(w, r)
		}
	}))
}

// setupOpenAIProxy wires an OpenAI LLM, a price for gpt-4o-mini and an App
// with a credential. A non-nil appBudget gives the App a monthly budget that
// one request's cost ($0.042) overruns.
func setupOpenAIProxy(t *testing.T, db *gorm.DB, upstreamURL string, appBudget *float64) (*Proxy, *models.LLM, *models.App, services.BudgetService, string) {
	t.Helper()

	service := services.NewService(db)
	budgetService := budget.NewService(db, services.NewTestNotificationService(db))
	budgetService.ClearCache()
	// One budget service for the proxy's checks and the analyzer's recorded
	// spend, as InitBudgets arranges in the server.
	service.Budget = budgetService
	p := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, p)

	user := &models.User{ID: 1, Email: "openai@example.com"}
	require.NoError(t, db.Create(user).Error)

	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         openAITestSlug,
		Vendor:       models.OPENAI,
		DefaultModel: openAITestModel,
		Active:       true,
		APIEndpoint:  upstreamURL,
		APIKey:       "sk-test",
	}
	require.NoError(t, db.Create(llm).Error)

	require.NoError(t, db.Create(&models.ModelPrice{
		ModelName: openAITestModel,
		Vendor:    string(models.OPENAI),
		CPT:       0.001,
		CPIT:      0.001,
		Currency:  "USD",
	}).Error)

	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "OpenAI Test App",
		UserID: user.ID,
	}
	if appBudget != nil {
		app.MonthlyBudget = appBudget
		app.BudgetStartDate = &startOfMonth
	}
	require.NoError(t, db.Create(app).Error)

	cred := &models.Credential{Model: gorm.Model{ID: 1}, Secret: "openai-secret", Active: true}
	require.NoError(t, db.Create(cred).Error)

	app.CredentialID = cred.ID
	app.LLMs = []models.LLM{*llm}
	require.NoError(t, db.Save(app).Error)

	require.NoError(t, p.loadResources())
	return p, llm, app, budgetService, cred.Secret
}

func postOpenAI(t *testing.T, srvURL, secret, path, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/llm/call/%s%s", srvURL, openAITestSlug, path), bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp
}

// drainAnalytics waits for every analysis the proxy has started, then gives
// the asynchronous analytics writer up to 3s to land want chat records. When
// analysis bails out nothing is ever queued, so the count stays below want.
func drainAnalytics(t *testing.T, p *Proxy, db *gorm.DB, want int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, p.WaitForAnalytics(ctx))

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var n int64
		if db.Model(&models.LLMChatRecord{}).Count(&n).Error == nil && n >= want {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	waitUntilIdle(t, db)
}

func countRows(t *testing.T, db *gorm.DB, model interface{}) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(model).Count(&n).Error)
	return n
}

// assertOpenAIExchangeRecorded checks the one exchange produced a chat record
// with the mock's token counts and a cost, plus a proxy log.
func assertOpenAIExchangeRecorded(t *testing.T, db *gorm.DB, llm *models.LLM) {
	t.Helper()
	require.Equal(t, int64(1), countRows(t, db, &models.LLMChatRecord{}), "expected one LLMChatRecord (analytics/budget source)")

	var rec models.LLMChatRecord
	require.NoError(t, db.First(&rec).Error)
	assert.Equal(t, llm.ID, rec.LLMID)
	assert.Equal(t, string(models.OPENAI), rec.Vendor)
	assert.Equal(t, 12, rec.PromptTokens)
	assert.Equal(t, 30, rec.ResponseTokens)
	assert.Equal(t, 42, rec.TotalTokens)
	assert.Greater(t, rec.Cost, 0.0, "the exchange should be priced")

	assert.Equal(t, int64(1), countRows(t, db, &models.ProxyLog{}), "expected one ProxyLog")
}

func TestOpenAIAnalytics_ChatCompletions_Control(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	upstream := newOpenAIUpstream(t)
	defer upstream.Close()
	p, llm, _, _, secret := setupOpenAIProxy(t, db, upstream.URL, nil)
	srv := startProxyServer(t, p)
	defer srv.Close()

	resp := postOpenAI(t, srv.URL, secret, "/v1/chat/completions",
		`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	drainAnalytics(t, p, db, 1)
	assertOpenAIExchangeRecorded(t, db, llm)
}

func TestOpenAIAnalytics_Responses_NonStreaming(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	upstream := newOpenAIUpstream(t)
	defer upstream.Close()
	p, llm, _, _, secret := setupOpenAIProxy(t, db, upstream.URL, nil)
	srv := startProxyServer(t, p)
	defer srv.Close()

	resp := postOpenAI(t, srv.URL, secret, "/v1/responses",
		`{"model":"gpt-4o-mini","input":"Hello"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "the request itself is proxied fine")

	drainAnalytics(t, p, db, 1)
	assertOpenAIExchangeRecorded(t, db, llm)
}

func TestOpenAIAnalytics_Responses_Streaming(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	upstream := newOpenAIUpstream(t)
	defer upstream.Close()
	p, llm, _, _, secret := setupOpenAIProxy(t, db, upstream.URL, nil)
	srv := startProxyServer(t, p)
	defer srv.Close()

	resp := postOpenAI(t, srv.URL, secret, "/v1/responses",
		`{"model":"gpt-4o-mini","input":"Hello","stream":true}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "the request itself is proxied fine")

	drainAnalytics(t, p, db, 1)
	assertOpenAIExchangeRecorded(t, db, llm)
}
