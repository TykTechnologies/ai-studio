package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The portal's unified catalog must show exactly what the per-catalog pages
// showed, and nothing more: active objects in catalogs attached to one of
// the caller's teams. These tests build two teams with different catalogs
// and check that the second team's objects, inactive objects and objects in
// no catalog never reach the first user.

func portalGet(t *testing.T, handler gin.HandlerFunc, user *models.User, params ...gin.Param) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("user", user)
	c.Params = params
	handler(c)
	return w
}

func idOf(id uint) string { return fmt.Sprintf("%d", id) }

// giveUserTeam puts the user in a new team that owns the given catalogs.
func giveUserTeam(t *testing.T, service interface {
	CreateGroup(name string, userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs []uint) (*models.Group, error)
}, name string, userID uint, llmCats, dataCats, toolCats []uint) {
	t.Helper()
	_, err := service.CreateGroup(name, []uint{userID}, llmCats, dataCats, toolCats)
	require.NoError(t, err)
}

func TestPortalCatalog_RespectsVisibility(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)

	user := createTestUser(t, service)
	other := createTestUserWithSettings(t, service, "other@example.com", "Other", false, true, true, true, false)

	// user's team: one LLM catalog, one data catalog, one tool catalog.
	llmCat := createTestCatalogue(t, service)
	dataCat := createTestDataCatalogue(t, service)
	toolCat := createTestToolCatalogue(t, service)
	giveUserTeam(t, service, "Platform", user.ID, []uint{llmCat.ID}, []uint{dataCat.ID}, []uint{toolCat.ID})

	// other's team: a second LLM catalog the first user is not in.
	otherCat, err := service.CreateCatalogue("Other Catalogue")
	require.NoError(t, err)
	giveUserTeam(t, service, "Marketing", other.ID, []uint{otherCat.ID}, nil, nil)

	visibleLLM := createTestLLM(t, service, "Visible LLM")
	require.NoError(t, service.AddLLMToCatalogue(visibleLLM.ID, llmCat.ID))
	draftLLM, err := service.CreateLLM("Draft LLM", "api_key", "https://api.example.com",
		80, "Not yet approved", "Long desc", "", models.OPENAI, false, nil, "", []string{}, nil, nil, false, nil, nil)
	require.NoError(t, err)
	require.NoError(t, service.AddLLMToCatalogue(draftLLM.ID, llmCat.ID))
	hiddenLLM := createTestLLM(t, service, "Other Team LLM")
	require.NoError(t, service.AddLLMToCatalogue(hiddenLLM.ID, otherCat.ID))
	// In no catalog at all (Enterprise no longer auto-adds to Default).
	createTestLLM(t, service, "Uncatalogued LLM")

	visibleDS := createTestDatasource(t, service, "Visible DS")
	require.NoError(t, service.AddDatasourceToDataCatalogue(dataCat.ID, visibleDS.ID))
	createTestDatasource(t, service, "Uncatalogued DS")

	visibleTool := createTestTool(t, service, "Visible Tool")
	require.NoError(t, service.AddToolToToolCatalogue(visibleTool.ID, toolCat.ID))
	offTool := createTestTool(t, service, "Switched Off Tool")
	require.NoError(t, service.AddToolToToolCatalogue(offTool.ID, toolCat.ID))
	require.NoError(t, db.Model(&models.Tool{}).Where("id = ?", offTool.ID).Update("active", false).Error)

	w := portalGet(t, api.getPortalCatalog, user)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var response CatalogListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	names := map[string]string{}
	for _, item := range response.Data {
		names[item.Type+":"+item.Attributes.Name] = item.ID
	}
	assert.Equal(t, map[string]string{
		"llm:Visible LLM":       idOf(visibleLLM.ID),
		"datasource:Visible DS": idOf(visibleDS.ID),
		"tool:Visible Tool":     idOf(visibleTool.ID),
	}, names)

	assert.Equal(t, 3, response.Meta.Total)
	assert.Equal(t, map[string]int{"llm": 1, "datasource": 1, "tool": 1, "mcp_server": 0, "model_router": 0, "semantic_router": 0, "plugin_resource": 0}, response.Meta.Counts)

	// Each item names the catalogs it is reachable through, and the filter
	// options list only the caller's catalogs.
	for _, item := range response.Data {
		require.Len(t, item.Attributes.Catalogs, 1, item.Attributes.Name)
		assert.NotNil(t, item.Attributes.PrivacyScore)
		assert.NotNil(t, item.Attributes.CreatedAt)
	}
	catalogNames := []string{}
	for _, option := range response.Meta.Catalogs {
		catalogNames = append(catalogNames, option.Type+":"+option.Name)
	}
	assert.ElementsMatch(t, []string{"llm:Test Catalogue", "datasource:Test Data Catalogue", "tool:Test Tool Catalogue"}, catalogNames)

	// The other user sees only their own team's LLM.
	w = portalGet(t, api.getPortalCatalog, other)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data, 1)
	assert.Equal(t, "Other Team LLM", response.Data[0].Attributes.Name)
}

func TestPortalCatalog_DetailEndpoints(t *testing.T) {
	api, _, service := setupTestAPIForCommonTests(t)

	user := createTestUser(t, service)
	llmCat := createTestCatalogue(t, service)
	dataCat := createTestDataCatalogue(t, service)
	toolCat := createTestToolCatalogue(t, service)
	giveUserTeam(t, service, "Platform", user.ID, []uint{llmCat.ID}, []uint{dataCat.ID}, []uint{toolCat.ID})

	llm, err := service.CreateLLM("Acme OpenAI", "api_key", "https://api.example.com",
		40, "Fast general model", "Long desc", "", models.OPENAI, true, nil,
		"gpt-4o", []string{"gpt-4o", "^gpt-4o-mini$"}, nil, nil, false,
		models.JSONMap{"region": "eu", "api_token": "plain-secret"}, nil)
	require.NoError(t, err)
	require.NoError(t, service.AddLLMToCatalogue(llm.ID, llmCat.ID))
	hidden := createTestLLM(t, service, "Hidden LLM")

	ds := createTestDatasource(t, service, "Visible DS")
	require.NoError(t, service.AddDatasourceToDataCatalogue(dataCat.ID, ds.ID))
	tool := createTestTool(t, service, "Visible Tool")
	require.NoError(t, service.AddToolToToolCatalogue(tool.ID, toolCat.ID))

	// Price table for the vendor: the allow list admits gpt-4o and
	// gpt-4o-mini, not o1. Prices are stored per token; the catalog reports
	// them per million tokens.
	const (
		gpt4oOutputPerToken = 0.00001
		gpt4oInputPerToken  = 0.0000025
		perMillion          = 1_000_000
	)
	_, err = service.CreateModelPrice("gpt-4o", "openai", gpt4oOutputPerToken, gpt4oInputPerToken, 0, 0, "USD")
	require.NoError(t, err)
	_, err = service.CreateModelPrice("gpt-4o-mini", "openai", 0.0000006, 0.00000015, 0, 0, "USD")
	require.NoError(t, err)
	_, err = service.CreateModelPrice("o1", "openai", 0.00006, 0.000015, 0, 0, "USD")
	require.NoError(t, err)

	t.Run("LLM detail lists models with prices and redacts metadata secrets", func(t *testing.T) {
		w := portalGet(t, api.getPortalCatalogLLM, user, gin.Param{Key: "id", Value: idOf(llm.ID)})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var response CatalogItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		attrs := response.Data.Attributes
		assert.Equal(t, "Acme OpenAI", attrs.Name)
		assert.Equal(t, "openai", attrs.Kind)
		assert.Equal(t, []string{"gpt-4o", "^gpt-4o-mini$"}, attrs.AllowedModels)

		require.Len(t, attrs.Models, 2)
		assert.Equal(t, "gpt-4o", attrs.Models[0].Name)
		assert.True(t, attrs.Models[0].IsDefault)
		require.NotNil(t, attrs.Models[0].InputPricePerMillion)
		assert.InDelta(t, gpt4oInputPerToken*perMillion, *attrs.Models[0].InputPricePerMillion, 0.0001)
		assert.InDelta(t, gpt4oOutputPerToken*perMillion, *attrs.Models[0].OutputPricePerMillion, 0.0001)
		assert.Equal(t, "gpt-4o-mini", attrs.Models[1].Name)
		assert.False(t, attrs.Models[1].IsDefault)

		assert.Equal(t, "eu", attrs.Metadata["region"])
		assert.NotEqual(t, "plain-secret", attrs.Metadata["api_token"])
	})

	t.Run("LLM outside the caller's catalogs is not found", func(t *testing.T) {
		w := portalGet(t, api.getPortalCatalogLLM, user, gin.Param{Key: "id", Value: idOf(hidden.ID)})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("data source and tool detail", func(t *testing.T) {
		w := portalGet(t, api.getPortalCatalogDatasource, user, gin.Param{Key: "id", Value: idOf(ds.ID)})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var response CatalogItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, "datasource", response.Data.Type)
		assert.Equal(t, "source_type", response.Data.Attributes.Kind)
		assert.Equal(t, "embed_model", response.Data.Attributes.EmbedModel)

		w = portalGet(t, api.getPortalCatalogTool, user, gin.Param{Key: "id", Value: idOf(tool.ID)})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, "tool", response.Data.Type)
		assert.Equal(t, models.ToolTypeREST, response.Data.Attributes.Kind)

		w = portalGet(t, api.getPortalCatalogTool, user, gin.Param{Key: "id", Value: "999999"})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestCatalogModels_AllowListAndPrices(t *testing.T) {
	llm := &models.LLM{DefaultModel: "claude-3-5-sonnet", AllowedModels: []string{"claude-3.*"}}
	prices := models.ModelPrices{
		{ModelName: "claude-3-5-sonnet", Vendor: "anthropic", CPIT: 0.000003, CPT: 0.000015, Currency: "USD"},
		{ModelName: "claude-3-haiku", Vendor: "anthropic", CPIT: 0.00000025, CPT: 0.00000125, Currency: "USD"},
		{ModelName: "claude-2", Vendor: "anthropic", CPIT: 0.000008, CPT: 0.000024, Currency: "USD"},
	}
	got := catalogModels(llm, prices)
	names := []string{}
	for _, m := range got {
		names = append(names, m.Name)
	}
	// Default first, then priced models the pattern admits, alphabetically;
	// the regex pattern itself is not listed as a model.
	assert.Equal(t, []string{"claude-3-5-sonnet", "claude-3-haiku"}, names)
	assert.True(t, got[0].IsDefault)
	require.NotNil(t, got[1].InputPricePerMillion)
	assert.InDelta(t, 0.25, *got[1].InputPricePerMillion, 0.0001)

	// No allow list: every priced model is offered.
	open := catalogModels(&models.LLM{DefaultModel: "claude-2"}, prices)
	assert.Len(t, open, 3)
	assert.Equal(t, "claude-2", open[0].Name)
}

func TestUserAppsUsageSummary(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)
	// Chat records live outside InitModels (the analytics writer migrates them).
	require.NoError(t, db.AutoMigrate(&models.LLMChatRecord{}))

	user := createTestUser(t, service)
	other := createTestUserWithSettings(t, service, "other@example.com", "Other", false, true, true, true, false)
	llmCat := createTestCatalogue(t, service)
	addCatalogueToUserGroup(t, service, user.ID, llmCat.ID)
	llm := createTestLLM(t, service, "LLM")
	require.NoError(t, service.AddLLMToCatalogue(llm.ID, llmCat.ID))

	// A budget window that started a week ago, so which records count
	// toward spend does not depend on today's date.
	monthlyBudget := 100.0
	now := time.Now()
	budgetStart := now.AddDate(0, 0, -7)
	mine, err := service.CreateApp("Mine", "", user.ID, nil, []uint{llm.ID}, nil, &monthlyBudget, &budgetStart, nil)
	require.NoError(t, err)
	quiet, err := service.CreateApp("Quiet", "", user.ID, nil, []uint{llm.ID}, nil, nil, nil, nil)
	require.NoError(t, err)
	theirs, err := service.CreateApp("Theirs", "", other.ID, nil, []uint{llm.ID}, nil, nil, nil, nil)
	require.NoError(t, err)

	// The same table the app page's charts and the budget spend read. Cost
	// is stored x10000, so 5000 is fifty cents.
	records := []models.LLMChatRecord{
		{AppID: mine.ID, LLMID: llm.ID, TimeStamp: now.Add(-48 * time.Hour), TotalTokens: 10, Cost: 5000},
		{AppID: mine.ID, LLMID: llm.ID, TimeStamp: now.Add(-1 * time.Hour), TotalTokens: 10, Cost: 5000},
		// Before the budget window but inside 30 days: a request, not spend.
		{AppID: mine.ID, LLMID: llm.ID, TimeStamp: now.AddDate(0, 0, -10), TotalTokens: 10, Cost: 5000},
		// Older than the window: counts for last access, not for requests_30d.
		{AppID: mine.ID, LLMID: llm.ID, TimeStamp: now.Add(-45 * 24 * time.Hour), TotalTokens: 10, Cost: 5000},
		{AppID: theirs.ID, LLMID: llm.ID, TimeStamp: now.Add(-1 * time.Hour), TotalTokens: 10, Cost: 5000},
	}
	for i := range records {
		require.NoError(t, db.Create(&records[i]).Error)
	}

	w := portalGet(t, api.getUserAppsUsageSummary, user)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var response AppUsageSummaryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data, 2)
	_, leaked := response.Data[idOf(theirs.ID)]
	assert.False(t, leaked, "another user's app must not be summarised")

	active := response.Data[idOf(mine.ID)]
	require.NotNil(t, active.LastAccessAt)
	assert.WithinDuration(t, now.Add(-1*time.Hour), *active.LastAccessAt, time.Minute)
	assert.Equal(t, int64(3), active.Requests30d)
	require.NotNil(t, active.MonthlyBudget)
	assert.Equal(t, 100.0, *active.MonthlyBudget)
	require.NotNil(t, active.Percentage)
	if budget.IsEnterpriseAvailable() {
		// Two records inside the budget window: $1.00 of $100.
		assert.True(t, response.SpendTracked)
		assert.InDelta(t, 1.0, active.CurrentSpend, 0.0001)
		assert.InDelta(t, 1.0, *active.Percentage, 0.0001)
	} else {
		assert.False(t, response.SpendTracked)
		assert.Equal(t, 0.0, active.CurrentSpend)
	}

	idle := response.Data[idOf(quiet.ID)]
	assert.Nil(t, idle.LastAccessAt)
	assert.Equal(t, int64(0), idle.Requests30d)
	assert.Nil(t, idle.MonthlyBudget)
	assert.Nil(t, idle.Percentage)
}

// The portal budget endpoint answered for any app id; it must refuse apps
// the caller does not own unless they may read analytics.
func TestBudgetUsageForApp_OwnershipOnPortalRoute(t *testing.T) {
	api, _, service := setupTestAPIForCommonTests(t)

	owner := createTestUser(t, service)
	stranger := createTestUserWithSettings(t, service, "stranger@example.com", "Stranger", false, true, true, true, false)
	admin := createTestUserWithSettings(t, service, "admin@example.com", "Admin", true, true, true, true, false)
	llmCat := createTestCatalogue(t, service)
	addCatalogueToUserGroup(t, service, owner.ID, llmCat.ID)
	llm := createTestLLM(t, service, "LLM")
	require.NoError(t, service.AddLLMToCatalogue(llm.ID, llmCat.ID))
	app, err := service.CreateApp("Mine", "", owner.ID, nil, []uint{llm.ID}, nil, nil, nil, nil)
	require.NoError(t, err)

	call := func(user *models.User) int {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/analytics/budget-usage-for-app?app_id=%d", app.ID), nil)
		c.Set("user", user)
		api.getBudgetUsageForApp(c)
		return w.Code
	}

	assert.Equal(t, http.StatusOK, call(owner))
	assert.Equal(t, http.StatusForbidden, call(stranger))
	assert.Equal(t, http.StatusOK, call(admin))
}

func portalGetQuery(t *testing.T, handler gin.HandlerFunc, user *models.User, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil)
	c.Set("user", user)
	handler(c)
	return w
}

// Search, filters, sort and paging happen on the server: the client only
// ever receives one page, and the facets describe the whole accessible set.
func TestPortalCatalog_QuerySortAndPage(t *testing.T) {
	api, _, service := setupTestAPIForCommonTests(t)

	user := createTestUser(t, service)
	llmCat := createTestCatalogue(t, service)
	toolCat := createTestToolCatalogue(t, service)
	giveUserTeam(t, service, "Platform", user.ID, []uint{llmCat.ID}, nil, []uint{toolCat.ID})

	mk := func(name string, vendor models.Vendor, privacy int, model string) {
		llm, err := service.CreateLLM(name, "api_key", "https://api.example.com",
			privacy, name+" short", "", "", vendor, true, nil, model, []string{}, nil, nil, false, nil, nil)
		require.NoError(t, err)
		require.NoError(t, service.AddLLMToCatalogue(llm.ID, llmCat.ID))
	}
	mk("Alpha OpenAI", models.OPENAI, 10, "gpt-4o")
	mk("Bravo Bedrock", models.BEDROCK, 60, "claude-3")
	mk("Charlie OpenAI", models.OPENAI, 90, "gpt-4o-mini")
	tool := createTestTool(t, service, "Delta Tool")
	require.NoError(t, service.AddToolToToolCatalogue(tool.ID, toolCat.ID))

	list := func(rawQuery string) CatalogListResponse {
		w := portalGetQuery(t, api.getPortalCatalog, user, rawQuery)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var response CatalogListResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		return response
	}
	names := func(r CatalogListResponse) []string {
		out := []string{}
		for _, item := range r.Data {
			out = append(out, item.Attributes.Name)
		}
		return out
	}

	t.Run("default is every item newest first with facets", func(t *testing.T) {
		r := list("")
		assert.Equal(t, 4, r.Meta.Total)
		assert.Equal(t, 1, r.Meta.Page)
		assert.Equal(t, catalogDefaultPageSize, r.Meta.PageSize)
		assert.Equal(t, 1, r.Meta.TotalPages)
		assert.Equal(t, map[string]int{"llm": 3, "datasource": 0, "tool": 1, "mcp_server": 0, "model_router": 0, "semantic_router": 0, "plugin_resource": 0}, r.Meta.Counts)
		kinds := []string{}
		for _, k := range r.Meta.Kinds {
			kinds = append(kinds, k.Type+":"+k.Kind+"="+strconv.Itoa(k.Count))
		}
		assert.Equal(t, []string{"llm:bedrock=1", "llm:openai=2", "tool:" + models.ToolTypeREST + "=1"}, kinds)
		// Created in this order, so newest first reverses it.
		assert.Equal(t, []string{"Delta Tool", "Charlie OpenAI", "Bravo Bedrock", "Alpha OpenAI"}, names(r))
	})

	t.Run("search matches names, vendor labels and model names", func(t *testing.T) {
		assert.Equal(t, []string{"Bravo Bedrock"}, names(list("q=aws")))
		assert.Equal(t, []string{"Charlie OpenAI", "Alpha OpenAI"}, names(list("q=gpt-4o")))
		assert.Equal(t, []string{"Alpha OpenAI"}, names(list("q=openai+alpha")))
		assert.Empty(t, names(list("q=nothing+here")))
	})

	t.Run("filters narrow the page but not the facets", func(t *testing.T) {
		r := list("type=llm&kind=openai")
		assert.Equal(t, 2, r.Meta.Total)
		assert.Equal(t, 3, r.Meta.Counts["llm"])
		assert.Len(t, r.Meta.Kinds, 3)
		assert.Equal(t, []string{"Charlie OpenAI"}, names(list("privacy=restricted")))
		assert.Equal(t, []string{"Delta Tool"}, names(list("catalog=tool:"+idOf(toolCat.ID))))
		assert.Empty(t, names(list("community=true")))
	})

	t.Run("sorts by name and privacy", func(t *testing.T) {
		assert.Equal(t, []string{"Alpha OpenAI", "Bravo Bedrock", "Charlie OpenAI", "Delta Tool"}, names(list("sort=name")))
		assert.Equal(t, []string{"Delta Tool", "Alpha OpenAI", "Bravo Bedrock", "Charlie OpenAI"}, names(list("sort=privacy_asc")))
		assert.Equal(t, []string{"Charlie OpenAI", "Bravo Bedrock", "Alpha OpenAI", "Delta Tool"}, names(list("sort=privacy_desc")))
	})

	t.Run("pages", func(t *testing.T) {
		r := list("sort=name&page_size=3")
		assert.Equal(t, []string{"Alpha OpenAI", "Bravo Bedrock", "Charlie OpenAI"}, names(r))
		assert.Equal(t, 4, r.Meta.Total)
		assert.Equal(t, 2, r.Meta.TotalPages)
		r = list("sort=name&page_size=3&page=2")
		assert.Equal(t, []string{"Delta Tool"}, names(r))
		assert.Equal(t, 2, r.Meta.Page)
		r = list("sort=name&page_size=3&page=9")
		assert.Empty(t, r.Data)
		assert.Equal(t, 2, r.Meta.TotalPages)
	})

	t.Run("rejects values outside the vocabulary", func(t *testing.T) {
		for _, raw := range []string{"sort=random", "privacy=secret", "type=widget", "page=0", "page_size=x"} {
			w := portalGetQuery(t, api.getPortalCatalog, user, raw)
			assert.Equal(t, http.StatusBadRequest, w.Code, raw)
		}
		// Oversized pages are clamped, not refused.
		assert.Equal(t, catalogMaxPageSize, list("page_size=5000").Meta.PageSize)
	})
}

// Names and descriptions are stripped of markup with the HTML tokenizer:
// tags (however malformed) go, text stays exactly as typed.
func TestCleanText(t *testing.T) {
	cases := map[string]string{
		"Acme OpenAI":                              "Acme OpenAI",
		"Martin's GPT & R&D":                       "Martin's GPT & R&D",
		"<script>alert(1)</script>Acme":            "alert(1)Acme",
		"<scr<script>ipt>alert(1)</script>":        "ipt>alert(1)",
		"<img src=x onerror=alert(1)>Vision":       "Vision",
		"a <!-- comment --> b":                     "a  b",
		"&lt;b&gt;literal&lt;/b&gt;":               "&lt;b&gt;literal&lt;/b&gt;",
		"1 < 2 and 3 > 2":                          "1 < 2 and 3 > 2",
		"<a href=\"javascript:alert(1)\">link</a>": "link",
	}
	for in, want := range cases {
		assert.Equal(t, want, cleanText(in), in)
	}
	assert.Equal(t, []string{"gpt-4o", "x"}, cleanTexts([]string{"gpt-4o", "<b>x</b>"}))
	assert.Equal(t, []string{}, cleanTexts(nil))
}
