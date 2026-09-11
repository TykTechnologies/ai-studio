package api

import (
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every route under /api/v1 must carry a permission annotation. This test
// walks gin's routing table and the annotation registry in both directions
// so an unannotated route, a stale annotation, or a drift between the
// registry key and gin's path joining all fail loudly. It runs in both
// editions because the annotations live in core.
// publicV1Routes are deliberately unauthenticated routes that share the
// /api/v1 prefix but are registered on the public group. Adding to this list
// is a conscious decision to expose an endpoint without any permission.
var publicV1Routes = map[string]bool{
	"GET /api/v1/branding/settings": true,
	"GET /api/v1/branding/logo":     true,
	"GET /api/v1/branding/favicon":  true,
}

func assertRoutesAnnotated(t *testing.T, api *API) {
	t.Helper()
	var missing, orphan []string
	seen := map[string]bool{}
	for _, r := range api.router.Routes() {
		if !strings.HasPrefix(r.Path, "/api/v1/") {
			continue
		}
		key := routeKey(r.Method, r.Path)
		if publicV1Routes[key] {
			continue
		}
		seen[key] = true
		if _, ok := api.routePerms[key]; !ok {
			missing = append(missing, key)
		}
	}
	for key := range api.routePerms {
		if !seen[key] {
			orphan = append(orphan, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(orphan)
	assert.Empty(t, missing, "routes under /api/v1 without a permission annotation")
	assert.Empty(t, orphan, "annotations that match no registered route")

	for key, e := range api.routePerms {
		if e.dynamic() {
			continue
		}
		for _, p := range e.permissions(nil) {
			assert.True(t, p.Valid(), "%s annotated with unknown permission %q", key, p)
		}
	}
}

func TestAuthzRoutes_AllAnnotated(t *testing.T) {
	api, _ := setupTestAPI(t)
	assertRoutesAnnotated(t, api)
	assert.Greater(t, len(api.routePerms), 300, "sanity: the registry covers the whole management API")
}

// The marketplace routes are only registered when the service is wired, so
// they need their own construction to be covered by the completeness check.
func TestAuthzRoutes_AllAnnotated_WithMarketplace(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	service.MarketplaceService = services.NewMarketplaceService(db, nil, service.PluginService, nil, t.TempDir(), "", 0)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	assertRoutesAnnotated(t, api)
	_, ok := api.routePerms[routeKey("GET", "/api/v1/marketplace/plugins")]
	assert.True(t, ok, "marketplace routes are annotated")
}

func TestAuthzRoutes_RegistryLookup(t *testing.T) {
	api, _ := setupTestAPI(t)

	get := func(method, path string) permEntry {
		e, ok := api.routePerms[routeKey(method, path)]
		require.True(t, ok, "%s %s", method, path)
		return e
	}
	assert.Equal(t, authz.Read("llms"), get("GET", "/api/v1/llms").perm)
	assert.Equal(t, authz.Write("llms"), get("PATCH", "/api/v1/llms/:id").perm)
	assert.Equal(t, authz.Delete("users"), get("DELETE", "/api/v1/users/:id").perm)
	assert.Equal(t, authz.Execute("filters"), get("POST", "/api/v1/filters/test").perm)
	assert.Equal(t, authz.Write("model-prices"), get("GET", "/api/v1/model-prices/by-name").perm, "get-or-create is a write")
	assert.Equal(t, authz.AnyAdmin, get("POST", "/api/v1/logout").perm)
	assert.Equal(t, authz.AnyAdmin, get("GET", "/api/v1/plugins/sidebar-menu").perm)
	assert.Equal(t, authz.Read("tools"), get("GET", "/api/v1/providers").perm)
	assert.Equal(t, authz.Read("marketplace"), get("GET", "/api/v1/admin/marketplaces").perm)
	assert.Equal(t, authz.Write("sso-profiles"), get("POST", "/api/v1/sso-profiles").perm)

	// Dedicated activate/deactivate routes carry publish, not write.
	assert.Equal(t, authz.Publish("llms"), get("POST", "/api/v1/llms/:id/activate").perm)
	assert.Equal(t, authz.Publish("llms"), get("POST", "/api/v1/llms/:id/deactivate").perm)
	assert.Equal(t, authz.Publish("tools"), get("POST", "/api/v1/tools/:id/activate").perm)
	assert.Equal(t, authz.Publish("datasources"), get("POST", "/api/v1/datasources/:id/deactivate").perm)
	assert.Equal(t, authz.Publish("apps"), get("POST", "/api/v1/apps/:id/activate").perm)
	assert.Equal(t, authz.Write("apps"), get("POST", "/api/v1/apps/:id/activate-credential").perm, "credential toggles stay write")
	assert.Equal(t, authz.Publish("agents"), get("POST", "/api/v1/agents/:id/activate").perm)
	assert.Equal(t, authz.Publish("model-routers"), get("PATCH", "/api/v1/model-routers/:id/toggle").perm)
	assert.Equal(t, authz.Publish("plugins"), get("POST", "/api/v1/plugins/:id/enable").perm)
	assert.Equal(t, authz.Publish("plugins"), get("POST", "/api/v1/plugins/:id/disable").perm)
	assert.Equal(t, authz.Publish("metadata"), get("POST", "/api/v1/metadata/schemas/:id/activate").perm)
}

func TestAuthzRoutes_MetadataObjectResolver(t *testing.T) {
	api, _ := setupTestAPI(t)
	e, ok := api.routePerms[routeKey("PUT", "/api/v1/metadata/objects/:object_type/:object_id")]
	require.True(t, ok)
	require.NotNil(t, e.resolve)

	resolveFor := func(objectType string) authz.Permission {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Params = gin.Params{{Key: "object_type", Value: objectType}, {Key: "object_id", Value: "1"}}
		return e.permission(c)
	}
	assert.Equal(t, authz.Write("llms"), resolveFor("llm"))
	assert.Equal(t, authz.Write("tools"), resolveFor("tool"))
	assert.Equal(t, authz.Write("datasources"), resolveFor("datasource"))
	assert.Equal(t, authz.Write("plugins"), resolveFor("plugin_resource"))
	assert.Equal(t, authz.Write("metadata"), resolveFor("something-else"), "unknown types fall back to schema governance")
}

// The plugin routes resolve to the platform-level permission or the
// per-plugin one of the plugin named in the path.
func TestAuthzRoutes_PluginResolvers(t *testing.T) {
	api, db := setupTestAPI(t)
	plugin := &models.Plugin{
		Name:      "Asset catalog",
		Command:   "/bin/true",
		HookType:  models.HookTypeStudioUI,
		HookTypes: []string{models.HookTypeStudioUI},
		Manifest:  map[string]interface{}{"id": "com.example.assets"},
	}
	require.NoError(t, plugin.Create(db))
	const key = "plugin:com.example.assets"
	assert.Equal(t, key, plugin.PermissionKey())

	ctxFor := func(id string, method string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Params = gin.Params{{Key: "id", Value: id}, {Key: "method", Value: method}}
		return c
	}
	entry := func(method, path string) permEntry {
		e, ok := api.routePerms[routeKey(method, path)]
		require.True(t, ok, "%s %s", method, path)
		require.True(t, e.dynamic())
		return e
	}
	id := strconv.FormatUint(uint64(plugin.ID), 10)

	get := entry("GET", "/api/v1/plugins/:id")
	assert.Equal(t, []authz.Permission{authz.Read("plugins"), authz.Read(key)}, get.permissions(ctxFor(id, "")))
	assert.Equal(t, authz.Read("plugins"), get.permission(ctxFor(id, "")), "the platform permission is the one a denial reports")
	assert.Equal(t, []authz.Permission{authz.Read("plugins")}, get.permissions(ctxFor("999999", "")), "unknown plugin: platform permission only")

	patch := entry("PATCH", "/api/v1/plugins/:id")
	assert.Equal(t, []authz.Permission{authz.Write("plugins"), authz.Write(key)}, patch.permissions(ctxFor(id, "")))

	rpc := entry("POST", "/api/v1/plugins/:id/rpc/:method")
	assert.Equal(t, []authz.Permission{authz.Write(key)}, rpc.permissions(ctxFor(id, "admin_stats")))
	assert.Equal(t, []authz.Permission{authz.Execute("plugins")}, rpc.permissions(ctxFor("999999", "x")))

	// The umbrella rule makes the resolved permissions satisfiable by the
	// legacy grant, and a per-plugin grant satisfies them without it.
	require.NoError(t, authz.Replace(services.PluginBaseResource(plugin)))
	t.Cleanup(func() { authz.UnregisterPlugin(key) })
	assert.True(t, authz.NewSet(authz.Execute("plugins")).HasAny(rpc.permissions(ctxFor(id, "x"))...))
	assert.True(t, authz.NewSet(authz.Write(key)).HasAny(rpc.permissions(ctxFor(id, "x"))...))
	assert.False(t, authz.NewSet(authz.Read(key)).HasAny(rpc.permissions(ctxFor(id, "x"))...))
	assert.True(t, authz.NewSet(authz.Read(key)).HasAny(get.permissions(ctxFor(id, ""))...))
	assert.False(t, authz.NewSet(authz.Read("plugins")).HasAny(rpc.permissions(ctxFor(id, "x"))...))
}

func TestPermRouter_RejectsUnknownPermission(t *testing.T) {
	api, _ := setupTestAPI(t)
	g := api.permGroup(gin.New().Group("/x"))
	assert.Panics(t, func() { g.GET("/y", authz.Permission("nope:read"), func(*gin.Context) {}) })
	assert.Panics(t, func() { g.GET("/z", authz.Permission("analytics:write"), func(*gin.Context) {}) })
}

func TestJoinRoutePath_MatchesGin(t *testing.T) {
	assert.Equal(t, "/api/v1/llms", joinRoutePath("/api/v1", "/llms"))
	assert.Equal(t, "/api/v1/admin/marketplaces", joinRoutePath("/api/v1/admin/marketplaces", ""))
	assert.Equal(t, "/api/v1/x/", joinRoutePath("/api/v1", "/x/"))
	assert.Equal(t, "/api/v1/plugins/assets/:id/*filepath", joinRoutePath("/api/v1", "/plugins/assets/:id/*filepath"))
}
