package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/services"
)

func features(on map[string]bool) func(string) bool {
	return func(name string) bool { return on[name] }
}

func allFeatures() map[string]bool {
	return map[string]bool{
		"feature_portal": true, "feature_chat": true, "feature_gateway": true,
		"feature_groups": true, "feature_rbac": true, "feature_model_router": true,
		"feature_semantic_router": true, "feature_tyk_mcp": true, "feature_webhooks": true,
	}
}

func labels(items []NavItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Text
	}
	return out
}

func find(items []NavItem, id string) *NavItem {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
		if f := find(items[i].Items, id); f != nil {
			return f
		}
	}
	return nil
}

func allowAll(NavItem) bool { return true }

func TestAdminNavGroupOrder(t *testing.T) {
	nav := filterNav(adminNav(navInputs{features: features(allFeatures()), enterprise: true}), allowAll)
	assert.Equal(t, []string{
		"Overview", "Analytics", "Access", "Catalogs", "LLM management",
		"Context management", "AI Portal", "Community", "Governance",
		"Settings", "Chat", "Plugins",
	}, labels(nav))
}

// Plugin sections sit after Governance (after Community in CE, where there
// is no Governance), sorted by label, and before Settings.
func TestAdminNavPluginSections(t *testing.T) {
	plugins := []services.SidebarMenuItem{
		{ID: "plugin_2", Label: "Zeta Tools", PluginID: 2, SubItems: []services.SidebarSubItem{{ID: "z1", Text: "Home", Path: "/admin/plugins/zeta"}}},
		{ID: "plugin_1", Label: "Asset Catalog", PluginID: 1, RequiredPermission: "plugin_1:read", SubItems: []services.SidebarSubItem{
			{ID: "a1", Text: "Overview", Path: "/admin/ac"},
			{ID: "a2", Text: "Detail", Path: "/admin/ac/detail", RequiredPermission: "plugin_1:write"},
		}},
	}
	nav := filterNav(adminNav(navInputs{features: features(allFeatures()), enterprise: true, plugins: plugins}), allowAll)
	l := labels(nav)
	i := indexOf(l, "Governance")
	assert.Equal(t, []string{"Governance", "Asset Catalog", "Zeta Tools", "Settings"}, l[i:i+4])
	assert.Equal(t, "Plugins", l[len(l)-1])

	ac := find(nav, "plugin_1")
	require.NotNil(t, ac)
	assert.Equal(t, "puzzle-piece", ac.Icon)
	assert.Equal(t, uint(1), ac.PluginID)
	assert.True(t, find(nav, "a1").Exact, "a page whose path prefixes a sibling's is exact")
	assert.False(t, find(nav, "a2").Exact)
	assert.Equal(t, "plugin_1:read", find(nav, "a1").Permission, "a page inherits its section's permission")
	assert.Equal(t, "plugin_1:write", find(nav, "a2").Permission)
	assert.Equal(t, "plugins:execute", find(nav, "z1").Permission, "plugin pages default to plugins:execute")

	ce := labels(filterNav(adminNav(navInputs{features: features(allFeatures()), plugins: plugins[:1]}), allowAll))
	assert.NotContains(t, ce, "Governance")
	assert.Equal(t, indexOf(ce, "Community")+1, indexOf(ce, "Zeta Tools"))
	assert.Equal(t, indexOf(ce, "Zeta Tools")+1, indexOf(ce, "Settings"))
}

func TestAdminNavEditionsAndModes(t *testing.T) {
	ent := filterNav(adminNav(navInputs{features: features(allFeatures()), enterprise: true}), allowAll)
	for _, id := range []string{"compliance", "audit", "metadata-schemas", "metadata-vocabularies", "metadata-compliance", "webhooks", "filters", "marketplace-settings"} {
		assert.NotNil(t, find(ent, id), id)
	}
	assert.Equal(t, "/admin/users", find(ent, "users").Path)
	assert.Nil(t, find(ent, "sso-profiles"), "identity providers need SSO, local sign-in and the permission")

	ce := filterNav(adminNav(navInputs{features: features(allFeatures())}), allowAll)
	for _, id := range []string{"governance", "filters", "marketplace-settings"} {
		assert.Nil(t, find(ce, id), id)
	}
	assert.NotNil(t, find(ce, "secrets"))

	gatewayOnly := labels(filterNav(adminNav(navInputs{features: features(map[string]bool{"feature_gateway": true, "feature_groups": true})}), allowAll))
	assert.Contains(t, gatewayOnly, "Apps & credentials")
	assert.NotContains(t, gatewayOnly, "AI Portal")
	assert.NotContains(t, gatewayOnly, "Catalogs")
	assert.NotContains(t, gatewayOnly, "Chat")
	assert.Equal(t, indexOf(gatewayOnly, "Context management")+1, indexOf(gatewayOnly, "Apps & credentials"))

	idp := filterNav(adminNav(navInputs{features: features(allFeatures()), identityProviders: true}), allowAll)
	assert.NotNil(t, find(idp, "sso-profiles"))
}

// Filtering follows the console drawer: pages by permission, groups kept
// while a page survives, Overview (no permission) always.
func TestFilterNavByPermission(t *testing.T) {
	granted := map[string]bool{"llms:read": true, "edges:read": true}
	nav := filterNav(adminNav(navInputs{features: features(allFeatures()), enterprise: true}), func(it NavItem) bool {
		return it.Permission == "" || granted[it.Permission]
	})
	assert.Equal(t, []string{"Overview", "LLM management", "AI Portal"}, labels(nav))
	assert.Equal(t, []string{"LLM providers"}, labels(find(nav, "llm-management").Items))
	assert.Equal(t, []string{"Edge Gateways"}, labels(find(nav, "ai-portal").Items))

	// A plugin section's own permission gates the whole section.
	plugins := []services.SidebarMenuItem{{ID: "p", Label: "P", RequiredPermission: "p:read", SubItems: []services.SidebarSubItem{{ID: "p1", Text: "One", Path: "/admin/p", RequiredPermission: "x:read"}}}}
	nav = filterNav(pluginNav(plugins), func(it NavItem) bool { return it.Permission == "x:read" })
	assert.Empty(t, nav)
}

// The admin menu with every feature on is kept as a golden file in the
// frontend, where a Jest test checks each path against admin/routes.js.
// UPDATE_NAV_GOLDEN=1 rewrites it.
func TestAdminNavGolden(t *testing.T) {
	plugins := []services.SidebarMenuItem{}
	nav := filterNav(adminNav(navInputs{features: features(allFeatures()), enterprise: true, identityProviders: true, plugins: plugins}), allowAll)
	got, err := json.MarshalIndent(nav, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')
	path := filepath.Join("..", "ui", "admin-frontend", "src", "admin", "nav.golden.json")
	if os.Getenv("UPDATE_NAV_GOLDEN") != "" {
		require.NoError(t, os.WriteFile(path, got, 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run with UPDATE_NAV_GOLDEN=1 to create it")
	assert.Equal(t, string(want), string(got), "the admin menu changed; rerun with UPDATE_NAV_GOLDEN=1 and review the diff")
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
