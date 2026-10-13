package services

import (
	"fmt"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The section always links to Studio's own plugin page, but never under a
// title one of the plugin's own items already uses (#723: Advanced LLM Cache
// ships its own "Configuration" page, and the sidebar showed two).
func TestSidebarGenericPluginPageLabel(t *testing.T) {
	register := func(t *testing.T, name string, titles ...string) []SidebarSubItem {
		service, db := setupManifestServiceTest(t)
		plugin := createTestPluginForManifest(t, db, name)
		items := make([]models.UISlotItem, 0, len(titles))
		for i, title := range titles {
			tag := fmt.Sprintf("%s-page-%d", name, i)
			items = append(items, models.UISlotItem{
				Type: "route", Path: fmt.Sprintf("/admin/%s/%d", name, i), Title: title,
				Mount: models.UIMount{Kind: "webc", Tag: tag, Entry: "/ui/" + tag + ".js"},
			})
		}
		manifest := &models.PluginManifest{
			ID: "com.test." + name, Version: "1.0.0", Name: name,
			UI: &struct {
				Slots []models.UISlot `json:"slots"`
			}{Slots: []models.UISlot{{Slot: "sidebar.section", Label: name, Items: items}}},
		}
		require.NoError(t, service.RegisterPluginUI(plugin, manifest))
		sections, err := service.GetSidebarMenuItems()
		require.NoError(t, err)
		require.Len(t, sections, 1)
		return sections[0].SubItems
	}
	genericOf := func(t *testing.T, subs []SidebarSubItem) SidebarSubItem {
		for _, s := range subs {
			if strings.HasSuffix(s.ID, "_configuration") {
				return s
			}
		}
		t.Fatalf("no generic plugin page entry in %+v", subs)
		return SidebarSubItem{}
	}
	labelsOf := func(subs []SidebarSubItem) []string {
		out := make([]string, len(subs))
		for i, s := range subs {
			out[i] = s.Text
		}
		return out
	}

	t.Run("no clash keeps Configuration", func(t *testing.T) {
		subs := register(t, "plain", "Dashboard")
		generic := genericOf(t, subs)
		assert.Equal(t, "Configuration", generic.Text)
		assert.Regexp(t, `^/admin/plugins/\d+$`, generic.Path)
		assert.Equal(t, []string{"Dashboard", "Configuration"}, labelsOf(subs))
	})

	t.Run("a plugin page titled Configuration renames the generic entry", func(t *testing.T) {
		subs := register(t, "cache", "Statistics", "Configuration")
		generic := genericOf(t, subs)
		assert.Equal(t, "Plugin settings", generic.Text)
		assert.Regexp(t, `^/admin/plugins/\d+$`, generic.Path)
		assert.Equal(t, []string{"Statistics", "Configuration", "Plugin settings"}, labelsOf(subs))
	})

	t.Run("the comparison ignores case and spaces, and skips every taken label", func(t *testing.T) {
		subs := register(t, "busy", " configuration ", "PLUGIN SETTINGS")
		assert.Equal(t, "Plugin details", genericOf(t, subs).Text)
	})
}

func TestGenericPluginPageLabelFallback(t *testing.T) {
	all := []SidebarSubItem{{Text: "Configuration"}, {Text: "Plugin settings"}, {Text: "Plugin details"}}
	label := genericPluginPageLabel(all)
	for _, it := range all {
		assert.NotEqual(t, it.Text, label)
	}
}
