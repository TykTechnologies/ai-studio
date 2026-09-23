package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A Semantic Router is a portal asset like a Model Router: published in LLM
// catalogues, granted to Apps, never showing its examples.

func createPortalSemanticRouter(t *testing.T, db *gorm.DB, name, slug string, target *models.LLM) *models.SemanticRouter {
	t.Helper()
	r := &models.SemanticRouter{Name: name, Slug: slug, Active: true, ShortDescription: "Routes by meaning",
		Settings: sr.Settings{DefaultRoute: "simple", AllowExplicitRoute: true},
		Routes: []sr.Route{
			{Name: "complex", Description: "Hard reasoning", Keywords: []sr.Keyword{{Pattern: "secret-keyword"}},
				Target: sr.Target{Type: sr.TargetLLM, LLMID: target.ID, Model: "big"}},
			{Name: "simple", Description: "Everything else", Target: sr.Target{Type: sr.TargetLLM, LLMID: target.ID, Model: "small"}},
		}}
	require.NoError(t, r.Create(db))
	return r
}

func TestPortal_SemanticRouterAsAsset(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)

	user := createTestUser(t, service)
	llmCat := createTestCatalogue(t, service)
	giveUserTeam(t, service, "Platform", user.ID, []uint{llmCat.ID}, nil, nil)

	llm := createTestLLM(t, service, "Semantic Target")
	router := createPortalSemanticRouter(t, db, "Smart Router", "smart", llm)
	hidden := createPortalSemanticRouter(t, db, "Hidden Router", "hidden-smart", llm)
	_, err := service.SetSemanticRouterCatalogues(router.ID, []uint{llmCat.ID})
	require.NoError(t, err)

	w := portalGetQuery(t, api.getPortalCatalog, user, "type=semantic_router")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list CatalogListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 1, w.Body.String())
	item := list.Data[0]
	assert.Equal(t, CatalogItemSemanticRouter, item.Type)
	assert.Equal(t, []string{"smart/auto", "smart/complex", "smart/simple"}, item.Attributes.RouterModels)
	require.Len(t, item.Attributes.RouterRoutes, 2)
	assert.Equal(t, "Hard reasoning", item.Attributes.RouterRoutes[0].Description)
	assert.True(t, item.Attributes.RouterRoutes[1].Default)
	assert.NotContains(t, w.Body.String(), "secret-keyword", "keywords and examples stay out of the portal")
	require.NotNil(t, item.Attributes.PrivacyScore)
	require.Len(t, item.Attributes.Catalogs, 1)
	assert.Equal(t, 1, list.Meta.Counts[CatalogItemSemanticRouter])

	w = portalGetQuery(t, api.getPortalCatalog, user, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"type":"semantic_router"`)
	w = portalGetQuery(t, api.getPortalCatalog, user, "q=meaning")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"Smart Router"`)

	w = portalGet(t, api.getPortalCatalogSemanticRouter, user, gin.Param{Key: "id", Value: idOf(router.ID)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var detail CatalogItemResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
	require.Len(t, detail.Data.Attributes.RouterLLMs, 1)
	assert.Equal(t, "Semantic Target", detail.Data.Attributes.RouterLLMs[0].Name)
	w = portalGet(t, api.getPortalCatalogSemanticRouter, user, gin.Param{Key: "id", Value: idOf(hidden.ID)})
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = portalCreateApp(t, api, user, CreateAppRequest{Name: "Smartly", Description: "d",
		DataSourceIDs: []uint{}, LLMIDs: []uint{}, ToolIDs: []uint{}, SemanticRouterIDs: []uint{router.ID}})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created AppResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, []uint{router.ID}, created.Attributes.SemanticRouterIDs)
	require.Len(t, created.Attributes.SemanticRouters, 1)
	assert.Equal(t, []string{"smart/auto", "smart/complex", "smart/simple"}, created.Attributes.SemanticRouters[0].Models)

	w = portalCreateApp(t, api, user, CreateAppRequest{Name: "Sneaky", Description: "d",
		DataSourceIDs: []uint{}, LLMIDs: []uint{}, ToolIDs: []uint{}, SemanticRouterIDs: []uint{hidden.ID}})
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	var n int64
	require.NoError(t, db.Model(&models.App{}).Where("name = ?", "Sneaky").Count(&n).Error)
	assert.Zero(t, n)
}

// In the Community Edition the admin API answers 402; the catalogue endpoint
// too, since there is nothing to publish.
func TestSemanticRouterAPI_CommunityEdition(t *testing.T) {
	api, _, _ := setupTestAPIForCommonTests(t)
	call := func(handler gin.HandlerFunc, method, path, body string, params ...gin.Param) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest(method, path, bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = params
		handler(c)
		return w
	}
	id := gin.Param{Key: "id", Value: "1"}
	assert.Equal(t, http.StatusPaymentRequired, call(api.createSemanticRouter, "POST", "/semantic-routers",
		`{"data":{"attributes":{"name":"S","slug":"s"}}}`).Code)
	assert.Equal(t, http.StatusPaymentRequired, call(api.listSemanticRouters, "GET", "/semantic-routers", "").Code)
	assert.Equal(t, http.StatusPaymentRequired, call(api.getSemanticRouter, "GET", "/semantic-routers/1", "", id).Code)
	assert.Equal(t, http.StatusPaymentRequired, call(api.testSemanticRouter, "POST", "/semantic-routers/1/test",
		`{"messages":[{"role":"user","content":"hi"}]}`, id).Code)
	assert.Equal(t, http.StatusPaymentRequired, call(api.setSemanticRouterCatalogues, "PUT", "/semantic-routers/1/catalogues",
		`{"catalogue_ids":[]}`, id).Code)
	assert.Equal(t, http.StatusBadRequest, call(api.createSemanticRouter, "POST", "/semantic-routers",
		`{"data":{"attributes":{"name":"S","slug":"s","logo_url":"javascript:alert(1)"}}}`).Code)
	assert.Equal(t, http.StatusBadRequest, call(api.testDraftSemanticRouter, "POST", "/semantic-routers/test",
		`{"messages":[{"role":"user","content":"hi"}]}`).Code, "a draft test needs the draft")
}
