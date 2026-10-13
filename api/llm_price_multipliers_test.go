package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

func llmPriceMultipliersOf(t *testing.T, body []byte) models.LLMPriceMultipliers {
	t.Helper()
	var resp struct {
		Data LLMResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp.Data.Attributes.PriceMultipliers
}

// price_multipliers is set on create, kept by a PATCH that omits it,
// replaced by a PATCH that sends it, cleared by null or {}, and validated.
func TestLLMPriceMultipliersAttribute(t *testing.T) {
	api, db := setupTestAPI(t)
	body := map[string]any{"data": map[string]any{"type": "llms", "attributes": map[string]any{
		"name": "Claude", "vendor": "anthropic", "api_endpoint": "https://api.anthropic.com",
		"privacy_score": 5, "price_multipliers": map[string]float64{"batch": 0.4},
	}}}
	w := performRequest(api.router, http.MethodPost, "/api/v1/llms", body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, models.LLMPriceMultipliers{"batch": 0.4}, llmPriceMultipliersOf(t, w.Body.Bytes()))
	var stored models.LLM
	require.NoError(t, db.Where("name = ?", "Claude").First(&stored).Error)
	assert.Equal(t, models.LLMPriceMultipliers{"batch": 0.4}, stored.PriceMultipliers)
	path := "/api/v1/llms/" + idStr(stored.ID)

	patch := func(attrs map[string]any) (int, []byte) {
		w := performRequest(api.router, http.MethodPatch, path, map[string]any{"data": map[string]any{"type": "llms", "attributes": attrs}})
		return w.Code, w.Body.Bytes()
	}
	code, out := patch(map[string]any{"short_description": "edited"})
	require.Equal(t, http.StatusOK, code, string(out))
	assert.Equal(t, models.LLMPriceMultipliers{"batch": 0.4}, llmPriceMultipliersOf(t, out), "a PATCH that omits it keeps the overrides")

	code, out = patch(map[string]any{"price_multipliers": map[string]float64{"priority": 1.5}})
	require.Equal(t, http.StatusOK, code, string(out))
	assert.Equal(t, models.LLMPriceMultipliers{"priority": 1.5}, llmPriceMultipliersOf(t, out), "the map is replaced, not merged")

	for _, bad := range []any{
		map[string]float64{"batch": -1},
		map[string]float64{"batch": 11},
		map[string]float64{"Batch": 0.5},
		map[string]string{"batch": "half"},
		[]float64{0.5},
	} {
		code, out = patch(map[string]any{"price_multipliers": bad})
		assert.Equal(t, http.StatusBadRequest, code, string(out))
	}

	code, out = patch(map[string]any{"price_multipliers": nil})
	require.Equal(t, http.StatusOK, code, string(out))
	assert.Empty(t, llmPriceMultipliersOf(t, out))
	require.NoError(t, db.First(&stored, stored.ID).Error)

	code, out = patch(map[string]any{"price_multipliers": map[string]float64{"batch": 0.3}})
	require.Equal(t, http.StatusOK, code, string(out))
	code, out = patch(map[string]any{"price_multipliers": map[string]float64{}})
	require.Equal(t, http.StatusOK, code, string(out))
	assert.Empty(t, llmPriceMultipliersOf(t, out))
	var raw *string
	require.NoError(t, db.Raw("SELECT price_multipliers FROM llms WHERE id = ?", stored.ID).Scan(&raw).Error)
	assert.Nil(t, raw, "{} is stored as NULL")
}
