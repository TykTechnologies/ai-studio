package proxy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gosimple/slug"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

// The analysis of a request runs after the response (goAnalyze) and must
// record into the analytics handler that was current when the request
// arrived, not into one installed in between. midsommar-clean-vrc.14: a
// serial test's late analysis landed in the next test's handler
// (TestAnalyzeResponse_CountTokensLogVerify saw "logs=2 records=1").
// Here the global handler is replaced while the upstream answers.
func TestAnalysisRecordsIntoTheHandlerCurrentAtRequestStart(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)
	service := services.NewService(db)
	budgetService := budget.NewService(db, services.NewTestNotificationService(db))

	atStart, replacement := &chatRecordCounter{}, &chatRecordCounter{}
	analytics.SetHandler(atStart)
	defer analytics.ResetHandler()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		analytics.SetHandler(replacement) // the next test installs its own
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"completion":"ok"}`))
	}))
	defer upstream.Close()

	cred := &models.Credential{Secret: "bind-handler-cred", Active: true}
	require.NoError(t, db.Create(cred).Error)
	app := &models.App{Name: "Bind handler app", CredentialID: cred.ID}
	require.NoError(t, db.Create(app).Error)
	llm := &models.LLM{Name: "Bind handler LLM", APIKey: "sk-fake", APIEndpoint: upstream.URL, Vendor: models.MOCK_VENDOR,
		Active: true, DefaultModel: "m", AllowInternalHost: true}
	require.NoError(t, db.Create(llm).Error)
	require.NoError(t, app.AddLLM(db, llm))

	p := NewProxy(service, &Config{Port: 8080}, budgetService)
	require.NoError(t, p.loadResources())
	req := httptest.NewRequest("POST", "/llm/rest/"+slug.Make(llm.Name)+"/v1/chat/completions",
		bytes.NewReader([]byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", cred.Secret)
	req = req.WithContext(context.WithValue(req.Context(), "app", app))
	w := httptest.NewRecorder()
	p.createHandler().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	p.waitForAnalyzers()

	require.Equal(t, 1, atStart.logCount(), "the request's proxy log goes to the handler current when it arrived")
	require.Zero(t, replacement.logCount()+replacement.count(), "nothing lands in the handler installed afterwards")
}
