package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveAccessGrantedViaApp(t *testing.T) {
	yes, no := true, false
	// The MCP registry manifest: primary hook custom_endpoint plus resource_provider.
	mcpShape := &Plugin{HookType: HookTypeCustomEndpoint, HookTypes: []string{HookTypeCustomEndpoint, HookTypeStudioUI, HookTypeResourceProvider}}
	// The asset catalog manifest: resource_provider with UI hooks, no endpoints.
	catalogShape := &Plugin{HookType: HookTypeResourceProvider, HookTypes: []string{HookTypeResourceProvider, HookTypeStudioUI, HookTypePortalUI}}
	// Endpoints only, no resources: not a resource provider at all.
	endpointOnly := &Plugin{HookType: HookTypeCustomEndpoint}

	cases := []struct {
		name     string
		declared *bool
		plugin   *Plugin
		want     bool
	}{
		{"undeclared, proxying resource provider", nil, mcpShape, true},
		{"undeclared, catalog-style provider", nil, catalogShape, false},
		{"undeclared, endpoints without resources", nil, endpointOnly, false},
		{"undeclared, nil plugin", nil, nil, false},
		{"explicit false beats the heuristic", &no, mcpShape, false},
		{"explicit true beats the heuristic", &yes, catalogShape, true},
		// A freshly registered plugin: the row has only the primary hook and the
		// stored manifest; the manifest's hooks must count.
		{"undeclared, row lags behind manifest", nil, &Plugin{HookType: HookTypeCustomEndpoint, Manifest: map[string]interface{}{
			"capabilities": map[string]interface{}{"hooks": []interface{}{"custom_endpoint", "studio_ui", "resource_provider"}, "primary_hook": "custom_endpoint"},
		}}, true},
		// Customised hooks are the admin's word: the manifest is ignored.
		{"undeclared, customised row ignores manifest", nil, &Plugin{HookType: HookTypeCustomEndpoint, HookTypesCustomized: true, Manifest: map[string]interface{}{
			"capabilities": map[string]interface{}{"hooks": []interface{}{"custom_endpoint", "resource_provider"}},
		}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ResolveAccessGrantedViaApp(tc.declared, tc.plugin))
		})
	}
}

func TestEffectiveInstanceAccessGrantedViaApp(t *testing.T) {
	yes, no := true, false
	assert.True(t, EffectiveInstanceAccessGrantedViaApp(true, nil))
	assert.False(t, EffectiveInstanceAccessGrantedViaApp(false, nil))
	assert.False(t, EffectiveInstanceAccessGrantedViaApp(true, &no), "instance override revokes")
	assert.True(t, EffectiveInstanceAccessGrantedViaApp(false, &yes), "instance override grants")
}

// TestPluginResourceType_AccessGrantedViaAppPersistence guards the GORM bool
// trap: the resolved column keeps an explicit false, and the declared value
// round-trips as NULL / false / true.
func TestPluginResourceType_AccessGrantedViaAppPersistence(t *testing.T) {
	db := setupTestDB(t)
	plugin := &Plugin{Name: "p", Command: "/usr/bin/p", HookType: HookTypeResourceProvider, IsActive: true}
	assert.NoError(t, db.Create(plugin).Error)

	no := false
	rows := []PluginResourceType{
		{PluginID: plugin.ID, Slug: "undeclared", Name: "U", AccessGrantedViaApp: false, IsActive: true},
		{PluginID: plugin.ID, Slug: "declared-false", Name: "F", AccessGrantedViaApp: false, AccessGrantedViaAppDeclared: &no, IsActive: true},
		{PluginID: plugin.ID, Slug: "granted", Name: "G", AccessGrantedViaApp: true, PortalDetailPath: "/portal/plugins/p#/items/{id}", IsActive: true},
	}
	for i := range rows {
		assert.NoError(t, rows[i].Create(db))
	}

	var got PluginResourceType
	assert.NoError(t, got.GetByPluginAndSlug(db, plugin.ID, "undeclared"))
	assert.False(t, got.AccessGrantedViaApp)
	assert.Nil(t, got.AccessGrantedViaAppDeclared, "nil declared value stays NULL")

	got = PluginResourceType{}
	assert.NoError(t, got.GetByPluginAndSlug(db, plugin.ID, "declared-false"))
	assert.False(t, got.AccessGrantedViaApp)
	if assert.NotNil(t, got.AccessGrantedViaAppDeclared) {
		assert.False(t, *got.AccessGrantedViaAppDeclared)
	}

	got = PluginResourceType{}
	assert.NoError(t, got.GetByPluginAndSlug(db, plugin.ID, "granted"))
	assert.True(t, got.AccessGrantedViaApp)
	assert.Equal(t, "/portal/plugins/p#/items/{id}", got.PortalDetailPath)

	// Save() after flipping to false must persist false, not the default.
	got.AccessGrantedViaApp = false
	assert.NoError(t, got.Update(db))
	again := PluginResourceType{}
	assert.NoError(t, again.GetByPluginAndSlug(db, plugin.ID, "granted"))
	assert.False(t, again.AccessGrantedViaApp)
}
