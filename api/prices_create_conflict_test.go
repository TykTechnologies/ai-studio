package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// POST /model-prices for a model traffic has already seen fills in the
// unpriced row the gateway auto-created; a POST for a model with a real
// price is a 409 naming that row. Both used to be a 500 unique violation.
func TestCreateModelPrice_FillsUnpricedRowAndConflictsOnPricedRow(t *testing.T) {
	api, db := setupTestAPI(t)

	// What the gateway does on the first request for a model.
	auto, err := api.service.GetModelPriceByModelNameAndVendor("gemini-2.5-flash", string(models.GOOGLEAI))
	require.NoError(t, err)

	post := func(cpit float64) (int, map[string]any) {
		body := map[string]any{"data": map[string]any{"type": "ModelPrice", "attributes": map[string]any{
			"model_name": "gemini-2.5-flash", "vendor": string(models.GOOGLEAI),
			"cpt": 0.0000025, "cpit": cpit, "currency": "EUR",
		}}}
		w := performRequest(api.router, http.MethodPost, "/api/v1/model-prices", body)
		var out map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return w.Code, out
	}

	code, out := post(0.0000003)
	assert.Equal(t, http.StatusOK, code, "the unpriced row is filled in")
	assert.Equal(t, fmt.Sprint(auto.ID), out["data"].(map[string]any)["id"])
	var got models.ModelPrice
	require.NoError(t, db.First(&got, auto.ID).Error)
	assert.Equal(t, 0.0000003, got.CPIT)
	assert.Equal(t, "EUR", got.Currency)
	var n int64
	require.NoError(t, db.Model(&models.ModelPrice{}).Where("model_name = ?", "gemini-2.5-flash").Count(&n).Error)
	assert.Equal(t, int64(1), n)

	code, out = post(0.0000009)
	assert.Equal(t, http.StatusConflict, code)
	detail := out["errors"].([]any)[0].(map[string]any)["detail"].(string)
	assert.Contains(t, detail, fmt.Sprintf("model price %d", auto.ID))
	require.NoError(t, db.First(&got, auto.ID).Error)
	assert.Equal(t, 0.0000003, got.CPIT, "a conflicting POST leaves the price alone")

	code, _ = post(0.0000003)
	assert.Equal(t, http.StatusConflict, code, "an identical POST is a conflict too")

	// A model nobody has seen is created as before.
	body := map[string]any{"data": map[string]any{"type": "ModelPrice", "attributes": map[string]any{
		"model_name": "gemini-new", "vendor": string(models.GOOGLEAI), "cpt": 0.000001, "cpit": 0.000001, "currency": "USD",
	}}}
	assert.Equal(t, http.StatusCreated, performRequest(api.router, http.MethodPost, "/api/v1/model-prices", body).Code)
}
