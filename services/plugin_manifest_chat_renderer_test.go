package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterPluginUI_ChatToolRenderers(t *testing.T) {
	service, db := setupManifestServiceTest(t)
	plugin := createTestPluginForManifest(t, db, "renderer-plugin")

	manifest := &models.PluginManifest{
		ID:      "com.test.renderers",
		Version: "1.0.0",
		Name:    "Renderer Plugin",
		Capabilities: &models.PluginCapabilities{
			Hooks: []string{"portal_ui"},
		},
		Portal: &struct {
			Slots []models.PortalUISlot `json:"slots"`
		}{
			Slots: []models.PortalUISlot{
				{
					Slot:  models.PortalSlotChatToolRenderer,
					Label: "Chat renderers",
					Items: []models.UISlotItem{
						{Type: "component", Tool: "getWeather", Title: "Weather card", Mount: models.UIMount{Kind: "webc", Tag: "x-weather", Entry: "/ui/weather.js"}},
						{Type: "component", Path: "getForecast", Mount: models.UIMount{Kind: "webc", Tag: "x-forecast", Entry: "/ui/forecast.js"}},
						{Type: "component", Mount: models.UIMount{Kind: "webc", Tag: "x-nameless", Entry: "/ui/n.js"}},
					},
				},
				{
					Slot:  "portal_sidebar.section",
					Label: "Pages",
					Items: []models.UISlotItem{
						{Type: "component", Path: "/ignored", Mount: models.UIMount{Kind: "webc", Tag: "x-ignored", Entry: "/ui/i.js"}},
					},
				},
			},
		},
	}

	require.NoError(t, service.RegisterPluginUI(plugin, manifest))

	var entries []models.UIRegistry
	require.NoError(t, db.Where("plugin_id = ? AND scope = ?", plugin.ID, "portal").Order("id").Find(&entries).Error)
	require.Len(t, entries, 2, "component items are persisted only for the chat.tool_renderer slot and need a tool name")

	assert.Equal(t, models.PortalSlotChatToolRenderer, entries[0].SlotType)
	assert.Equal(t, "getWeather", entries[0].RoutePattern, "the tool name is the lookup key")
	assert.Equal(t, "x-weather", entries[0].ComponentTag)
	assert.Equal(t, "/ui/weather.js", entries[0].EntryPoint)
	assert.Equal(t, "webc", entries[0].MountConfig["kind"])

	assert.Equal(t, "getForecast", entries[1].RoutePattern, "path is accepted as the tool name for older manifests")
}
