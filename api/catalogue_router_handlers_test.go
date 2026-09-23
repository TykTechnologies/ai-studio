package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Routers can be added to an LLM catalogue from the catalogue, as LLMs are;
// the router pages see the same membership.
func TestCatalogueRoutersEndpoints(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)
	llm := createTestLLM(t, service, "Catalogue Target")
	mr := createPortalRouter(t, db, "Prod Router", "prod", llm)
	sr := createPortalSemanticRouter(t, db, "Smart Router", "smart", llm)
	cat := createTestCatalogue(t, service)

	call := func(handler gin.HandlerFunc, method, id, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest(method, "/catalogues/"+id+"/routers", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: id}}
		handler(c)
		return w
	}

	orig := routersAvailable
	t.Cleanup(func() { routersAvailable = orig })

	routersAvailable = func() bool { return false }
	assert.Equal(t, http.StatusPaymentRequired, call(api.getCatalogueRouters, "GET", idOf(cat.ID), "").Code)
	assert.Equal(t, http.StatusPaymentRequired, call(api.setCatalogueRouters, "PUT", idOf(cat.ID), `{}`).Code)

	routersAvailable = func() bool { return true }

	w := call(api.setCatalogueRouters, "PUT", idOf(cat.ID),
		`{"model_router_ids":[`+idOf(mr.ID)+`],"semantic_router_ids":[`+idOf(sr.ID)+`]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = call(api.getCatalogueRouters, "GET", idOf(cat.ID), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out struct {
		Data struct {
			ModelRouters    []CatalogueRouterRef `json:"model_routers"`
			SemanticRouters []CatalogueRouterRef `json:"semantic_routers"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Data.ModelRouters, 1)
	require.Len(t, out.Data.SemanticRouters, 1)
	assert.Equal(t, "smart", out.Data.SemanticRouters[0].Slug)

	cats, err := service.GetSemanticRouterCatalogues(sr.ID)
	require.NoError(t, err)
	assert.Len(t, cats, 1, "the router side sees the membership")

	assert.Equal(t, http.StatusBadRequest, call(api.setCatalogueRouters, "PUT", idOf(cat.ID), `{"model_router_ids":[99999]}`).Code)
	assert.Equal(t, http.StatusNotFound, call(api.getCatalogueRouters, "GET", "99999", "").Code)
}
