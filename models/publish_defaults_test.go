package models

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The publish guards in the API rely on the live switch of LLMs, datasources,
// model routers and plugins being false when a create request omits it (the
// request structs use a plain bool). A GORM `default:true` on any of these
// columns would flip an omitted switch to live on insert and bypass the
// publish permission, so this test pins the invariant. Tools, apps and
// agents do default to true; their handlers deactivate explicitly.
func TestPublishableCreateDefaultsToInactive(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&LLM{}, &Datasource{}, &ModelRouter{}, &Plugin{}))

	llm := &LLM{Name: "l"}
	require.NoError(t, db.Create(llm).Error)
	var gotLLM LLM
	require.NoError(t, db.First(&gotLLM, llm.ID).Error)
	require.False(t, gotLLM.Active, "LLM.Active must not default to true")

	ds := &Datasource{Name: "d"}
	require.NoError(t, db.Create(ds).Error)
	var gotDS Datasource
	require.NoError(t, db.First(&gotDS, ds.ID).Error)
	require.False(t, gotDS.Active, "Datasource.Active must not default to true")

	router := &ModelRouter{Name: "r", Slug: "r"}
	require.NoError(t, db.Create(router).Error)
	var gotRouter ModelRouter
	require.NoError(t, db.First(&gotRouter, router.ID).Error)
	require.False(t, gotRouter.Active, "ModelRouter.Active must not default to true")

	plugin := &Plugin{Name: "p", Command: "/bin/true", HookType: HookTypeStudioUI}
	require.NoError(t, db.Create(plugin).Error)
	var gotPlugin Plugin
	require.NoError(t, db.First(&gotPlugin, plugin.ID).Error)
	require.False(t, gotPlugin.IsActive, "Plugin.IsActive must not default to true")
}
