//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
)

// A caller with users:write but not roles:write who sends role_ids is
// refused before the user is created: nothing is left behind (both account
// types).
func TestUserAPI_RoleIDsWithoutRolesWriteCreatesNoUser(t *testing.T) {
	f := setupRBACFixture(t)
	w := f.do("POST", "/api/v1/rbac/roles", RoleInput{
		Name: "User manager", Permissions: []string{"users:read", "users:write"},
	}, f.owner)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var role struct{ Data RoleResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &role))
	f.bind(t, "user", f.viewer.ID, uintFromString(t, role.Data.ID))

	for name, attrs := range map[string]map[string]any{
		"person":          {"email": "new@example.com", "name": "New", "password": "Sup3r-secret-pw!"},
		"service account": {"name": "Bot", "account_type": models.AccountTypeService},
	} {
		t.Run(name, func(t *testing.T) {
			attrs["role_ids"] = []uint{f.roles[models.SystemRoleViewer]}
			w := f.do("POST", "/api/v1/users", map[string]any{"data": map[string]any{"type": "users", "attributes": attrs}}, f.viewer)
			assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			assert.Equal(t, authz.CodePermissionDenied, decodeErrorCode(t, w.Body.Bytes()))
		})
	}
	var n int64
	require.NoError(t, f.api.service.DB.Model(&models.User{}).
		Where("email = ? OR name = ?", "new@example.com", "Bot").Count(&n).Error)
	assert.Zero(t, n, "no user is left behind")

	// Without role_ids the same caller can create a service account, and the
	// owner can give it a role on the same request.
	w = f.do("POST", "/api/v1/users", serviceAccountBody(nil), f.viewer)
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	w = f.do("POST", "/api/v1/users", serviceAccountBody(map[string]any{
		"name": "Bot 2", "role_ids": []uint{f.roles[models.SystemRoleViewer]},
	}), f.owner)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct{ Data UserResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	key := created.Data.Attributes.APIKey
	require.NotEmpty(t, key)

	// Its key reaches /api/v1 with exactly that role.
	w = f.do("GET", "/api/v1/llms", nil, &models.User{APIKey: key})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = f.do("POST", "/api/v1/llms", map[string]any{"data": map[string]any{"attributes": map[string]any{"name": "x"}}}, &models.User{APIKey: key})
	assert.Equal(t, http.StatusForbidden, w.Code)
}
