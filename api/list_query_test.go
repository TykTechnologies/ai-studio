package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseListQuery(t *testing.T) {
	parse := func(query string) (*httptest.ResponseRecorder, bool, string, bool, string) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/v1/llms?"+query, nil)
		opts, ok := parseListQuery(c, llmSortFields)
		return w, ok, opts.Sort, opts.SortDesc, opts.Search
	}

	_, ok, sort, desc, search := parse("")
	assert.True(t, ok)
	assert.Equal(t, "", sort, "no sort means the default order")
	assert.False(t, desc)
	assert.Equal(t, "", search)

	_, ok, sort, desc, search = parse("search=%20mock%20&sort=-name")
	assert.True(t, ok)
	assert.Equal(t, "name", sort)
	assert.True(t, desc)
	assert.Equal(t, "mock", search, "search is trimmed")

	_, ok, sort, desc, _ = parse("sort=privacy_score")
	assert.True(t, ok)
	assert.Equal(t, "privacy_score", sort)
	assert.False(t, desc)

	w, ok, _, _, _ := parse("sort=bogus")
	assert.False(t, ok)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Errors, 1)
	assert.Equal(t, "Bad Request", resp.Errors[0].Title)
	assert.Equal(t, "unsupported sort field: bogus", resp.Errors[0].Detail)

	w, ok, _, _, _ = parse("sort=-var_name")
	assert.False(t, ok, "a column from another type's whitelist is rejected")
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// An SQL fragment never reaches the database.
	w, ok, _, _, _ = parse("sort=name%3B%20DROP%20TABLE%20llms")
	assert.False(t, ok)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSortFieldAliases(t *testing.T) {
	assert.Equal(t, "model_name", modelPriceSortFields["name"], "prices accept name for model_name")
	assert.Equal(t, "var_name", secretSortFields["name"], "secrets accept name for var_name")
	_, ok := filterSortFields["active"]
	assert.False(t, ok, "filters have no live flag to sort by")
	_, ok = llmSortFields["vendor"]
	assert.True(t, ok)
}

// GET /llms end to end: search narrows the set and the headers, sort orders
// the page, and the pre-existing parameters keep working alongside them.
func TestListLLMs_SearchAndSort(t *testing.T) {
	api, db := setupTestAPI(t)
	for _, l := range []models.LLM{
		{Name: "mock-beta", Vendor: models.MOCK_VENDOR, PrivacyScore: 20},
		{Name: "OpenAI GPT", Vendor: models.OPENAI, PrivacyScore: 80, ShortDescription: "flagship"},
		{Name: "mock-alpha", Vendor: models.MOCK_VENDOR, PrivacyScore: 40},
	} {
		require.NoError(t, db.Create(&l).Error)
	}

	list := func(t *testing.T, query string) (*httptest.ResponseRecorder, []string) {
		t.Helper()
		w := performRequest(api.router, "GET", "/api/v1/llms?"+query, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var body struct {
			Data []struct {
				Attributes struct {
					Name string `json:"name"`
				} `json:"attributes"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		names := []string{}
		for _, d := range body.Data {
			names = append(names, d.Attributes.Name)
		}
		return w, names
	}

	w, names := list(t, "")
	assert.Equal(t, []string{"mock-beta", "OpenAI GPT", "mock-alpha"}, names, "default order is id ascending")
	assert.Equal(t, "3", w.Header().Get("X-Total-Count"))
	assert.Equal(t, "1", w.Header().Get("X-Total-Pages"))

	w, names = list(t, "search=MOCK&sort=-name")
	assert.Equal(t, []string{"mock-beta", "mock-alpha"}, names)
	assert.Equal(t, "2", w.Header().Get("X-Total-Count"), "the count is of the filtered set")

	_, names = list(t, "search=flagship")
	assert.Equal(t, []string{"OpenAI GPT"}, names, "description fields are searched")

	_, names = list(t, "sort=privacy_score")
	assert.Equal(t, []string{"mock-beta", "mock-alpha", "OpenAI GPT"}, names)

	w, names = list(t, "search=mock&sort=name&page_size=1&page=2")
	assert.Equal(t, []string{"mock-beta"}, names, "pagination applies after search and sort")
	assert.Equal(t, "2", w.Header().Get("X-Total-Count"))
	assert.Equal(t, "2", w.Header().Get("X-Total-Pages"))

	w = performRequest(api.router, "GET", "/api/v1/llms?sort=bogus", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "unsupported sort field: bogus")
}

// Every other list accepts the same parameters; a smoke pass over each so a
// handler that was missed (or a whitelist that names a missing column,
// which would be a 500) shows up here.
func TestAllLists_AcceptSearchAndSort(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")
	api, db := setupTestAPI(t)
	require.NoError(t, db.Create(&models.Tool{Name: "Weather", Description: "forecast"}).Error)
	require.NoError(t, db.Create(&models.Datasource{Name: "Docs", ShortDescription: "wiki"}).Error)
	require.NoError(t, db.Create(&models.Filter{Name: "PII", Description: "redact"}).Error)
	require.NoError(t, db.Create(&models.ModelPrice{ModelName: "gpt-4", Vendor: "openai"}).Error)
	require.NoError(t, db.Create(&models.Catalogue{Name: "LLM cat"}).Error)
	require.NoError(t, db.Create(&models.DataCatalogue{Name: "Data cat"}).Error)
	require.NoError(t, db.Create(&models.ToolCatalogue{Name: "Tool cat"}).Error)

	for _, tc := range []struct {
		path, search, sort, miss string
	}{
		{"/api/v1/tools", "FORECAST", "-privacy_score", "vendor"},
		{"/api/v1/datasources", "wiki", "-active", "vendor"},
		{"/api/v1/filters", "REDACT", "-name", "active"},
		{"/api/v1/model-prices", "OPENAI", "-name", "privacy_score"},
		{"/api/v1/catalogues", "llm", "-name", "active"},
		{"/api/v1/data-catalogues", "DATA", "-created_at", "active"},
		{"/api/v1/tool-catalogues", "tool", "-updated_at", "active"},
		{"/api/v1/secrets", "nothing-here", "-name", "active"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := performRequest(api.router, "GET", tc.path+"?search="+tc.search+"&sort="+tc.sort, nil)
			if w.Code == http.StatusPaymentRequired {
				t.Skip("list is Enterprise-gated in this build")
			}
			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.NotEmpty(t, w.Header().Get("X-Total-Count"))
			if tc.path != "/api/v1/secrets" {
				assert.Equal(t, "1", w.Header().Get("X-Total-Count"), "search matches the one row")
			}

			w = performRequest(api.router, "GET", tc.path+"?search=zzz-no-match", nil)
			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, "0", w.Header().Get("X-Total-Count"))

			w = performRequest(api.router, "GET", tc.path+"?sort="+tc.miss, nil)
			assert.Equal(t, http.StatusBadRequest, w.Code, "%s is not sortable by %s", tc.path, tc.miss)
		})
	}
}
