package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupManifestServiceTest(t *testing.T) (*PluginManifestService, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = models.InitModels(db)
	require.NoError(t, err)

	service := NewPluginManifestService(db, nil)
	return service, db
}

func createTestPluginForManifest(t *testing.T, db *gorm.DB, name string) *models.Plugin {
	plugin := &models.Plugin{
		Name:      name,
		Command:   "file:///test/" + name,
		HookType:  models.HookTypeStudioUI,
		HookTypes: []string{models.HookTypeStudioUI, models.HookTypePortalUI},
		IsActive:  true,
	}
	require.NoError(t, db.Create(plugin).Error)
	return plugin
}

func TestGroupsOverlap(t *testing.T) {
	t.Run("overlapping groups", func(t *testing.T) {
		assert.True(t, groupsOverlap([]string{"a", "b"}, []string{"b", "c"}))
	})

	t.Run("no overlap", func(t *testing.T) {
		assert.False(t, groupsOverlap([]string{"a", "b"}, []string{"c", "d"}))
	})

	t.Run("empty user groups", func(t *testing.T) {
		assert.False(t, groupsOverlap([]string{}, []string{"a", "b"}))
	})

	t.Run("empty allowed groups", func(t *testing.T) {
		assert.False(t, groupsOverlap([]string{"a", "b"}, []string{}))
	})

	t.Run("both empty", func(t *testing.T) {
		assert.False(t, groupsOverlap([]string{}, []string{}))
	})

	t.Run("nil inputs", func(t *testing.T) {
		assert.False(t, groupsOverlap(nil, nil))
		assert.False(t, groupsOverlap(nil, []string{"a"}))
		assert.False(t, groupsOverlap([]string{"a"}, nil))
	})

	t.Run("exact match single element", func(t *testing.T) {
		assert.True(t, groupsOverlap([]string{"admin"}, []string{"admin"}))
	})
}

func TestRegisterPluginUI_PortalEntries(t *testing.T) {
	service, db := setupManifestServiceTest(t)
	plugin := createTestPluginForManifest(t, db, "portal-test-plugin")

	manifest := &models.PluginManifest{
		ID:      "com.test.portal",
		Version: "1.0.0",
		Name:    "Portal Test Plugin",
		Capabilities: &models.PluginCapabilities{
			Hooks: []string{"studio_ui", "portal_ui"},
		},
		UI: &struct {
			Slots []models.UISlot `json:"slots"`
		}{
			Slots: []models.UISlot{
				{
					Slot:  "sidebar.section",
					Label: "Admin Panel",
					Icon:  "settings",
					Items: []models.UISlotItem{
						{Type: "route", Path: "/admin/test", Title: "Admin", Mount: models.UIMount{Kind: "webc", Tag: "test-admin", Entry: "/ui/admin.js"}},
					},
				},
			},
		},
		Portal: &struct {
			Slots []models.PortalUISlot `json:"slots"`
		}{
			Slots: []models.PortalUISlot{
				{
					Slot:   "portal_sidebar.section",
					Label:  "Portal Feature",
					Icon:   "star",
					Groups: []string{"engineering"},
					Items: []models.UISlotItem{
						{Type: "route", Path: "/portal/plugins/test/page1", Title: "Page 1", Mount: models.UIMount{Kind: "webc", Tag: "test-portal-1", Entry: "/ui/portal1.js"}},
						{Type: "route", Path: "/portal/plugins/test/page2", Title: "Page 2", Mount: models.UIMount{Kind: "webc", Tag: "test-portal-2", Entry: "/ui/portal2.js"}},
					},
				},
			},
		},
	}

	err := service.RegisterPluginUI(plugin, manifest)
	require.NoError(t, err)

	t.Run("creates admin-scoped entries", func(t *testing.T) {
		var adminEntries []models.UIRegistry
		require.NoError(t, db.Where("plugin_id = ? AND (scope = ? OR scope = ? OR scope IS NULL)", plugin.ID, "admin", "").Find(&adminEntries).Error)
		assert.Len(t, adminEntries, 1)
		assert.Equal(t, "/admin/test", adminEntries[0].RoutePattern)
		assert.Equal(t, "test-admin", adminEntries[0].ComponentTag)
	})

	t.Run("creates portal-scoped entries", func(t *testing.T) {
		var portalEntries []models.UIRegistry
		require.NoError(t, db.Where("plugin_id = ? AND scope = ?", plugin.ID, "portal").Find(&portalEntries).Error)
		assert.Len(t, portalEntries, 2)
		assert.Equal(t, "portal", portalEntries[0].Scope)
		assert.Equal(t, "portal", portalEntries[1].Scope)
	})

	t.Run("portal entries have correct allowed groups", func(t *testing.T) {
		var portalEntries []models.UIRegistry
		require.NoError(t, db.Where("plugin_id = ? AND scope = ?", plugin.ID, "portal").Find(&portalEntries).Error)
		for _, entry := range portalEntries {
			assert.Equal(t, []string{"engineering"}, entry.AllowedGroups)
		}
	})

	t.Run("portal entries have correct slot type", func(t *testing.T) {
		var portalEntries []models.UIRegistry
		require.NoError(t, db.Where("plugin_id = ? AND scope = ?", plugin.ID, "portal").Find(&portalEntries).Error)
		for _, entry := range portalEntries {
			assert.Equal(t, "portal_sidebar.section", entry.SlotType)
		}
	})
}

func TestGetPortalUIRegistryForUser(t *testing.T) {
	service, db := setupManifestServiceTest(t)
	plugin := createTestPluginForManifest(t, db, "portal-registry-test")

	// Create portal entries with different group restrictions
	entries := []models.UIRegistry{
		{PluginID: plugin.ID, SlotType: "portal_sidebar.section", RoutePattern: "/portal/plugins/public", ComponentTag: "public-page", IsActive: true, Scope: "portal", AllowedGroups: []string{}},
		{PluginID: plugin.ID, SlotType: "portal_sidebar.section", RoutePattern: "/portal/plugins/eng", ComponentTag: "eng-page", IsActive: true, Scope: "portal", AllowedGroups: []string{"engineering"}},
		{PluginID: plugin.ID, SlotType: "portal_sidebar.section", RoutePattern: "/portal/plugins/support", ComponentTag: "support-page", IsActive: true, Scope: "portal", AllowedGroups: []string{"support", "ops"}},
		{PluginID: plugin.ID, SlotType: "sidebar.section", RoutePattern: "/admin/test", ComponentTag: "admin-page", IsActive: true, Scope: "admin"},
	}
	for i := range entries {
		require.NoError(t, db.Create(&entries[i]).Error)
	}
	// Create inactive entry separately - GORM's default:true overrides false on create,
	// so we create it active then explicitly update to inactive
	inactiveEntry := models.UIRegistry{PluginID: plugin.ID, SlotType: "portal_sidebar.section", RoutePattern: "/portal/plugins/inactive", ComponentTag: "inactive-page", IsActive: true, Scope: "portal", AllowedGroups: []string{}}
	require.NoError(t, db.Create(&inactiveEntry).Error)
	require.NoError(t, db.Model(&inactiveEntry).Update("is_active", false).Error)

	t.Run("user with no groups sees only public entries", func(t *testing.T) {
		result, err := service.GetPortalUIRegistryForUser([]string{})
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "public-page", result[0].ComponentTag)
	})

	t.Run("user in engineering group sees public + eng entries", func(t *testing.T) {
		result, err := service.GetPortalUIRegistryForUser([]string{"engineering"})
		require.NoError(t, err)
		assert.Len(t, result, 2)

		tags := make(map[string]bool)
		for _, entry := range result {
			tags[entry.ComponentTag] = true
		}
		assert.True(t, tags["public-page"])
		assert.True(t, tags["eng-page"])
	})

	t.Run("user in ops group sees public + support entries", func(t *testing.T) {
		result, err := service.GetPortalUIRegistryForUser([]string{"ops"})
		require.NoError(t, err)
		assert.Len(t, result, 2)

		tags := make(map[string]bool)
		for _, entry := range result {
			tags[entry.ComponentTag] = true
		}
		assert.True(t, tags["public-page"])
		assert.True(t, tags["support-page"])
	})

	t.Run("user in multiple groups sees all matching entries", func(t *testing.T) {
		result, err := service.GetPortalUIRegistryForUser([]string{"engineering", "support"})
		require.NoError(t, err)
		assert.Len(t, result, 3)
	})

	t.Run("does not return admin-scoped entries", func(t *testing.T) {
		result, err := service.GetPortalUIRegistryForUser([]string{"engineering", "support", "ops"})
		require.NoError(t, err)
		for _, entry := range result {
			assert.Equal(t, "portal", entry.Scope)
			assert.NotEqual(t, "admin-page", entry.ComponentTag)
		}
	})

	t.Run("does not return inactive entries", func(t *testing.T) {
		result, err := service.GetPortalUIRegistryForUser([]string{})
		require.NoError(t, err)
		for _, entry := range result {
			assert.NotEqual(t, "inactive-page", entry.ComponentTag)
		}
	})
}

func TestGetPortalSidebarMenuItemsForUser(t *testing.T) {
	service, db := setupManifestServiceTest(t)
	plugin := createTestPluginForManifest(t, db, "portal-sidebar-test")

	// Create portal sidebar entries
	entries := []models.UIRegistry{
		{
			PluginID: plugin.ID, SlotType: "portal_sidebar.section",
			RoutePattern: "/portal/plugins/feedback", ComponentTag: "feedback-form",
			EntryPoint: "/ui/feedback.js", IsActive: true, Scope: "portal",
			AllowedGroups: []string{},
			MountConfig: map[string]interface{}{
				"title": "Send Feedback", "label": "Feedback", "icon": "star",
			},
		},
		{
			PluginID: plugin.ID, SlotType: "portal_sidebar.section",
			RoutePattern: "/portal/plugins/premium", ComponentTag: "premium-page",
			EntryPoint: "/ui/premium.js", IsActive: true, Scope: "portal",
			AllowedGroups: []string{"premium"},
			MountConfig: map[string]interface{}{
				"title": "Premium Features", "label": "Premium", "icon": "crown",
			},
		},
	}
	for i := range entries {
		require.NoError(t, db.Create(&entries[i]).Error)
	}

	t.Run("non-premium user sees only feedback", func(t *testing.T) {
		items, err := service.GetPortalSidebarMenuItemsForUser([]string{})
		require.NoError(t, err)
		assert.Len(t, items, 1)
		assert.Equal(t, "Feedback", items[0].Label)
		assert.Len(t, items[0].SubItems, 1)
		assert.Equal(t, "Send Feedback", items[0].SubItems[0].Text)
	})

	t.Run("premium user sees both sections", func(t *testing.T) {
		items, err := service.GetPortalSidebarMenuItemsForUser([]string{"premium"})
		require.NoError(t, err)
		// Both entries are for the same plugin, so they're grouped into one section
		// But they have different labels so they appear as separate sub-items
		totalSubItems := 0
		for _, item := range items {
			totalSubItems += len(item.SubItems)
		}
		assert.Equal(t, 2, totalSubItems)
	})

	t.Run("sub-items have correct paths", func(t *testing.T) {
		items, err := service.GetPortalSidebarMenuItemsForUser([]string{"premium"})
		require.NoError(t, err)

		paths := make(map[string]bool)
		for _, item := range items {
			for _, sub := range item.SubItems {
				paths[sub.Path] = true
			}
		}
		assert.True(t, paths["/portal/plugins/feedback"])
		assert.True(t, paths["/portal/plugins/premium"])
	})
}

func TestGetUIRegistry_ExcludesPortalEntries(t *testing.T) {
	service, db := setupManifestServiceTest(t)
	plugin := createTestPluginForManifest(t, db, "scope-separation-test")

	// Create both admin and portal entries
	adminEntry := models.UIRegistry{
		PluginID: plugin.ID, SlotType: "sidebar.section",
		RoutePattern: "/admin/test", ComponentTag: "admin-comp",
		IsActive: true, Scope: "admin",
	}
	portalEntry := models.UIRegistry{
		PluginID: plugin.ID, SlotType: "portal_sidebar.section",
		RoutePattern: "/portal/plugins/test", ComponentTag: "portal-comp",
		IsActive: true, Scope: "portal",
	}
	require.NoError(t, db.Create(&adminEntry).Error)
	require.NoError(t, db.Create(&portalEntry).Error)

	// Admin UI registry should NOT include portal entries
	result, err := service.GetUIRegistry()
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "admin-comp", result[0].ComponentTag)
}
