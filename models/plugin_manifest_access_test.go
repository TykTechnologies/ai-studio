package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManifestResourceType_AccessGrantedViaApp: the manifest field is
// tri-state, so "omitted" (platform decides) is distinguishable from an
// explicit false.
func TestManifestResourceType_AccessGrantedViaApp(t *testing.T) {
	raw := `{
		"id": "com.example.p", "version": "1.0.0", "name": "P",
		"capabilities": {"hooks": ["resource_provider"], "primary_hook": "resource_provider"},
		"resource_types": [
			{"slug": "agent", "name": "Agent", "access_granted_via_app": false, "portal_detail_path": "/portal/plugins/p#/assets/{id}"},
			{"slug": "prompt", "name": "Prompt"},
			{"slug": "server", "name": "Server", "access_granted_via_app": true}
		]
	}`
	var m PluginManifest
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	require.Len(t, m.ResourceTypes, 3)

	agent, prompt, server := m.ResourceTypes[0], m.ResourceTypes[1], m.ResourceTypes[2]
	require.NotNil(t, agent.AccessGrantedViaApp)
	assert.False(t, *agent.AccessGrantedViaApp)
	assert.Equal(t, "/portal/plugins/p#/assets/{id}", agent.PortalDetailPath)

	assert.Nil(t, prompt.AccessGrantedViaApp, "omitted stays nil")
	assert.Empty(t, prompt.PortalDetailPath)

	require.NotNil(t, server.AccessGrantedViaApp)
	assert.True(t, *server.AccessGrantedViaApp)
}
