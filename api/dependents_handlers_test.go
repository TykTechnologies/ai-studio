package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func idStr(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

// The dependents endpoints answer "what breaks if I delete this" for the
// delete confirmation. Every array is always present and total is their sum.
func TestDependentsEndpoints(t *testing.T) {
	api, db := setupTestAPI(t)

	llm := &models.LLM{Name: "Shared LLM", Active: true}
	require.NoError(t, db.Create(llm).Error)
	app := &models.App{Name: "Billing Copilot", UserID: 1}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Model(app).Association("LLMs").Append(llm))
	catalogue := &models.Catalogue{Name: "Platform LLMs"}
	require.NoError(t, db.Create(catalogue).Error)
	require.NoError(t, db.Model(catalogue).Association("LLMs").Append(llm))

	tool := &models.Tool{Name: "Weather", Active: true}
	require.NoError(t, db.Create(tool).Error)
	require.NoError(t, db.Model(app).Association("Tools").Append(tool))

	ds := &models.Datasource{Name: "Docs", Active: true}
	require.NoError(t, db.Create(ds).Error)

	filter := &models.Filter{Name: "PII"}
	require.NoError(t, db.Create(filter).Error)
	require.NoError(t, db.Model(llm).Association("Filters").Append(filter))

	router := &models.ModelRouter{Name: "Router", Slug: "router"}
	require.NoError(t, db.Create(router).Error)

	get := func(t *testing.T, path string) DependentsResponse {
		t.Helper()
		w := performRequest(api.router, "GET", path, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		// The contract: every array present, even when empty.
		var raw map[string]map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
		attrs := map[string]json.RawMessage{}
		require.NoError(t, json.Unmarshal(raw["data"]["attributes"], &attrs))
		for _, key := range []string{"apps", "catalogues", "llms", "tools", "datasources", "agents", "model_routers", "total"} {
			assert.Contains(t, attrs, key)
		}

		var response DependentsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, "dependents", response.Data.Type)
		return response
	}

	t.Run("llm", func(t *testing.T) {
		response := get(t, "/api/v1/llms/"+idStr(llm.ID)+"/dependents")
		assert.Equal(t, idStr(llm.ID), response.Data.ID)
		require.Len(t, response.Data.Attributes.Apps, 1)
		assert.Equal(t, "Billing Copilot", response.Data.Attributes.Apps[0].Name)
		require.Len(t, response.Data.Attributes.Catalogues, 1)
		assert.Equal(t, "Platform LLMs", response.Data.Attributes.Catalogues[0].Name)
		assert.Equal(t, 2, response.Data.Attributes.Total)
	})

	t.Run("tool", func(t *testing.T) {
		response := get(t, "/api/v1/tools/"+idStr(tool.ID)+"/dependents")
		require.Len(t, response.Data.Attributes.Apps, 1)
		assert.Equal(t, 1, response.Data.Attributes.Total)
	})

	t.Run("datasource", func(t *testing.T) {
		response := get(t, "/api/v1/datasources/"+idStr(ds.ID)+"/dependents")
		assert.Equal(t, 0, response.Data.Attributes.Total)
	})

	t.Run("filter", func(t *testing.T) {
		response := get(t, "/api/v1/filters/"+idStr(filter.ID)+"/dependents")
		require.Len(t, response.Data.Attributes.LLMs, 1)
		assert.Equal(t, "Shared LLM", response.Data.Attributes.LLMs[0].Name)
		assert.Equal(t, 1, response.Data.Attributes.Total)
	})

	t.Run("model router", func(t *testing.T) {
		response := get(t, "/api/v1/model-routers/"+idStr(router.ID)+"/dependents")
		assert.Equal(t, 0, response.Data.Attributes.Total)
	})

	t.Run("unknown object is 404", func(t *testing.T) {
		w := performRequest(api.router, "GET", "/api/v1/llms/99999/dependents", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("bad id is 400", func(t *testing.T) {
		w := performRequest(api.router, "GET", "/api/v1/tools/not-a-number/dependents", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
