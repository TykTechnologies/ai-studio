package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The app budget views report a budget's currency as implied by its spend:
// a EUR-only App in EUR, and for a period that mixes currencies (which the
// budget refuses) the spend by currency instead of a sum. Spend is tracked in
// Enterprise only.
func TestBudgetViews_ImpliedCurrency(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)
	require.NoError(t, db.AutoMigrate(&models.LLMChatRecord{}))
	user := createTestUser(t, service)
	llmCat := createTestCatalogue(t, service)
	addCatalogueToUserGroup(t, service, user.ID, llmCat.ID)
	llm := createTestLLM(t, service, "LLM")
	require.NoError(t, service.AddLLMToCatalogue(llm.ID, llmCat.ID))
	limit := 100.0
	eur, err := service.CreateApp("EUR", "", user.ID, nil, []uint{llm.ID}, nil, &limit, nil, nil)
	require.NoError(t, err)
	mixed, err := service.CreateApp("Mixed", "", user.ID, nil, []uint{llm.ID}, nil, &limit, nil, nil)
	require.NoError(t, err)
	now := time.Now()
	for _, r := range []models.LLMChatRecord{
		{AppID: eur.ID, LLMID: llm.ID, Cost: 250000, Currency: "EUR", TimeStamp: now.Add(-time.Minute)},
		{AppID: mixed.ID, LLMID: llm.ID, Cost: 10000, Currency: "", TimeStamp: now.Add(-time.Minute)},
		{AppID: mixed.ID, LLMID: llm.ID, Cost: 20000, Currency: "EUR", TimeStamp: now.Add(-time.Minute)},
	} {
		require.NoError(t, db.Create(&r).Error)
	}

	w := portalGet(t, api.getUserAppsUsageSummary, user)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var summary AppUsageSummaryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))

	forApp := func(id uint) map[string]interface{} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/analytics/budget-usage-for-app?app_id=%d", id), nil)
		c.Set("user", user)
		api.getBudgetUsageForApp(c)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var out map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		return out
	}
	eurView, mixedView := forApp(eur.ID), forApp(mixed.ID)
	// The period the view covers, for the UI to show rather than recompute.
	start, next := budget.Period(nil, time.Now())
	assert.Equal(t, start.Format(time.RFC3339), eurView["start_date"])
	assert.Equal(t, next.Format(time.RFC3339), eurView["period_end"])

	if !budget.IsEnterpriseAvailable() {
		assert.Empty(t, summary.Data[idOf(eur.ID)].Currency)
		assert.Nil(t, eurView["currency"])
		return
	}
	e := summary.Data[idOf(eur.ID)]
	assert.Equal(t, "EUR", e.Currency)
	assert.InDelta(t, 25, e.CurrentSpend, 1e-9)
	assert.Nil(t, e.MixedCurrencySpend)
	m := summary.Data[idOf(mixed.ID)]
	assert.Empty(t, m.Currency)
	assert.Zero(t, m.CurrentSpend, "not added up")
	assert.Equal(t, map[string]float64{"USD": 1, "EUR": 2}, m.MixedCurrencySpend)

	assert.Equal(t, "EUR", eurView["currency"])
	assert.Nil(t, eurView["mixed_currency_spend"])
	assert.Nil(t, mixedView["currency"])
	assert.Equal(t, 0.0, mixedView["current_usage"])
	assert.Equal(t, map[string]interface{}{"USD": 1.0, "EUR": 2.0}, mixedView["mixed_currency_spend"])
}
