package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Decision-model fixtures: an OpenAI Decisions reply and a System One reply
// to the same three questions.

const decisionRequestOpenAI = `{"model":"gpt-6-luna","input":"I was charged twice for my order.",` +
	`"questions":[` +
	`{"type":"predicate","name":"angry","instructions":"Is the customer angry?"},` +
	`{"type":"choice","name":"topic","instructions":"Which team?","choices":[{"value":"shipping"},{"value":"billing"}]},` +
	`{"type":"score","name":"urgency","instructions":"How urgent?","levels":[{"label":"low"},{"label":"high"}]}]}`

const decisionRequestSystemOne = `{"model":"jev-latest","state":"I was charged twice for my order.",` +
	`"questions":{"angry":{"type":"noul","instructions":"Is the customer angry?"},` +
	`"topic":{"type":"choice","instructions":"Which team?","criteria":{"shipping":"shipping","billing":"billing"}},` +
	`"urgency":{"type":"score","instructions":"How urgent?","criteria":["low","high"]}}}`

func serveOpenAIDecision() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"dec_1","object":"decision","model":"gpt-6-luna","answers":[` +
			`{"type":"predicate","name":"angry","probability":0.8},` +
			`{"type":"choice","name":"topic","choice":"billing","probabilities":[{"value":"shipping","probability":0.1},{"value":"billing","probability":0.9}],"confidence":0.9},` +
			`{"type":"refusal","name":"urgency"}],` +
			`"usage":{"input_tokens":120,"output_tokens":0}}`))
	}
}

// serveSystemOneDecision answers two of the three questions: System One
// leaves an unanswered question out of the answers map.
func serveSystemOneDecision() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{` +
			`"angry":{"type":"noul","noul":0.8},` +
			`"topic":{"type":"choice","choice":"billing","probabilities":{"shipping":0.1,"billing":0.9},"confidence":0.9}},` +
			`"usage":{"input_tokens":200,"output_tokens":0}}`))
	}
}

func asSystemOne(primary, _ *models.LLM) {
	primary.Vendor = models.SYSTEMONE
	primary.DefaultModel = "jev-latest"
	primary.Failover = models.LLMFailover{}
}

func noFailover(primary, _ *models.LLM) {
	primary.Failover = models.LLMFailover{}
}

func (h *failoverHarness) chatRecords() []models.LLMChatRecord {
	var recs []models.LLMChatRecord
	require.NoError(h.t, h.db.Where("app_id = ?", h.app.ID).Order("id asc").Find(&recs).Error)
	return recs
}

func (h *failoverHarness) addPrice(model string, vendor models.Vendor, cpit float64) {
	require.NoError(h.t, h.db.Create(&models.ModelPrice{ModelName: model, Vendor: string(vendor), CPIT: cpit, Currency: "USD"}).Error)
}

// Before decision support, a native OpenAI /v1/decisions call failed
// response analysis and left no proxy log, no chat record and no spend.
func TestDecisions_NativeOpenAIPassThroughIsRecordedAndPriced(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIDecision(), serveStatus(500), noFailover)
	h.addPrice("gpt-6-luna", models.OPENAI, 0.0000001)

	resp, body := h.post("/llm/rest/primary/v1/decisions", decisionRequestOpenAI)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Contains(t, string(body), `"answers"`)

	calls := h.primaryVendor.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "/v1/decisions", calls[0].Path)

	waitForAnalytics(t, h.db, 1)
	h.proxy.waitForAnalyzers()

	recs := h.chatRecords()
	require.Len(t, recs, 1)
	rec := recs[0]
	assert.Equal(t, models.DecisionRequest, rec.RequestKind)
	assert.Equal(t, "gpt-6-luna", rec.Name)
	assert.Equal(t, 120, rec.PromptTokens)
	assert.Equal(t, 0, rec.ResponseTokens)
	assert.Equal(t, 2, rec.Choices, "answered questions")
	assert.Equal(t, 3, rec.DecisionQuestions)
	assert.Equal(t, 1, rec.DecisionRefusals)
	// 120 input tokens at $0.10/1M, stored in the x10000 cost unit.
	assert.InDelta(t, 120*0.0000001*10000, rec.Cost, 1e-9)

	logs := h.proxyLogs()
	require.Len(t, logs, 1)
	assert.Equal(t, models.DecisionRequest, logs[0].RequestKind)
}

func TestDecisions_NativeSystemOnePassThroughIsRecordedAndPriced(t *testing.T) {
	h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), asSystemOne)
	h.addPrice("jev-1.13.0", models.SYSTEMONE, 0.000000042)

	resp, body := h.post("/llm/rest/primary/v1/systemone", decisionRequestSystemOne)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	calls := h.primaryVendor.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "/v1/systemone", calls[0].Path)
	assert.Equal(t, "Bearer primary-vendor-key", calls[0].Headers.Get("Authorization"))

	waitForAnalytics(t, h.db, 1)
	h.proxy.waitForAnalyzers()

	recs := h.chatRecords()
	require.Len(t, recs, 1)
	rec := recs[0]
	assert.Equal(t, models.DecisionRequest, rec.RequestKind)
	assert.Equal(t, string(models.SYSTEMONE), rec.Vendor)
	assert.Equal(t, 200, rec.PromptTokens)
	assert.Equal(t, 2, rec.Choices)
	// The third question is missing from the answers map: only the request
	// says it was asked.
	assert.Equal(t, 3, rec.DecisionQuestions)
	assert.Equal(t, 1, rec.DecisionRefusals)
	assert.InDelta(t, 200*0.000000042*10000, rec.Cost, 1e-9)
}

// Found by the live conformance suite: asked for the alias jev-latest, Jev
// answers as jev-1.13.0. An admin prices the name they configured; pricing by
// the reported name alone recorded the call at zero cost, outside budgets.
func TestDecisions_AliasIsPricedByTheRequestedModel(t *testing.T) {
	h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), asSystemOne)
	h.addPrice("jev-latest", models.SYSTEMONE, 0.000000042)

	resp, body := h.post("/llm/rest/primary/v1/systemone", decisionRequestSystemOne)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	waitForAnalytics(t, h.db, 1)
	h.proxy.waitForAnalyzers()
	recs := h.chatRecords()
	require.Len(t, recs, 1)
	assert.Equal(t, "jev-1.13.0", recs[0].Name, "the record names the model that answered")
	assert.InDelta(t, 200*0.000000042*10000, recs[0].Cost, 1e-9, "priced by the requested alias")
}

// The reported model's own price wins when it has one.
func TestDecisions_ReportedModelPriceWins(t *testing.T) {
	h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), asSystemOne)
	h.addPrice("jev-latest", models.SYSTEMONE, 0.000000042)
	h.addPrice("jev-1.13.0", models.SYSTEMONE, 0.0000001)

	resp, body := h.post("/llm/rest/primary/v1/systemone", decisionRequestSystemOne)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	waitForAnalytics(t, h.db, 1)
	h.proxy.waitForAnalyzers()
	recs := h.chatRecords()
	require.Len(t, recs, 1)
	assert.InDelta(t, 200*0.0000001*10000, recs[0].Cost, 1e-9)
}

// serveDatedChat answers a chat completion as a dated snapshot of the model,
// as OpenAI does.
func serveDatedChat(dated string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"` + dated + `",` +
			`"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}
}

// The price fallback is not decision-specific: a chat model priced under the
// name the caller uses, answering as a dated snapshot, was also recorded at
// zero cost. Chat records keep naming the snapshot that answered.
func TestPricing_ChatSnapshotIsPricedByTheRequestedModel(t *testing.T) {
	h := newFailoverHarness(t, serveDatedChat("gpt-4o-2024-08-06"), serveStatus(500), noFailover)
	require.NoError(t, h.db.Create(&models.ModelPrice{ModelName: "gpt-4o", Vendor: "openai", CPIT: 0.0000025, CPT: 0.00001, Currency: "USD"}).Error)

	resp, body := h.post("/llm/rest/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	waitForAnalytics(t, h.db, 1)
	h.proxy.waitForAnalyzers()

	recs := h.chatRecords()
	require.Len(t, recs, 1)
	assert.Equal(t, "gpt-4o-2024-08-06", recs[0].Name)
	assert.Equal(t, models.GenerativeRequest, recs[0].RequestKind)
	assert.InDelta(t, (10*0.0000025+5*0.00001)*10000, recs[0].Cost, 1e-9)
}

// Neither name priced: the exchange is still recorded, at zero cost, as before.
func TestPricing_UnpricedModelsStayZero(t *testing.T) {
	h := newFailoverHarness(t, serveDatedChat("gpt-4o-2024-08-06"), serveStatus(500), noFailover)

	resp, body := h.post("/llm/rest/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	waitForAnalytics(t, h.db, 1)
	h.proxy.waitForAnalyzers()

	recs := h.chatRecords()
	require.Len(t, recs, 1)
	assert.Zero(t, recs[0].Cost)
	assert.Equal(t, 15, recs[0].TotalTokens)
}

// A model whose reported and requested names match takes one lookup and is
// priced exactly as before the fallback existed.
func TestPricing_ExactModelUnchanged(t *testing.T) {
	h := newFailoverHarness(t, serveDatedChat("gpt-4o"), serveStatus(500), noFailover)
	require.NoError(t, h.db.Create(&models.ModelPrice{ModelName: "gpt-4o", Vendor: "openai", CPIT: 0.0000025, CPT: 0.00001, Currency: "USD"}).Error)

	resp, body := h.post("/llm/rest/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	waitForAnalytics(t, h.db, 1)
	h.proxy.waitForAnalyzers()

	recs := h.chatRecords()
	require.Len(t, recs, 1)
	assert.InDelta(t, (10*0.0000025+5*0.00001)*10000, recs[0].Cost, 1e-9)
}

func TestDecisions_NativeSystemOneScreening(t *testing.T) {
	h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), asSystemOne)
	for name, body := range map[string]string{
		"no state":           `{"model":"jev-latest","questions":{"q":{"type":"noul","instructions":"?"}}}`,
		"questions as array": `{"model":"jev-latest","state":"x","questions":[{"type":"noul"}]}`,
		"no questions":       `{"model":"jev-latest","state":"x"}`,
		"not JSON":           `{"model":`,
		"no model":           `{"state":"x","questions":{"q":{"type":"noul","instructions":"?"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			resp, out := h.post("/llm/rest/primary/v1/systemone", body)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
		})
	}
	assert.Empty(t, h.primaryVendor.calls(), "a malformed request never reaches the vendor")
}

// A failed decision call is still logged as a decision, and its questions
// were not refused by the model: a refusal is the model declining a question
// it was asked, not an upstream error, a budget denial or a policy block.
func TestDecisions_FailuresAreRecordedWithoutRefusals(t *testing.T) {
	t.Run("upstream validation error", func(t *testing.T) {
		serve := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"detail":"questions.team.criteria: too few options"}`))
		}
		h := newFailoverHarness(t, serve, serveStatus(500), asSystemOne)
		resp, _ := h.post("/llm/rest/primary/v1/systemone", decisionRequestSystemOne)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

		waitForAnalytics(t, h.db, 1)
		h.proxy.waitForAnalyzers()
		recs := h.chatRecords()
		require.Len(t, recs, 1)
		assert.Equal(t, models.DecisionRequest, recs[0].RequestKind)
		assert.Equal(t, 3, recs[0].DecisionQuestions)
		assert.Equal(t, 0, recs[0].DecisionRefusals)
		assert.Equal(t, 0, recs[0].Choices)
		logs := h.proxyLogs()
		require.Len(t, logs, 1)
		assert.Equal(t, http.StatusUnprocessableEntity, logs[0].ResponseCode)
		assert.Equal(t, models.DecisionRequest, logs[0].RequestKind)
	})

	t.Run("budget denial", func(t *testing.T) {
		h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), asSystemOne)
		h.proxy.budgetService = denyBudget{err: fmt.Errorf("budget exceeded: app monthly budget is 0")}
		resp, _ := h.post("/llm/rest/primary/v1/systemone", decisionRequestSystemOne)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.Empty(t, h.primaryVendor.calls())

		waitForAnalytics(t, h.db, 1)
		h.proxy.waitForAnalyzers()
		recs := h.chatRecords()
		require.Len(t, recs, 1)
		assert.Equal(t, models.DecisionRequest, recs[0].RequestKind)
		assert.Equal(t, 0, recs[0].DecisionRefusals)
		assert.Zero(t, recs[0].Cost)
	})

	t.Run("a non-JSON upstream body is not recorded, as for chat", func(t *testing.T) {
		serve := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<html>bad gateway</html>"))
		}
		h := newFailoverHarness(t, serve, serveStatus(500), asSystemOne)
		resp, _ := h.post("/llm/rest/primary/v1/systemone", decisionRequestSystemOne)
		require.Equal(t, http.StatusBadGateway, resp.StatusCode)
		h.proxy.waitForAnalyzers()
		assert.Empty(t, h.chatRecords())
	})
}

func TestDecisions_NativePassThroughRejectsStreaming(t *testing.T) {
	h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), asSystemOne)

	var req map[string]any
	require.NoError(t, json.Unmarshal([]byte(decisionRequestSystemOne), &req))
	req["stream"] = true
	b, _ := json.Marshal(req)

	resp, _ := h.post("/llm/rest/primary/v1/systemone", string(b))
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Empty(t, h.primaryVendor.calls())
}

func TestDecisions_SystemOneLLMServesOnlyItsDecisionEndpoint(t *testing.T) {
	h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), asSystemOne)

	resp, _ := h.post("/llm/rest/primary/v1/chat/completions", failoverChatBody)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Empty(t, h.primaryVendor.calls())
}
