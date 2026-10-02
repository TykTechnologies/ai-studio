package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndpointAuthPluginRoutes(t *testing.T) {
	api, db := setupTestAPI(t)

	idp := &models.Plugin{Name: "idp", Command: "file:///idp", HookType: models.HookTypeAuth, IsActive: true}
	require.NoError(t, db.Create(idp).Error)
	logger := &models.Plugin{Name: "logger", Command: "file:///logger", HookType: models.HookTypePostAuth, IsActive: true}
	require.NoError(t, db.Create(logger).Error)
	ds := &models.Datasource{Name: "Docs", Active: true}
	require.NoError(t, db.Create(ds).Error)
	tool := &models.Tool{Name: "CRM", Active: true}
	require.NoError(t, db.Create(tool).Error)

	for _, base := range []string{
		fmt.Sprintf("/api/v1/datasources/%d/auth-plugins", ds.ID),
		fmt.Sprintf("/api/v1/tools/%d/auth-plugins", tool.ID),
	} {
		t.Run(base, func(t *testing.T) {
			w := performRequest(api.router, http.MethodPut, base, EndpointAuthPluginsRequest{PluginIDs: []uint{idp.ID}})
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			w = performRequest(api.router, http.MethodGet, base, nil)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var resp struct {
				Data []PluginResponse `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Data, 1)
			assert.Equal(t, fmt.Sprint(idp.ID), resp.Data[0].ID)

			w = performRequest(api.router, http.MethodPut, base, EndpointAuthPluginsRequest{PluginIDs: []uint{logger.ID}})
			assert.Equal(t, http.StatusBadRequest, w.Code, "a plugin without the auth hook; %s", w.Body.String())
		})
	}

	w := performRequest(api.router, http.MethodGet, "/api/v1/datasources/9999/auth-plugins", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = performRequest(api.router, http.MethodPut, "/api/v1/datasources/abc/auth-plugins", EndpointAuthPluginsRequest{})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
