package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupLLMPluginTest(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = InitModels(db)
	assert.NoError(t, err)

	return db
}

func createTestLLMForPlugin(t *testing.T, db *gorm.DB, name string) *LLM {
	llm := &LLM{
		Name:             name,
		Vendor:           OPENAI,
		ShortDescription: "Test LLM",
		DefaultModel:     "gpt-4",
		Active:           true,
	}
	db.Create(llm)
	return llm
}

func createTestPluginForLLM(t *testing.T, db *gorm.DB, name string) *Plugin {
	plugin := &Plugin{
		Name:     name,
		Command:  "/bin/test-" + name,
		HookType: "post_auth",
		IsActive: true,
	}
	db.Create(plugin)
	return plugin
}

func TestNewLLMPlugin(t *testing.T) {
	t.Run("Create new LLM plugin association", func(t *testing.T) {
		lp := NewLLMPlugin()
		assert.NotNil(t, lp)
		assert.True(t, lp.IsActive)
		assert.NotNil(t, lp.ConfigOverride)
		assert.Len(t, lp.ConfigOverride, 0)
	})
}

func TestLLMPlugin_Create(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm := createTestLLMForPlugin(t, db, "test-llm")
	plugin := createTestPluginForLLM(t, db, "test-plugin")

	t.Run("Create LLM-Plugin association", func(t *testing.T) {
		lp := &LLMPlugin{
			LLMID:          llm.ID,
			PluginID:       plugin.ID,
			OrderIndex:     0,
			ConfigOverride: map[string]interface{}{"key": "value"},
		}

		err := lp.Create(db)
		assert.NoError(t, err)
	})

	t.Run("Create duplicate association fails", func(t *testing.T) {
		lp := &LLMPlugin{
			LLMID:    llm.ID,
			PluginID: plugin.ID,
		}

		err := lp.Create(db)
		assert.Error(t, err) // Should fail due to primary key constraint
	})
}

func TestLLMPlugin_Get(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm := createTestLLMForPlugin(t, db, "get-llm")
	plugin := createTestPluginForLLM(t, db, "get-plugin")

	lp := &LLMPlugin{
		LLMID:    llm.ID,
		PluginID: plugin.ID,
	}
	lp.Create(db)

	t.Run("Get existing association", func(t *testing.T) {
		retrieved := &LLMPlugin{}
		err := retrieved.Get(db, llm.ID, plugin.ID)
		assert.NoError(t, err)
		assert.Equal(t, llm.ID, retrieved.LLMID)
		assert.Equal(t, plugin.ID, retrieved.PluginID)
		assert.NotNil(t, retrieved.LLM)
		assert.NotNil(t, retrieved.Plugin)
	})

	t.Run("Get non-existent association", func(t *testing.T) {
		retrieved := &LLMPlugin{}
		err := retrieved.Get(db, 99999, 99999)
		assert.Error(t, err)
	})
}

func TestLLMPlugin_Update(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm := createTestLLMForPlugin(t, db, "update-llm")
	plugin := createTestPluginForLLM(t, db, "update-plugin")

	lp := &LLMPlugin{
		LLMID:      llm.ID,
		PluginID:   plugin.ID,
		OrderIndex: 0,
	}
	lp.Create(db)

	t.Run("Update association", func(t *testing.T) {
		lp.OrderIndex = 5
		lp.ConfigOverride = map[string]interface{}{"updated": true}

		err := lp.Update(db)
		assert.NoError(t, err)

		// Verify update
		retrieved := &LLMPlugin{}
		retrieved.Get(db, llm.ID, plugin.ID)
		assert.Equal(t, 5, retrieved.OrderIndex)
	})
}

func TestLLMPlugin_Delete(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm := createTestLLMForPlugin(t, db, "delete-llm")
	plugin := createTestPluginForLLM(t, db, "delete-plugin")

	lp := &LLMPlugin{
		LLMID:    llm.ID,
		PluginID: plugin.ID,
	}
	lp.Create(db)

	t.Run("Delete association", func(t *testing.T) {
		err := lp.Delete(db)
		assert.NoError(t, err)

		// Verify deletion
		retrieved := &LLMPlugin{}
		err = retrieved.Get(db, llm.ID, plugin.ID)
		assert.Error(t, err)
	})
}

func TestLLMPlugin_UpdateOrder(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm := createTestLLMForPlugin(t, db, "order-llm")
	plugin := createTestPluginForLLM(t, db, "order-plugin")

	lp := &LLMPlugin{
		LLMID:      llm.ID,
		PluginID:   plugin.ID,
		OrderIndex: 0,
	}
	lp.Create(db)

	t.Run("Update order index", func(t *testing.T) {
		err := lp.UpdateOrder(db, 10)
		assert.NoError(t, err)
		assert.Equal(t, 10, lp.OrderIndex)

		// Verify in database
		retrieved := &LLMPlugin{}
		retrieved.Get(db, llm.ID, plugin.ID)
		assert.Equal(t, 10, retrieved.OrderIndex)
	})
}

func TestLLMPlugin_UpdateConfig(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm := createTestLLMForPlugin(t, db, "config-llm")
	plugin := createTestPluginForLLM(t, db, "config-plugin")

	lp := &LLMPlugin{
		LLMID:    llm.ID,
		PluginID: plugin.ID,
	}
	lp.Create(db)

	t.Run("Update config override", func(t *testing.T) {
		newConfig := map[string]interface{}{
			"api_key":  "test-key",
			"endpoint": "https://api.test.com",
		}

		err := lp.UpdateConfig(db, newConfig)
		assert.NoError(t, err)

		// Verify in database
		retrieved := &LLMPlugin{}
		retrieved.Get(db, llm.ID, plugin.ID)
		assert.Equal(t, "test-key", retrieved.ConfigOverride["api_key"])
	})
}

func TestLLMPlugin_ActivateDeactivate(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm := createTestLLMForPlugin(t, db, "active-llm")
	plugin := createTestPluginForLLM(t, db, "active-plugin")

	lp := &LLMPlugin{
		LLMID:    llm.ID,
		PluginID: plugin.ID,
		IsActive: true,
	}
	lp.Create(db)

	t.Run("Deactivate association", func(t *testing.T) {
		err := lp.Deactivate(db)
		assert.NoError(t, err)
		assert.False(t, lp.IsActive)

		retrieved := &LLMPlugin{}
		retrieved.Get(db, llm.ID, plugin.ID)
		assert.False(t, retrieved.IsActive)
	})

	t.Run("Activate association", func(t *testing.T) {
		err := lp.Activate(db)
		assert.NoError(t, err)
		assert.True(t, lp.IsActive)

		retrieved := &LLMPlugin{}
		retrieved.Get(db, llm.ID, plugin.ID)
		assert.True(t, retrieved.IsActive)
	})
}

func TestLLMPlugins_GetPluginsForLLM(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm := createTestLLMForPlugin(t, db, "multi-llm")
	plugin1 := createTestPluginForLLM(t, db, "plugin1")
	plugin2 := createTestPluginForLLM(t, db, "plugin2")
	plugin3 := createTestPluginForLLM(t, db, "plugin3")

	// Create associations with different orders
	lp1 := &LLMPlugin{LLMID: llm.ID, PluginID: plugin1.ID, OrderIndex: 2, IsActive: true}
	lp2 := &LLMPlugin{LLMID: llm.ID, PluginID: plugin2.ID, OrderIndex: 1, IsActive: true}
	lp3 := &LLMPlugin{LLMID: llm.ID, PluginID: plugin3.ID, OrderIndex: 3, IsActive: true}
	lp1.Create(db)
	lp2.Create(db)
	lp3.Create(db)

	// Deactivate lp3 after creation
	lp3.Deactivate(db)

	t.Run("Get plugins for LLM ordered", func(t *testing.T) {
		var llmPlugins LLMPlugins
		err := llmPlugins.GetPluginsForLLM(db, llm.ID)
		assert.NoError(t, err)
		assert.Len(t, llmPlugins, 2) // Only active plugins

		// Verify order
		assert.Equal(t, plugin2.ID, llmPlugins[0].PluginID) // OrderIndex 1
		assert.Equal(t, plugin1.ID, llmPlugins[1].PluginID) // OrderIndex 2
	})
}

func TestLLMPlugins_GetLLMsForPlugin(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm1 := createTestLLMForPlugin(t, db, "llm1")
	llm2 := createTestLLMForPlugin(t, db, "llm2")
	plugin := createTestPluginForLLM(t, db, "multi-plugin")

	// Create associations
	lp1 := &LLMPlugin{LLMID: llm1.ID, PluginID: plugin.ID, OrderIndex: 0, IsActive: true}
	lp2 := &LLMPlugin{LLMID: llm2.ID, PluginID: plugin.ID, OrderIndex: 1, IsActive: true}
	lp1.Create(db)
	lp2.Create(db)

	t.Run("Get LLMs for plugin", func(t *testing.T) {
		var llmPlugins LLMPlugins
		err := llmPlugins.GetLLMsForPlugin(db, plugin.ID)
		assert.NoError(t, err)
		assert.Len(t, llmPlugins, 2)
	})
}

func TestLLMPlugins_GetActiveAssociations(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm := createTestLLMForPlugin(t, db, "active-llm")
	plugin := createTestPluginForLLM(t, db, "active-plugin")

	lp1 := &LLMPlugin{LLMID: llm.ID, PluginID: plugin.ID, IsActive: true}
	lp1.Create(db)

	t.Run("Get only active associations", func(t *testing.T) {
		var llmPlugins LLMPlugins
		err := llmPlugins.GetActiveAssociations(db)
		assert.NoError(t, err)
		// Should only include active associations
		for _, lp := range llmPlugins {
			assert.True(t, lp.IsActive)
		}
	})
}

func TestDeleteAssociationsForLLM(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm := createTestLLMForPlugin(t, db, "del-llm")
	plugin1 := createTestPluginForLLM(t, db, "del-plugin1")
	plugin2 := createTestPluginForLLM(t, db, "del-plugin2")

	lp1 := &LLMPlugin{LLMID: llm.ID, PluginID: plugin1.ID}
	lp2 := &LLMPlugin{LLMID: llm.ID, PluginID: plugin2.ID}
	lp1.Create(db)
	lp2.Create(db)

	t.Run("Delete all associations for LLM", func(t *testing.T) {
		err := DeleteAssociationsForLLM(db, llm.ID)
		assert.NoError(t, err)

		// Verify all associations are deleted
		var llmPlugins LLMPlugins
		llmPlugins.GetPluginsForLLM(db, llm.ID)
		assert.Len(t, llmPlugins, 0)
	})
}

func TestDeleteAssociationsForPlugin(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm1 := createTestLLMForPlugin(t, db, "del-llm1")
	llm2 := createTestLLMForPlugin(t, db, "del-llm2")
	plugin := createTestPluginForLLM(t, db, "del-plugin")

	lp1 := &LLMPlugin{LLMID: llm1.ID, PluginID: plugin.ID}
	lp2 := &LLMPlugin{LLMID: llm2.ID, PluginID: plugin.ID}
	lp1.Create(db)
	lp2.Create(db)

	t.Run("Delete all associations for plugin", func(t *testing.T) {
		err := DeleteAssociationsForPlugin(db, plugin.ID)
		assert.NoError(t, err)

		// Verify all associations are deleted
		var llmPlugins LLMPlugins
		llmPlugins.GetLLMsForPlugin(db, plugin.ID)
		assert.Len(t, llmPlugins, 0)
	})
}

func TestUpdatePluginOrder(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm := createTestLLMForPlugin(t, db, "order-llm")
	plugin1 := createTestPluginForLLM(t, db, "order-plugin1")
	plugin2 := createTestPluginForLLM(t, db, "order-plugin2")
	plugin3 := createTestPluginForLLM(t, db, "order-plugin3")

	// Create initial associations
	lp1 := &LLMPlugin{LLMID: llm.ID, PluginID: plugin1.ID, OrderIndex: 0}
	lp2 := &LLMPlugin{LLMID: llm.ID, PluginID: plugin2.ID, OrderIndex: 1}
	lp1.Create(db)
	lp2.Create(db)

	t.Run("Update plugin order", func(t *testing.T) {
		// Reorder: plugin3 first, then plugin2, then plugin1
		newOrder := []uint{plugin3.ID, plugin2.ID, plugin1.ID}
		err := UpdatePluginOrder(db, llm.ID, newOrder)
		assert.NoError(t, err)

		// Verify new order
		var llmPlugins LLMPlugins
		llmPlugins.GetPluginsForLLM(db, llm.ID)
		assert.Len(t, llmPlugins, 3)
		assert.Equal(t, plugin3.ID, llmPlugins[0].PluginID)
		assert.Equal(t, plugin2.ID, llmPlugins[1].PluginID)
		assert.Equal(t, plugin1.ID, llmPlugins[2].PluginID)
	})

	t.Run("Update with empty order removes all associations", func(t *testing.T) {
		llm2 := createTestLLMForPlugin(t, db, "order-llm2")
		plugin4 := createTestPluginForLLM(t, db, "order-plugin4")

		lp := &LLMPlugin{LLMID: llm2.ID, PluginID: plugin4.ID}
		lp.Create(db)

		err := UpdatePluginOrder(db, llm2.ID, []uint{})
		assert.NoError(t, err)

		var llmPlugins LLMPlugins
		llmPlugins.GetPluginsForLLM(db, llm2.ID)
		assert.Len(t, llmPlugins, 0)
	})
}

func TestCountAssociationsForLLM(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm := createTestLLMForPlugin(t, db, "count-llm")
	plugin1 := createTestPluginForLLM(t, db, "count-plugin1")
	plugin2 := createTestPluginForLLM(t, db, "count-plugin2")

	lp1 := &LLMPlugin{LLMID: llm.ID, PluginID: plugin1.ID, IsActive: true}
	lp2 := &LLMPlugin{LLMID: llm.ID, PluginID: plugin2.ID, IsActive: false}
	lp1.Create(db)
	lp2.Create(db)

	t.Run("Count active associations for LLM", func(t *testing.T) {
		count, err := CountAssociationsForLLM(db, llm.ID)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count) // Only active
	})
}

func TestCountAssociationsForPlugin(t *testing.T) {
	db := setupLLMPluginTest(t)

	llm1 := createTestLLMForPlugin(t, db, "count-llm1")
	llm2 := createTestLLMForPlugin(t, db, "count-llm2")
	plugin := createTestPluginForLLM(t, db, "count-plugin-assoc")

	lp1 := &LLMPlugin{LLMID: llm1.ID, PluginID: plugin.ID, IsActive: true}
	lp2 := &LLMPlugin{LLMID: llm2.ID, PluginID: plugin.ID, IsActive: true}
	lp1.Create(db)
	lp2.Create(db)

	t.Run("Count associations for plugin", func(t *testing.T) {
		count, err := CountAssociationsForPlugin(db, plugin.ID)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), count)
	})
}
