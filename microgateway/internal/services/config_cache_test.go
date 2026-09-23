package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The plugin lookup runs several times per request and is cached. A plugin
// associated with an LLM after the first lookup must be seen by the next one:
// the cache is invalidated by the write, not by waiting out a TTL.
func TestPluginService_GetPluginsForLLM_CacheSeesNewAssociation(t *testing.T) {
	db := setupTestDB(t)
	svc := NewPluginService(db, database.NewRepository(db))

	llm := &database.LLM{Name: "l", Slug: "l", Vendor: "openai", IsActive: true}
	require.NoError(t, db.Create(llm).Error)

	got, err := svc.GetPluginsForLLM(llm.ID)
	require.NoError(t, err)
	assert.Empty(t, got)

	plugin := &database.Plugin{Name: "p", Command: "./p", HookType: "post_auth", IsActive: true}
	require.NoError(t, db.Create(plugin).Error)
	require.NoError(t, db.Create(&database.LLMPlugin{LLMID: llm.ID, PluginID: plugin.ID, IsActive: true}).Error)

	got, err = svc.GetPluginsForLLM(llm.ID)
	require.NoError(t, err)
	require.Len(t, got, 1, "a new association must be visible immediately")
	assert.Equal(t, plugin.ID, got[0].ID)

	// Deactivating the association is also seen at once.
	require.NoError(t, db.Model(&database.LLMPlugin{}).Where("llm_id = ?", llm.ID).Update("is_active", false).Error)
	got, err = svc.GetPluginsForLLM(llm.ID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// Callers receive their own slice, so one caller cannot change what the next
// request sees.
func TestPluginService_GetPluginsForLLM_ReturnsCopies(t *testing.T) {
	db := setupTestDB(t)
	svc := NewPluginService(db, database.NewRepository(db))
	llm := &database.LLM{Name: "l", Slug: "l", Vendor: "openai", IsActive: true}
	require.NoError(t, db.Create(llm).Error)
	plugin := &database.Plugin{Name: "p", Command: "./p", HookType: "post_auth", IsActive: true}
	require.NoError(t, db.Create(plugin).Error)
	require.NoError(t, db.Create(&database.LLMPlugin{LLMID: llm.ID, PluginID: plugin.ID, IsActive: true}).Error)

	first, err := svc.GetPluginsForLLM(llm.ID)
	require.NoError(t, err)
	first[0].Name = "mutated"

	second, err := svc.GetPluginsForLLM(llm.ID)
	require.NoError(t, err)
	assert.Equal(t, "p", second[0].Name)
}

// The app is looked up on every authenticated request and is cached. A change
// to the app's LLM access (a config sync, an admin edit) must take effect on
// the next request.
func TestHybridGatewayService_GetAppByTokenID_CacheSeesAccessChange(t *testing.T) {
	db, repo := setupHybridTestDB(t)
	h := createTestHybridService(t, db, repo)

	app := &database.App{Name: "a", IsActive: true}
	require.NoError(t, db.Create(app).Error)
	llm := &database.LLM{Name: "l", Slug: "l", Vendor: "openai", IsActive: true}
	require.NoError(t, db.Create(llm).Error)

	got, err := h.GetAppByTokenID(app.ID)
	require.NoError(t, err)
	assert.Empty(t, got.LLMs)

	require.NoError(t, db.Create(&database.AppLLM{AppID: app.ID, LLMID: llm.ID, IsActive: true}).Error)
	got, err = h.GetAppByTokenID(app.ID)
	require.NoError(t, err)
	require.Len(t, got.LLMs, 1, "granting an LLM must be visible on the next request")

	require.NoError(t, db.Exec("DELETE FROM app_llms WHERE app_id = ?", app.ID).Error)
	got, err = h.GetAppByTokenID(app.ID)
	require.NoError(t, err)
	assert.Empty(t, got.LLMs, "revoking an LLM must be visible on the next request")
}

// Analytics and budget writes happen on every request; they must not empty
// the configuration caches, or caching would buy nothing under load.
func TestHybridGatewayService_GetAppByTokenID_RuntimeWritesKeepCache(t *testing.T) {
	db, repo := setupHybridTestDB(t)
	h := createTestHybridService(t, db, repo)
	app := &database.App{Name: "a", IsActive: true}
	require.NoError(t, db.Create(app).Error)

	_, err := h.GetAppByTokenID(app.ID)
	require.NoError(t, err)
	gen := database.ConfigGeneration()

	require.NoError(t, db.Create(&database.AnalyticsEvent{RequestID: "r", AppID: app.ID}).Error)
	require.NoError(t, db.Create(&database.BudgetUsage{AppID: app.ID}).Error)
	assert.Equal(t, gen, database.ConfigGeneration())

	_, cached := h.apps.Get(app.ID)
	assert.True(t, cached, "the app entry should survive runtime writes")
}
