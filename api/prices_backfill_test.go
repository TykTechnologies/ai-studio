package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The price list counts each price's unpriced history, and
// POST /model-prices/:id/backfill prices it, over an optional date range.
func TestBackfillModelPriceHistory(t *testing.T) {
	api, db := setupTestAPI(t)
	vendor := string(models.GOOGLEAI)
	price := models.ModelPrice{ModelName: "gemini-2.5-pro", Vendor: vendor, Currency: "USD", CPIT: 0.000001, CPT: 0.000004}
	require.NoError(t, price.Create(db))
	for _, d := range []int{1, 10, 20} {
		require.NoError(t, db.Create(&models.LLMChatRecord{Name: "gemini-2.5-pro", Vendor: vendor,
			PromptTokens: 1000, ResponseTokens: 100, TotalTokens: 1100, Currency: "USD",
			TimeStamp: time.Date(2026, 5, d, 12, 0, 0, 0, time.UTC)}).Error)
	}

	unpriced := func() float64 {
		w := performRequest(api.router, http.MethodGet, "/api/v1/model-prices", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var out struct {
			Data []struct {
				ID   string         `json:"id"`
				Meta map[string]any `json:"meta"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		require.Len(t, out.Data, 1)
		return out.Data[0].Meta["unpriced_rows"].(float64)
	}
	assert.Equal(t, 3.0, unpriced())

	path := fmt.Sprintf("/api/v1/model-prices/%d/backfill", price.ID)
	w := performRequest(api.router, http.MethodPost, path, map[string]string{"from": "2026-05-10", "to": "2026-05-20"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp BackfillModelPriceResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int64(2), resp.Data.RowsPriced, "both ends of the range are inclusive")
	assert.Equal(t, 1.0, unpriced())

	w = performRequest(api.router, http.MethodPost, path, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int64(1), resp.Data.RowsPriced)
	assert.Equal(t, 0.0, unpriced())

	assert.Equal(t, http.StatusBadRequest, performRequest(api.router, http.MethodPost, path, map[string]string{"from": "May 1"}).Code)
	assert.Equal(t, http.StatusBadRequest, performRequest(api.router, http.MethodPost, path, map[string]string{"from": "2026-05-20", "to": "2026-05-10"}).Code)
	assert.Equal(t, http.StatusNotFound, performRequest(api.router, http.MethodPost, "/api/v1/model-prices/9999/backfill", nil).Code)
}
