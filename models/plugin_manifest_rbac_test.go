package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestRBAC_Validate(t *testing.T) {
	ok := &ManifestRBAC{
		Sensitive: true,
		Resources: []ManifestPermissionResource{
			{Key: "asset-types", Label: "Asset types", Actions: []string{"read", "write", "delete"}},
			{Key: "assets", Label: "Assets", Actions: []string{"read", "write", "delete", "publish"}},
		},
		RPCMethods: map[string]string{
			"admin_list_types":  "asset-types:read",
			"admin_upsert_type": "asset-types:write",
			"admin_stats":       "read",
			"admin_platform":    "plugins:execute",
			"admin_full":        "plugin:com.example.other:write",
		},
	}
	require.NoError(t, ok.Validate())
	var nilBlock *ManifestRBAC
	assert.NoError(t, nilBlock.Validate())

	bad := []ManifestRBAC{
		{Resources: []ManifestPermissionResource{{Key: "Bad Key", Label: "x", Actions: []string{"read"}}}},
		{Resources: []ManifestPermissionResource{{Key: "a", Label: "x", Actions: []string{"read"}}, {Key: "a", Label: "y", Actions: []string{"read"}}}},
		{Resources: []ManifestPermissionResource{{Key: "a", Label: "", Actions: []string{"read"}}}},
		{Resources: []ManifestPermissionResource{{Key: "a", Label: "x", Actions: []string{"write"}}}},
		{Resources: []ManifestPermissionResource{{Key: "a", Label: "x", Actions: []string{"read", "fly"}}}},
		{Resources: []ManifestPermissionResource{{Key: "a", Label: "x", Actions: []string{"read", "read"}}}},
		{RPCMethods: map[string]string{"m": ""}},
		{RPCMethods: map[string]string{"m": "assets:fly"}},
		{RPCMethods: map[string]string{"m": "Undeclared Thing:read"}},
	}
	for i, b := range bad {
		assert.Error(t, b.Validate(), "case %d", i)
	}

	// The block rides on the manifest and ValidateManifest runs it.
	pm := &PluginManifest{ID: "com.example.assets", Version: "1", Name: "Assets",
		Capabilities: &PluginCapabilities{Hooks: []string{HookTypeStudioUI}},
		RBAC:         &ManifestRBAC{Resources: []ManifestPermissionResource{{Key: "a", Label: "x", Actions: []string{"write"}}}}}
	assert.Error(t, pm.ValidateManifest())
	pm.RBAC = ok
	assert.NoError(t, pm.ValidateManifest())
}

func TestPlugin_PermissionKeyAndManifestRBAC(t *testing.T) {
	p := &Plugin{ID: 7}
	assert.Equal(t, "plugin:id-7", p.PermissionKey(), "no manifest yet")
	assert.False(t, p.HasAdminSurface())
	assert.Nil(t, p.ManifestRBAC())
	assert.Equal(t, "", p.RPCMethodPermission("x"))

	p.HookTypes = []string{HookTypeStudioUI}
	p.Manifest = map[string]interface{}{"id": " com.example.assets "}
	assert.Equal(t, "plugin:com.example.assets", p.PermissionKey())
	assert.True(t, p.HasAdminSurface())

	p.Manifest["id"] = "has space"
	assert.Equal(t, "plugin:id-7", p.PermissionKey(), "unusable ids fall back")

	// The rbac block round-trips through the stored map.
	raw := `{"id":"com.example.assets","rbac":{"sensitive":true,"resources":[{"key":"assets","label":"Assets","actions":["read","write"]}],"rpc_methods":{"admin_stats":"read"}}}`
	require.NoError(t, json.Unmarshal([]byte(raw), &p.Manifest))
	block := p.ManifestRBAC()
	require.NotNil(t, block)
	assert.True(t, block.Sensitive)
	assert.Len(t, block.Resources, 1)
	assert.Equal(t, "read", p.RPCMethodPermission("admin_stats"))
	assert.Equal(t, "", p.RPCMethodPermission("undeclared"))
}
