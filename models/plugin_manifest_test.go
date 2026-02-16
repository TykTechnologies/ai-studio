package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupManifestTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = InitModels(db)
	require.NoError(t, err)
	return db
}

func TestPluginManifest_PortalValidation(t *testing.T) {
	t.Run("valid manifest with portal section", func(t *testing.T) {
		manifest := &PluginManifest{
			ID:      "test.portal.plugin",
			Version: "1.0.0",
			Name:    "Test Portal Plugin",
			Capabilities: &PluginCapabilities{
				Hooks: []string{"studio_ui", "portal_ui"},
			},
			Portal: &struct {
				Slots []PortalUISlot `json:"slots"`
			}{
				Slots: []PortalUISlot{
					{
						Slot:   "portal_sidebar.section",
						Label:  "Test Section",
						Icon:   "star",
						Groups: []string{},
						Items: []UISlotItem{
							{
								Type:  "route",
								Path:  "/portal/plugins/test",
								Title: "Test Page",
								Mount: UIMount{Kind: "webc", Tag: "test-portal", Entry: "/ui/test.js"},
							},
						},
					},
				},
			},
		}

		err := manifest.ValidateManifest()
		assert.NoError(t, err)
	})

	t.Run("portal slot missing slot identifier", func(t *testing.T) {
		manifest := &PluginManifest{
			ID:      "test.portal.invalid",
			Version: "1.0.0",
			Name:    "Invalid Portal Plugin",
			Capabilities: &PluginCapabilities{
				Hooks: []string{"studio_ui", "portal_ui"},
			},
			Portal: &struct {
				Slots []PortalUISlot `json:"slots"`
			}{
				Slots: []PortalUISlot{
					{
						Slot: "", // Missing
						Items: []UISlotItem{
							{Type: "route", Mount: UIMount{Kind: "webc"}},
						},
					},
				},
			},
		}

		err := manifest.ValidateManifest()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "portal slot 0 missing slot identifier")
	})

	t.Run("portal item missing type", func(t *testing.T) {
		manifest := &PluginManifest{
			ID:      "test.portal.notype",
			Version: "1.0.0",
			Name:    "No Type",
			Capabilities: &PluginCapabilities{
				Hooks: []string{"portal_ui"},
			},
			Portal: &struct {
				Slots []PortalUISlot `json:"slots"`
			}{
				Slots: []PortalUISlot{
					{
						Slot: "portal_sidebar.section",
						Items: []UISlotItem{
							{Type: "", Mount: UIMount{Kind: "webc"}},
						},
					},
				},
			},
		}

		err := manifest.ValidateManifest()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "portal slot 0 item 0 missing type")
	})

	t.Run("portal item missing mount kind", func(t *testing.T) {
		manifest := &PluginManifest{
			ID:      "test.portal.nomount",
			Version: "1.0.0",
			Name:    "No Mount",
			Capabilities: &PluginCapabilities{
				Hooks: []string{"portal_ui"},
			},
			Portal: &struct {
				Slots []PortalUISlot `json:"slots"`
			}{
				Slots: []PortalUISlot{
					{
						Slot: "portal_sidebar.section",
						Items: []UISlotItem{
							{Type: "route", Mount: UIMount{Kind: ""}},
						},
					},
				},
			},
		}

		err := manifest.ValidateManifest()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "portal slot 0 item 0 missing mount kind")
	})

	t.Run("manifest without portal section is valid", func(t *testing.T) {
		manifest := &PluginManifest{
			ID:      "test.no.portal",
			Version: "1.0.0",
			Name:    "No Portal",
			Capabilities: &PluginCapabilities{
				Hooks: []string{"studio_ui"},
			},
		}

		err := manifest.ValidateManifest()
		assert.NoError(t, err)
	})

	t.Run("portal_ui is a valid hook type", func(t *testing.T) {
		assert.True(t, IsValidHookType("portal_ui"))
	})
}

func TestPluginManifest_GetPortalRoutes(t *testing.T) {
	t.Run("returns portal routes", func(t *testing.T) {
		manifest := &PluginManifest{
			Portal: &struct {
				Slots []PortalUISlot `json:"slots"`
			}{
				Slots: []PortalUISlot{
					{
						Slot: "portal_sidebar.section",
						Items: []UISlotItem{
							{Type: "route", Path: "/portal/plugins/feedback", Title: "Feedback"},
							{Type: "route", Path: "/portal/plugins/tickets", Title: "Tickets"},
							{Type: "component", Path: "/portal/plugins/widget"},
						},
					},
				},
			},
		}

		routes := manifest.GetPortalRoutes()
		assert.Len(t, routes, 2) // Only "route" type items
		assert.Equal(t, "/portal/plugins/feedback", routes[0].Path)
		assert.Equal(t, "/portal/plugins/tickets", routes[1].Path)
	})

	t.Run("returns empty for nil portal section", func(t *testing.T) {
		manifest := &PluginManifest{}
		routes := manifest.GetPortalRoutes()
		assert.Empty(t, routes)
	})
}

func TestPluginManifest_GetPortalSidebarItems(t *testing.T) {
	t.Run("returns only portal_sidebar.section slots", func(t *testing.T) {
		manifest := &PluginManifest{
			Portal: &struct {
				Slots []PortalUISlot `json:"slots"`
			}{
				Slots: []PortalUISlot{
					{Slot: "portal_sidebar.section", Label: "Section 1"},
					{Slot: "portal_sidebar.link", Label: "Link"},
					{Slot: "portal_sidebar.section", Label: "Section 2"},
				},
			},
		}

		items := manifest.GetPortalSidebarItems()
		assert.Len(t, items, 2)
		assert.Equal(t, "Section 1", items[0].Label)
		assert.Equal(t, "Section 2", items[1].Label)
	})

	t.Run("returns empty for nil portal section", func(t *testing.T) {
		manifest := &PluginManifest{}
		items := manifest.GetPortalSidebarItems()
		assert.Empty(t, items)
	})
}

func TestPluginManifest_PortalUIPermission(t *testing.T) {
	manifest := &PluginManifest{
		ID:      "test.perms",
		Version: "1.0.0",
		Name:    "Perms Test",
		Capabilities: &PluginCapabilities{
			Hooks: []string{"portal_ui"},
		},
	}
	manifest.Permissions.PortalUI = []string{"sidebar.register", "route.register"}

	assert.True(t, manifest.HasPermission("portal_ui", "sidebar.register"))
	assert.True(t, manifest.HasPermission("portal_ui", "route.register"))
	assert.False(t, manifest.HasPermission("portal_ui", "nonexistent"))
}

func TestUIRegistry_ScopeAndGroups(t *testing.T) {
	db := setupManifestTestDB(t)

	// Create a plugin first (needed for foreign key)
	plugin := &Plugin{
		Name:     "Test Portal Plugin",
		Command:  "test-command",
		HookType: HookTypeStudioUI,
		IsActive: true,
	}
	require.NoError(t, db.Create(plugin).Error)

	t.Run("create admin-scoped entry", func(t *testing.T) {
		entry := &UIRegistry{
			PluginID:     plugin.ID,
			SlotType:     "sidebar.section",
			RoutePattern: "/admin/test",
			ComponentTag: "test-admin",
			IsActive:     true,
			Scope:        "admin",
		}
		require.NoError(t, db.Create(entry).Error)
		assert.Equal(t, "admin", entry.Scope)
	})

	t.Run("create portal-scoped entry with groups", func(t *testing.T) {
		entry := &UIRegistry{
			PluginID:      plugin.ID,
			SlotType:      "portal_sidebar.section",
			RoutePattern:  "/portal/plugins/test",
			ComponentTag:  "test-portal",
			IsActive:      true,
			Scope:         "portal",
			AllowedGroups: []string{"engineering", "support"},
		}
		require.NoError(t, db.Create(entry).Error)

		// Reload from DB to verify serialization
		var loaded UIRegistry
		require.NoError(t, db.First(&loaded, entry.ID).Error)
		assert.Equal(t, "portal", loaded.Scope)
		assert.Equal(t, []string{"engineering", "support"}, loaded.AllowedGroups)
	})

	t.Run("create portal-scoped entry with empty groups", func(t *testing.T) {
		entry := &UIRegistry{
			PluginID:      plugin.ID,
			SlotType:      "portal_sidebar.section",
			RoutePattern:  "/portal/plugins/public",
			ComponentTag:  "test-public",
			IsActive:      true,
			Scope:         "portal",
			AllowedGroups: []string{},
		}
		require.NoError(t, db.Create(entry).Error)

		var loaded UIRegistry
		require.NoError(t, db.First(&loaded, entry.ID).Error)
		assert.Equal(t, "portal", loaded.Scope)
		assert.Empty(t, loaded.AllowedGroups)
	})

	t.Run("query by scope", func(t *testing.T) {
		var adminEntries []UIRegistry
		require.NoError(t, db.Where("scope = ?", "admin").Find(&adminEntries).Error)
		assert.Len(t, adminEntries, 1)

		var portalEntries []UIRegistry
		require.NoError(t, db.Where("scope = ?", "portal").Find(&portalEntries).Error)
		assert.Len(t, portalEntries, 2)
	})
}

func TestHookTypePortalUI(t *testing.T) {
	assert.True(t, IsValidHookType(HookTypePortalUI))
	assert.Equal(t, "portal_ui", HookTypePortalUI)

	// Verify it's in the valid hook types list
	validTypes := GetValidHookTypes()
	found := false
	for _, ht := range validTypes {
		if ht == HookTypePortalUI {
			found = true
			break
		}
	}
	assert.True(t, found, "portal_ui should be in valid hook types")
}

func TestPlugin_SupportsPortalUI(t *testing.T) {
	plugin := &Plugin{
		HookType:  HookTypeStudioUI,
		HookTypes: []string{HookTypeStudioUI, HookTypePortalUI},
	}

	assert.True(t, plugin.SupportsHookType(HookTypePortalUI))
	assert.True(t, plugin.SupportsHookType(HookTypeStudioUI))
	assert.False(t, plugin.SupportsHookType(HookTypePostAuth))
}
