package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppGuardrailCapabilityHandlers(t *testing.T) {
	api, db := setupTestAPI(t)
	app := &models.App{Name: "Visor", UserID: 1}
	require.NoError(t, app.Create(db))
	path := fmt.Sprintf("/api/v1/apps/%d/guardrail-capability", app.ID)

	decode := func(body []byte) AppGuardrailCapabilityResponse {
		var resp struct {
			Data AppGuardrailCapabilityResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &resp), string(body))
		return resp.Data
	}

	w := performRequest(api.router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got := decode(w.Body.Bytes())
	assert.Equal(t, "", got.Value, "unattested")
	assert.Equal(t, "acting", got.Effective, "unattested is treated as acting")
	assert.Equal(t, "read_only", got.Suggestion)
	assert.NotEmpty(t, got.Reasons)

	w = performRequest(api.router, http.MethodPut, path, map[string]string{"value": "root"})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	w = performRequest(api.router, http.MethodPut, path, map[string]string{})
	assert.Equal(t, http.StatusBadRequest, w.Code, "value is required")

	w = performRequest(api.router, http.MethodPut, path, map[string]string{"value": "read_only"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got = decode(w.Body.Bytes())
	assert.Equal(t, "read_only", got.Value)
	assert.Equal(t, "read_only", got.Effective)

	// The app itself reports it.
	w = performRequest(api.router, http.MethodGet, fmt.Sprintf("/api/v1/apps/%d", app.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"guardrail_capability":"read_only"`)

	w = performRequest(api.router, http.MethodGet, "/api/v1/apps/99999/guardrail-capability", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Attesting a capability loosens what exceptions may do, so it is for full
// administrators only; reading it is an app read.
func TestAppGuardrailCapabilityRoutes_Permissions(t *testing.T) {
	api, _ := setupTestAPI(t)
	get := func(method, path string) authz.Permission {
		e, ok := api.routePerms[routeKey(method, path)]
		require.True(t, ok, "%s %s", method, path)
		return e.perm
	}
	assert.Equal(t, authz.FullAdmin, get("PUT", "/api/v1/apps/:id/guardrail-capability"))
	assert.Equal(t, authz.Read("apps"), get("GET", "/api/v1/apps/:id/guardrail-capability"))
}

func TestAppGuardrailCapabilityRoutes_NonAdminDenied(t *testing.T) {
	api, _, member := setupAuthzAPI(t)
	w := apitest.PerformAuthRequest(api.router, http.MethodPut, "/api/v1/apps/1/guardrail-capability", map[string]string{"value": "read_only"}, member.APIKey)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
