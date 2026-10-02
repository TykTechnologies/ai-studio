package services

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDBForEndpointAuthPlugins(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	return db
}

func createAuthTestPlugin(t *testing.T, db *gorm.DB, name, namespace string, hooks ...string) *models.Plugin {
	p := &models.Plugin{
		Name: name, Command: "file:///" + name, HookType: hooks[0], HookTypes: hooks,
		IsActive: true, Namespace: namespace,
	}
	require.NoError(t, db.Create(p).Error)
	return p
}

func TestEndpointAuthPlugins_SetGetAndValidate(t *testing.T) {
	db := setupTestDBForEndpointAuthPlugins(t)
	svc := NewPluginService(db)

	ds := &models.Datasource{Name: "Docs", Active: true, Namespace: "team-a"}
	require.NoError(t, db.Create(ds).Error)
	idp := createAuthTestPlugin(t, db, "idp", "", "auth")
	hybrid := createAuthTestPlugin(t, db, "hybrid", "team-a", "post_auth", "auth")
	notAuth := createAuthTestPlugin(t, db, "logger", "", "post_auth")
	otherNS := createAuthTestPlugin(t, db, "elsewhere", "team-b", "auth")

	require.NoError(t, svc.SetEndpointAuthPlugins(models.EndpointTypeDatasource, ds.ID, []uint{hybrid.ID, idp.ID}))
	got, err := svc.GetEndpointAuthPlugins(models.EndpointTypeDatasource, ds.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, hybrid.ID, got[0].ID, "order is kept; a plugin whose auth hook is not primary counts")
	assert.Equal(t, idp.ID, got[1].ID)

	for name, ids := range map[string][]uint{
		"no auth hook":    {notAuth.ID},
		"other namespace": {otherNS.ID},
		"missing":         {99999},
		"listed twice":    {idp.ID, idp.ID},
	} {
		t.Run(name, func(t *testing.T) {
			err := svc.SetEndpointAuthPlugins(models.EndpointTypeDatasource, ds.ID, ids)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidEndpointAuthPlugin), err)
		})
	}
	got, err = svc.GetEndpointAuthPlugins(models.EndpointTypeDatasource, ds.ID)
	require.NoError(t, err)
	assert.Len(t, got, 2, "a refused update leaves the list as it was")

	require.NoError(t, svc.SetEndpointAuthPlugins(models.EndpointTypeDatasource, ds.ID, nil))
	got, err = svc.GetEndpointAuthPlugins(models.EndpointTypeDatasource, ds.ID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestEndpointAuthPlugins_UnknownEndpoint(t *testing.T) {
	db := setupTestDBForEndpointAuthPlugins(t)
	svc := NewPluginService(db)
	idp := createAuthTestPlugin(t, db, "idp", "", "auth")
	notEndpoint := createAuthTestPlugin(t, db, "not-an-endpoint", "", "auth")

	for name, call := range map[string]func() error{
		"missing datasource": func() error {
			return svc.SetEndpointAuthPlugins(models.EndpointTypeDatasource, 404, []uint{idp.ID})
		},
		"unknown type": func() error { return svc.SetEndpointAuthPlugins("llm", 1, []uint{idp.ID}) },
		"plugin without custom endpoints": func() error {
			return svc.SetEndpointAuthPlugins(models.EndpointTypePlugin, notEndpoint.ID, []uint{idp.ID})
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrEndpointNotFound), err)
		})
	}
}

// Deleting the endpoint or the plugin removes the attachment, whichever path
// deletes it (the REST handlers and the plugin SDK share these deletes).
func TestEndpointAuthPlugins_DeleteCleansUp(t *testing.T) {
	db := setupTestDBForEndpointAuthPlugins(t)
	pluginSvc := NewPluginService(db)
	svc := NewService(db)
	idp := createAuthTestPlugin(t, db, "idp", "", "auth")
	endpoint := createAuthTestPlugin(t, db, "my-endpoint", "", "custom_endpoint")

	ds := &models.Datasource{Name: "Docs", Active: true}
	require.NoError(t, db.Create(ds).Error)
	tool := &models.Tool{Name: "CRM", Active: true}
	require.NoError(t, db.Create(tool).Error)
	router := &models.ModelRouter{Name: "Smart", Slug: "smart", Active: true}
	require.NoError(t, db.Create(router).Error)
	sem := &models.SemanticRouter{Name: "Sem", Slug: "sem", Active: true}
	require.NoError(t, db.Create(sem).Error)

	for typ, id := range map[string]uint{
		models.EndpointTypeDatasource:     ds.ID,
		models.EndpointTypeTool:           tool.ID,
		models.EndpointTypeModelRouter:    router.ID,
		models.EndpointTypeSemanticRouter: sem.ID,
		models.EndpointTypePlugin:         endpoint.ID,
	} {
		require.NoError(t, pluginSvc.SetEndpointAuthPlugins(typ, id, []uint{idp.ID}), typ)
	}
	count := func(where string, args ...interface{}) int64 {
		var n int64
		require.NoError(t, db.Model(&models.EndpointAuthPlugin{}).Where(where, args...).Count(&n).Error)
		return n
	}
	require.EqualValues(t, 5, count("1 = 1"))

	require.NoError(t, svc.DeleteDatasource(ds.ID))
	require.NoError(t, svc.DeleteTool(tool.ID))
	require.NoError(t, router.Delete(db))
	require.NoError(t, sem.Delete(db))
	assert.EqualValues(t, 1, count("1 = 1"), "only the plugin endpoint's list is left")

	require.NoError(t, pluginSvc.DeletePlugin(endpoint.ID))
	assert.Zero(t, count("1 = 1"))

	// And a deleted auth plugin drops off every list.
	ds2 := &models.Datasource{Name: "Docs 2", Active: true}
	require.NoError(t, db.Create(ds2).Error)
	require.NoError(t, pluginSvc.SetEndpointAuthPlugins(models.EndpointTypeDatasource, ds2.ID, []uint{idp.ID}))
	require.NoError(t, pluginSvc.DeletePlugin(idp.ID))
	assert.Zero(t, count("1 = 1"))
}
