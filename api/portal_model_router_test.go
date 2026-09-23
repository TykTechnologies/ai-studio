package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A Model Router published in an LLM catalogue is a portal asset: the teams
// holding the catalogue see it, may add it to their Apps, and the App then
// carries the grant; everyone else sees nothing and is refused.

func createPortalRouter(t *testing.T, db *gorm.DB, name, slug string, llms ...*models.LLM) *models.ModelRouter {
	t.Helper()
	vendors := []*models.PoolVendor{}
	for _, l := range llms {
		vendors = append(vendors, &models.PoolVendor{LLMID: l.ID, Weight: 1, Active: true})
	}
	r := &models.ModelRouter{Name: name, Slug: slug, Active: true, ShortDescription: "Routes by model",
		APICompat: "openai", Pools: []*models.ModelPool{{Name: "gpt", ModelPattern: "gpt-4o,gpt-4o-mini",
			SelectionAlgorithm: models.SelectionRoundRobin, Vendors: vendors}}}
	require.NoError(t, r.Create(db))
	return r
}

func portalCreateApp(t *testing.T, api *API, user *models.User, req CreateAppRequest) *httptest.ResponseRecorder {
	t.Helper()
	payload, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/common/apps", bytes.NewBuffer(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", user)
	api.createUserApp(c)
	return w
}

func TestPortal_ModelRouterAsAsset(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)

	user := createTestUser(t, service)
	other := createTestUserWithSettings(t, service, "other-router@example.com", "Other", false, true, true, true, false)
	llmCat := createTestCatalogue(t, service)
	giveUserTeam(t, service, "Platform", user.ID, []uint{llmCat.ID}, nil, nil)

	llm := createTestLLM(t, service, "Router Target")
	router := createPortalRouter(t, db, "Prod Router", "prod", llm)
	hidden := createPortalRouter(t, db, "Hidden Router", "hidden", llm)
	_, err := service.SetModelRouterCatalogues(router.ID, []uint{llmCat.ID})
	require.NoError(t, err)

	// The catalog lists the published router, in the LLM catalogue family,
	// with the model strings to call it by and the target's privacy score.
	w := portalGetQuery(t, api.getPortalCatalog, user, "type=model_router")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list CatalogListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 1, w.Body.String())
	item := list.Data[0]
	assert.Equal(t, CatalogItemModelRouter, item.Type)
	assert.Equal(t, "Prod Router", item.Attributes.Name)
	assert.Equal(t, []string{"prod/gpt-4o", "prod/gpt-4o-mini"}, item.Attributes.RouterModels)
	require.NotNil(t, item.Attributes.PrivacyScore)
	assert.Equal(t, llm.PrivacyScore, *item.Attributes.PrivacyScore)
	require.Len(t, item.Attributes.Catalogs, 1)
	assert.Equal(t, llmCat.Name, item.Attributes.Catalogs[0].Name)
	assert.True(t, item.Attributes.AccessGrantedViaApp)
	assert.Equal(t, 1, list.Meta.Counts[CatalogItemModelRouter])

	// It shows up in the mixed view next to the LLM catalogue's LLMs.
	w = portalGetQuery(t, api.getPortalCatalog, user, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"type":"model_router"`)

	// Detail names the LLMs a request may end up at.
	w = portalGet(t, api.getPortalCatalogModelRouter, user, gin.Param{Key: "id", Value: idOf(router.ID)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var detail CatalogItemResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
	require.Len(t, detail.Data.Attributes.RouterLLMs, 1)
	assert.Equal(t, "Router Target", detail.Data.Attributes.RouterLLMs[0].Name)

	// Not published to the other user's teams.
	w = portalGet(t, api.getPortalCatalogModelRouter, other, gin.Param{Key: "id", Value: idOf(router.ID)})
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = portalGet(t, api.getPortalCatalogModelRouter, user, gin.Param{Key: "id", Value: idOf(hidden.ID)})
	assert.Equal(t, http.StatusNotFound, w.Code)

	// The user can build an App on the router alone.
	w = portalCreateApp(t, api, user, CreateAppRequest{Name: "Routed", Description: "d",
		DataSourceIDs: []uint{}, LLMIDs: []uint{}, ToolIDs: []uint{}, ModelRouterIDs: []uint{router.ID}})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created AppResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, []uint{router.ID}, created.Attributes.ModelRouterIDs)
	require.Len(t, created.Attributes.ModelRouters, 1)
	assert.Equal(t, "prod", created.Attributes.ModelRouters[0].Slug)
	assert.Empty(t, created.Attributes.LLMIDs)

	// But not on a router outside their catalogues, and nothing is created.
	w = portalCreateApp(t, api, user, CreateAppRequest{Name: "Sneaky", Description: "d",
		DataSourceIDs: []uint{}, LLMIDs: []uint{}, ToolIDs: []uint{}, ModelRouterIDs: []uint{hidden.ID}})
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	var n int64
	require.NoError(t, db.Model(&models.App{}).Where("name = ?", "Sneaky").Count(&n).Error)
	assert.Zero(t, n)
}

func TestModelRouterCataloguesEndpoint(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)
	llm := createTestLLM(t, service, "Router Target")
	router := createPortalRouter(t, db, "Prod Router", "prod", llm)
	cat := createTestCatalogue(t, service)

	put := func(id string, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest(http.MethodPut, "/model-routers/"+id+"/catalogues", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: id}}
		api.setModelRouterCatalogues(c)
		return w
	}

	w := put(idOf(router.ID), `{"catalogue_ids":[`+idOf(cat.ID)+`]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), cat.Name)

	assert.Equal(t, http.StatusBadRequest, put(idOf(router.ID), `{"catalogue_ids":[99999]}`).Code)
	assert.Equal(t, http.StatusNotFound, put("99999", `{"catalogue_ids":[]}`).Code)

	var r models.ModelRouter
	require.NoError(t, r.Get(db, router.ID))
	require.Len(t, r.Catalogues, 1, "a refused update leaves the publication as it was")
}
