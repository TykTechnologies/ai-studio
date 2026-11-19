package api

import (
	"encoding/json"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The permission catalogue carries the Workspaces resource and each
// resource's scope kind, in both editions.
func TestAuthzMiddleware_PermissionCatalogueScopes(t *testing.T) {
	api, admin, _ := setupAuthzAPI(t)
	w := apitest.PerformAuthRequest(api.router, "GET", "/api/v1/rbac/permissions", nil, admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code)
	var resp PermissionCatalogueResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	byKey := map[string]authz.Resource{}
	for _, r := range resp.Resources {
		byKey[r.Key] = r
	}
	ws, ok := byKey["workspaces"]
	require.True(t, ok, "workspaces is in the catalogue")
	assert.Equal(t, "Access", ws.Group)
	assert.Equal(t, authz.ScopeGlobal, ws.Scope)
	assert.Equal(t, authz.ScopeWorkspaceWrite, byKey["llms"].Scope)
	assert.Equal(t, authz.ScopeWorkspaceFull, byKey["apps"].Scope)
	assert.Equal(t, authz.ScopeGlobal, byKey["plugins"].Scope)
	assert.Equal(t, authz.ScopeWorkspaceParent, byKey["credentials"].Scope)
	assert.Equal(t, "apps", byKey["credentials"].ScopeParent)
}
