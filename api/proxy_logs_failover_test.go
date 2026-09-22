package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proxyLogAttrs is what the two proxy-log endpoints put under attributes.
// The failover marker is written to ProxyLog by both the hub proxy and the
// analytics pulse, but the response type never carried it, so the LLM and
// App detail views could not tell a fallback row from a primary one.
type proxyLogAttrs struct {
	AppID             uint   `json:"app_id"`
	LLMID             uint   `json:"llm_id"`
	ModelName         string `json:"model_name"`
	Vendor            string `json:"vendor"`
	ResponseCode      int    `json:"response_code"`
	FailoverAttempt   int    `json:"failover_attempt"`
	FailoverFromLLMID *uint  `json:"failover_from_llm_id"`
}

func TestProxyLogs_FailoverAttributesSerialised(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.ProxyLog{}))
	api, router := setupAnalyticsTestAPI(db)
	router.GET("/analytics/proxy-logs-for-llm", api.getProxyLogsForLLM)
	router.GET("/analytics/proxy-logs-for-app", api.getProxyLogsForApp)

	now := time.Now().Truncate(time.Second)
	const appID, primaryID, fallbackID = 7, 3, 4
	from := uint(primaryID)
	require.NoError(t, db.Create(&models.ProxyLog{
		AppID: appID, LLMID: primaryID, Vendor: "openai", ModelName: "gpt-4o",
		ResponseCode: http.StatusServiceUnavailable, TimeStamp: now,
	}).Error)
	require.NoError(t, db.Create(&models.ProxyLog{
		AppID: appID, LLMID: fallbackID, Vendor: "anthropic", ModelName: "claude-sonnet-4",
		ResponseCode: http.StatusOK, TimeStamp: now.Add(time.Second),
		FailoverFromLLMID: &from, FailoverAttempt: 1,
	}).Error)

	get := func(path string, params map[string]string) []proxyLogAttrs {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		q := req.URL.Query()
		q.Add("start_date", now.AddDate(0, 0, -1).Format("2006-01-02"))
		q.Add("end_date", now.AddDate(0, 0, 1).Format("2006-01-02"))
		for k, v := range params {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

		var out struct {
			Data []struct {
				Attributes proxyLogAttrs `json:"attributes"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		attrs := make([]proxyLogAttrs, len(out.Data))
		for i, d := range out.Data {
			attrs[i] = d.Attributes
		}
		return attrs
	}

	t.Run("for app: both attempts, fallback row marked", func(t *testing.T) {
		rows := get("/analytics/proxy-logs-for-app", map[string]string{"app_id": "7"})
		require.Len(t, rows, 2)
		// Newest first.
		fallback, primary := rows[0], rows[1]
		assert.Equal(t, uint(fallbackID), fallback.LLMID)
		assert.Equal(t, "claude-sonnet-4", fallback.ModelName)
		assert.Equal(t, 1, fallback.FailoverAttempt)
		require.NotNil(t, fallback.FailoverFromLLMID)
		assert.Equal(t, uint(primaryID), *fallback.FailoverFromLLMID)

		assert.Equal(t, uint(primaryID), primary.LLMID)
		assert.Equal(t, "gpt-4o", primary.ModelName)
		assert.Equal(t, 0, primary.FailoverAttempt)
		assert.Nil(t, primary.FailoverFromLLMID)
	})

	t.Run("for llm: the fallback's own view carries the marker", func(t *testing.T) {
		rows := get("/analytics/proxy-logs-for-llm", map[string]string{"llm_id": "4"})
		require.Len(t, rows, 1)
		assert.Equal(t, uint(fallbackID), rows[0].LLMID)
		assert.Equal(t, 1, rows[0].FailoverAttempt)
		require.NotNil(t, rows[0].FailoverFromLLMID)
		assert.Equal(t, uint(primaryID), *rows[0].FailoverFromLLMID)
	})

	t.Run("for llm: a primary row has no from-id key", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/analytics/proxy-logs-for-llm?llm_id=3&start_date="+
			now.AddDate(0, 0, -1).Format("2006-01-02")+"&end_date="+now.AddDate(0, 0, 1).Format("2006-01-02"), nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), `"failover_from_llm_id"`, "omitempty on a nil pointer")
		assert.Contains(t, w.Body.String(), `"failover_attempt":0`)
	})
}
