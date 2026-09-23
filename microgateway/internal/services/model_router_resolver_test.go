package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func vendorFor(llmID uint, slug string, active bool, mappings ...database.ModelMapping) database.PoolVendor {
	return database.PoolVendor{ID: llmID, LLMID: llmID, LLMSlug: slug, Weight: 1, IsActive: active, Mappings: mappings}
}

// routedService has router "prod" (id 1): pool "gpt" (gpt-4o,gpt-4o-mini)
// over LLMs 10 and 11, pool "claude" (claude-*) over LLM 12 with a mapping,
// and an inactive vendor for LLM 13.
func routedService() *ModelRouterService {
	svc := NewModelRouterService(nil)
	gpt := createTestPool("gpt", "gpt-4o, gpt-4o-mini", "round_robin", []database.PoolVendor{
		vendorFor(10, "openai-a", true), vendorFor(11, "openai-b", true),
	})
	claude := createTestPool("claude", "claude-*", "round_robin", []database.PoolVendor{
		vendorFor(12, "anthropic", true, database.ModelMapping{SourceModel: "claude-best", TargetModel: "claude-opus-4"}),
		vendorFor(13, "anthropic-old", false),
	})
	svc.routerMutex.Lock()
	svc.routers["prod"] = createTestRouter("prod", []*CompiledPool{gpt, claude})
	svc.routerMutex.Unlock()
	return svc
}

func TestSelectVendorFor_RestrictsToPermittedVendors(t *testing.T) {
	svc := routedService()
	only11 := func(id uint) bool { return id == 11 }
	for i := 0; i < 5; i++ {
		sel, err := svc.SelectVendorFor("prod", "gpt-4o", only11)
		require.NoError(t, err)
		assert.Equal(t, uint(11), sel.Vendor.LLMID)
	}

	_, err := svc.SelectVendorFor("prod", "claude-x", only11)
	assert.ErrorIs(t, err, ErrNoPermittedVendors, "the matching pool has nothing permitted; no other pool is tried")

	_, err = svc.SelectVendorFor("prod", "llama", nil)
	assert.ErrorIs(t, err, ErrNoMatchingPool)
}

func TestModelRouterService_Reaches(t *testing.T) {
	svc := routedService()
	assert.True(t, svc.Reaches("prod", 1, 10))
	assert.True(t, svc.Reaches("prod", 1, 12))
	assert.False(t, svc.Reaches("prod", 1, 13), "an inactive vendor is not reachable")
	assert.False(t, svc.Reaches("prod", 1, 99))
	assert.False(t, svc.Reaches("prod", 2, 10), "a different router id under the same slug")
	assert.False(t, svc.Reaches("nope", 1, 10))
}

func TestModelRouterService_AdvertisedModels(t *testing.T) {
	svc := routedService()
	assert.Equal(t, []string{"claude-best", "gpt-4o", "gpt-4o-mini"}, svc.AdvertisedModels("prod"))
	assert.Nil(t, svc.AdvertisedModels("nope"))
}

func TestModelRouterResolver(t *testing.T) {
	res := NewModelRouterResolver(routedService())

	ref, ok := res.Lookup("prod")
	require.True(t, ok)
	assert.Equal(t, proxy.RouterRef{Kind: proxy.RouterKindModel, ID: 1, Slug: "prod"}, ref)
	_, ok = res.Lookup("nope")
	assert.False(t, ok)

	d, err := res.Resolve(context.Background(), proxy.RouteRequest{Router: ref, Model: "claude-best"})
	require.NoError(t, err)
	assert.Equal(t, uint(12), d.LLMID)
	assert.Equal(t, "claude-opus-4", d.Model)
	assert.Equal(t, "claude", d.Pool)
	assert.Equal(t, "model_pattern", d.Reason)

	_, err = res.Resolve(context.Background(), proxy.RouteRequest{Router: ref, Model: "llama"})
	assert.ErrorIs(t, err, proxy.ErrRouteNoMatch)

	_, err = res.Resolve(context.Background(), proxy.RouteRequest{Router: ref, Model: "gpt-4o", Allow: func(uint) bool { return false }})
	assert.ErrorIs(t, err, proxy.ErrRouteNoCandidates)

	assert.True(t, res.Reaches(ref, 10))
	assert.False(t, res.Reaches(proxy.RouterRef{Kind: "semantic_router", ID: 1, Slug: "prod"}, 10))
	assert.Contains(t, res.Models(ref), "gpt-4o")
}

func TestModelRouterResolver_InactivePoolIsUnavailable(t *testing.T) {
	svc := NewModelRouterService(nil)
	pool := createTestPool("dead", "*", "round_robin", []database.PoolVendor{vendorFor(10, "x", false)})
	svc.routerMutex.Lock()
	svc.routers["prod"] = createTestRouter("prod", []*CompiledPool{pool})
	svc.routerMutex.Unlock()
	res := NewModelRouterResolver(svc)
	ref, _ := res.Lookup("prod")
	_, err := res.Resolve(context.Background(), proxy.RouteRequest{Router: ref, Model: "gpt-4o"})
	assert.ErrorIs(t, err, proxy.ErrRouteUnavailable)
}

func TestModelRouterHandler_RewritesToTheAIChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotPath string
	h := NewModelRouterHandler(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusTeapot)
	})
	engine := gin.New()
	engine.POST("/router/:routerSlug/v1/chat/completions", h.GinHandler())

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/router/prod/v1/chat/completions", nil))
	assert.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "/ai/prod/v1/chat/completions", gotPath)
}
