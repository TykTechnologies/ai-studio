package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
)

func TestServiceTierUsageEndpoint(t *testing.T) {
	api, db := setupTestAPI(t)
	at := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, db.Create(&[]models.LLMChatRecord{
		{LLMID: 1, AppID: 10, Vendor: "anthropic", Name: "claude-luna", TotalTokens: 100, Cost: 20, Currency: "USD", TimeStamp: at, ServiceTier: "batch"},
		{LLMID: 1, AppID: 10, Vendor: "anthropic", Name: "claude-luna", TotalTokens: 10, Cost: 4, Currency: "USD", TimeStamp: at},
	}).Error)
	day := at.Format("2006-01-02")
	base := "/api/v1/analytics/service-tier-usage?start_date=" + day + "&end_date=" + time.Now().UTC().Add(24*time.Hour).Format("2006-01-02")

	w := performRequest(api.router, http.MethodGet, base, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var usage analytics.ServiceTierUsage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &usage))
	assert.Len(t, usage.ByTier, 2)
	assert.Len(t, usage.ByModel, 2)

	w = performRequest(api.router, http.MethodGet, base+"&service_tier=batch", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &usage))
	require.Len(t, usage.ByTier, 1)
	assert.Equal(t, "batch", usage.ByTier[0].ServiceTier)
	assert.InDelta(t, 20.0/10000, usage.ByTier[0].CostByCurrency["USD"], 1e-12)

	w = performRequest(api.router, http.MethodGet, base+"&service_tier=none", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &usage))
	require.Len(t, usage.ByTier, 1)
	assert.Equal(t, "", usage.ByTier[0].ServiceTier)

	for _, q := range []string{"&service_tier=Batch", "&service_tier=", "&llm_id=x", "&app_id=-1"} {
		w = performRequest(api.router, http.MethodGet, base+q, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
	}
	w = performRequest(api.router, http.MethodGet, "/api/v1/analytics/service-tier-usage", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, "dates are required")
}
