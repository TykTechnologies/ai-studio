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
	"github.com/TykTechnologies/midsommar/v2/services"
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

// role_ids are validated before the user exists: an unknown role (422) or
// the Owner role for a service account (409) is refused and leaves no
// account (and no unseen key) behind.
func TestUserAPI_RoleIDsValidatedBeforeCreate(t *testing.T) {
	f := setupRBACFixture(t)
	for name, attrs := range map[string]map[string]any{
		"unknown role":          {"name": "Bot", "account_type": models.AccountTypeService, "role_ids": []uint{99999}},
		"unknown role (person)": {"email": "new@example.com", "name": "New", "password": "Sup3r-secret-pw!", "role_ids": []uint{99999}},
	} {
		t.Run(name, func(t *testing.T) {
			w := f.do("POST", "/api/v1/users", map[string]any{"data": map[string]any{"type": "users", "attributes": attrs}}, f.owner)
			assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
		})
	}
	// Owner for a service account is one of the Owner rules: 409, code
	// service_account.
	w := f.do("POST", "/api/v1/users", map[string]any{"data": map[string]any{"type": "users", "attributes": map[string]any{
		"name": "Bot", "account_type": models.AccountTypeService, "role_ids": []uint{f.roles[models.SystemRoleOwner]},
	}}}, f.owner)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, services.ServiceAccountCode, decodeErrorCode(t, w.Body.Bytes()))
	var n int64
	require.NoError(t, f.api.service.DB.Model(&models.User{}).
		Where("email = ? OR name = ?", "new@example.com", "Bot").Count(&n).Error)
	assert.Zero(t, n, "no user is left behind")
}

// ?account_type= filters the users list to service accounts or to people.
func TestUserAPI_ListFiltersByAccountType(t *testing.T) {
	f := setupRBACFixture(t)
	w := f.do("POST", "/api/v1/users", serviceAccountBody(nil), f.owner)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	types := func(query string) map[string]int {
		w := f.do("GET", "/api/v1/users?all=true"+query, nil, f.owner)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var list struct{ Data []UserResponse }
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
		out := map[string]int{}
		for _, u := range list.Data {
			out[u.Attributes.AccountType]++
		}
		return out
	}
	all := types("")
	assert.Equal(t, 1, all[models.AccountTypeService])
	assert.Positive(t, all[models.AccountTypeUser])
	assert.Equal(t, map[string]int{models.AccountTypeService: 1}, types("&account_type=service"))
	people := types("&account_type=user")
	assert.Zero(t, people[models.AccountTypeService])
	assert.Equal(t, all[models.AccountTypeUser], people[models.AccountTypeUser])
	// Any other value is no filter: the list it answers is the one without
	// the parameter, never more.
	assert.Equal(t, all, types("&account_type=bogus"))
	assert.Equal(t, all, types("&account_type='%20OR%201=1--"))
}
