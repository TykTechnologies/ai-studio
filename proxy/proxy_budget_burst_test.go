//go:build enterprise
// +build enterprise

package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

// burstFixture is an embedded proxy in front of a mock upstream whose every
// response costs $0.30 (100 prompt tokens at $0.001, 100 response tokens at
// $0.002), and an App with a $1 budget.
type burstFixture struct {
	db      *gorm.DB
	proxy   *Proxy
	srv     *httptest.Server
	app     *models.App
	budgetS budget.Service
}

func newBurstFixture(t *testing.T) *burstFixture {
	t.Helper()
	db, cancel := setupTest(t)
	t.Cleanup(func() { tearDownTest(db, cancel) })

	// As in main.go: the proxy checks with the service's own budget
	// service, the one the post-request analysis reports spend to.
	service := services.NewService(db)
	service.InitBudgets(services.NewTestNotificationService(db))
	budgetService := service.Budget
	p := NewProxy(service, &Config{Port: 9999}, budgetService)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "mock", "object": "chat.completion", "model": "test-model",
			"choices": []map[string]interface{}{{"message": map[string]interface{}{"content": "hi"}}},
			"usage":   map[string]interface{}{"prompt_tokens": 100, "completion_tokens": 100, "total_tokens": 200},
		})
	}))
	t.Cleanup(upstream.Close)

	llm := &models.LLM{Name: "BurstLLM", Vendor: models.MOCK_VENDOR, DefaultModel: "test-model", Active: true, APIEndpoint: upstream.URL}
	require.NoError(t, db.Create(llm).Error)
	require.NoError(t, db.Create(&models.ModelPrice{ModelName: "test-model", Vendor: string(models.MOCK_VENDOR), CPT: 0.002, CPIT: 0.001, Currency: "USD"}).Error)

	user := &models.User{Email: "burst@example.com"}
	require.NoError(t, db.Create(user).Error)
	cred := &models.Credential{Secret: "burst-token", Active: true}
	require.NoError(t, db.Create(cred).Error)
	appBudget := 1.0
	app := &models.App{Name: "BurstApp", MonthlyBudget: &appBudget, UserID: user.ID, CredentialID: cred.ID}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, app.AddLLM(db, llm))

	p.credValidator.RegisterValidator(string(models.MOCK_VENDOR), func(r *http.Request) (string, error) {
		return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), nil
	})
	require.NoError(t, p.loadResources())

	srv := httptest.NewServer(p.createHandler())
	t.Cleanup(srv.Close)
	return &burstFixture{db: db, proxy: p, srv: srv, app: app, budgetS: budgetService}
}

func (f *burstFixture) send(t *testing.T) int {
	t.Helper()
	body := []byte(`{"model": "test-model", "prompt": "Hello"}`)
	req, err := http.NewRequest(http.MethodPost, f.srv.URL+"/llm/rest/burstllm/v1/test", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer burst-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	return resp.StatusCode
}

// Each served request's cost counts toward the next check: $0.30 requests on
// a $1 budget stop after 0, 0.30, 0.60 and 0.90 have been spent. The budget
// check used to cache the spend for five minutes (and treat a cold cache as
// nothing spent), so every request of the burst was served.
func TestEmbeddedBudget_SequentialBurstStopsAtBudget(t *testing.T) {
	f := newBurstFixture(t)

	served := 0
	for i := 0; i < 10; i++ {
		if f.send(t) == http.StatusOK {
			served++
		}
		f.proxy.waitForAnalyzers() // the request's analysis has run
	}
	assert.Equal(t, 4, served)
}

// Without waiting for the analysis between requests, the overshoot is
// bounded by the requests whose analysis had not run yet.
func TestEmbeddedBudget_UnpacedBurstOvershootIsBounded(t *testing.T) {
	f := newBurstFixture(t)

	served := 0
	for i := 0; i < 20; i++ {
		if f.send(t) == http.StatusOK {
			served++
		}
	}
	f.proxy.waitForAnalyzers()
	assert.GreaterOrEqual(t, served, 4)
	assert.LessOrEqual(t, served, 6, "a paced client can overshoot by at most the requests still being analysed")
	assert.Equal(t, http.StatusForbidden, f.send(t), "once the budget is spent every request is refused")
}

// Spend already in the database is counted on the first request after a
// restart (a cold cache used to mean nothing spent).
func TestEmbeddedBudget_StoredSpendRefusedOnFirstRequest(t *testing.T) {
	f := newBurstFixture(t)
	require.NoError(t, f.db.Create(&models.LLMChatRecord{
		AppID: f.app.ID, Cost: 2 * 10000, TimeStamp: time.Now(),
	}).Error)

	assert.Equal(t, http.StatusForbidden, f.send(t))
}
