//go:build enterprise
// +build enterprise

package proxy

// TAS-45: budget half of the OpenAI /v1/responses reproduction (see
// openai_responses_analytics_test.go). Spend is only recorded for exchanges the
// analyzer understands, so /v1/responses traffic never exhausts a budget.

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runOpenAIBudgetScenario sends two requests against an App whose budget the
// first request's cost overruns, and returns the second request's status.
func runOpenAIBudgetScenario(t *testing.T, path, body string) int {
	t.Helper()
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	upstream := newOpenAIUpstream(t)
	defer upstream.Close()
	appBudget := 0.01
	p, _, _, _, secret := setupOpenAIProxy(t, db, upstream.URL, &appBudget)
	srv := startProxyServer(t, p)
	defer srv.Close()

	first := postOpenAI(t, srv.URL, secret, path, body)
	require.Equal(t, http.StatusOK, first.StatusCode, "the first request is within budget")

	drainAnalytics(t, p, db, 1)

	second := postOpenAI(t, srv.URL, secret, path, body)
	return second.StatusCode
}

func TestOpenAIBudget_ChatCompletions_Control(t *testing.T) {
	status := runOpenAIBudgetScenario(t, "/v1/chat/completions",
		`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}]}`)
	assert.Equal(t, http.StatusForbidden, status, "spend from the first request should exhaust the $0.01 budget")
}

func TestOpenAIBudget_Responses(t *testing.T) {
	status := runOpenAIBudgetScenario(t, "/v1/responses",
		`{"model":"gpt-4o-mini","input":"Hello"}`)
	assert.Equal(t, http.StatusForbidden, status, "spend from the first request should exhaust the $0.01 budget")
}
