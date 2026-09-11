package plugin_sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPortalUserContext_Can(t *testing.T) {
	const key = "plugin:com.example.assets"
	withKey := func(perms ...string) *PortalUserContext {
		return &PortalUserContext{Permissions: perms, Metadata: map[string]string{MetadataPluginPermissionKey: key}}
	}

	// Plugin-relative forms resolve against the plugin's own key.
	writer := withKey(key + ":write")
	assert.True(t, writer.Can("write"))
	assert.True(t, writer.Can("read"), "write implies read")
	assert.False(t, writer.Can("execute"))
	assert.False(t, writer.Can("assets:read"), "sub-resources are separate")
	assert.True(t, writer.Can(key+":write"), "fully qualified")
	assert.True(t, writer.Can(""), "empty means no requirement")

	sub := withKey(key + ":assets:publish")
	assert.True(t, sub.Can("assets:publish"))
	assert.True(t, sub.Can("assets:read"))
	assert.False(t, sub.Can("assets:write"))
	assert.False(t, sub.Can("write"))

	// Platform permissions pass through unchanged; the wildcard grants all.
	assert.True(t, withKey("llms:read").Can("llms:read"))
	assert.False(t, withKey("llms:read").Can("write"))
	assert.True(t, withKey("*").Can("assets:delete"))
	assert.True(t, withKey("*").Can("plugins:execute"))

	// The platform spells out the umbrella grant before handing permissions
	// over, so a plugins:execute holder arrives with the per-plugin entries.
	umbrella := withKey("plugins:execute", key+":read", key+":write", key+":execute")
	assert.True(t, umbrella.Can("write"))

	// Without the plugin key (older host) Can falls back to IsAdmin for
	// plugin-relative checks but still honours explicit permissions.
	legacyAdmin := &PortalUserContext{IsAdmin: true}
	assert.True(t, legacyAdmin.Can("write"))
	legacyUser := &PortalUserContext{IsAdmin: false, Permissions: []string{"llms:read"}}
	assert.False(t, legacyUser.Can("write"))
	assert.True(t, legacyUser.Can("llms:read"))
	var nilCtx *PortalUserContext
	assert.False(t, nilCtx.Can("read"))
	assert.Equal(t, "", nilCtx.PluginPermissionKey())
	assert.Equal(t, key, writer.PluginPermissionKey())
}
