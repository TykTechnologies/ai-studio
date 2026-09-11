//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End-to-end authorization through the real middleware chain (TestMode off)
// with the Enterprise evaluator: system roles are seeded, a Viewer is
// read-only, an Editor can write resources but not manage access, and the
// role management API enforces the Owner rules.

type rbacFixture struct {
	api    *API
	owner  *models.User
	viewer *models.User
	editor *models.User
	roles  map[string]uint // slug -> id
}

func setupRBACFixture(t *testing.T) *rbacFixture {
	t.Helper()
	api, owner, viewer := setupAuthzAPI(t)
	db := api.service.DB

	editor := models.NewUser()
	editor.Email = "editor@tyk.io"
	editor.Name = "Editor"
	editor.Password = "hash"
	editor.EmailVerified = true
	require.NoError(t, editor.Create(db))

	f := &rbacFixture{api: api, owner: owner, viewer: viewer, editor: editor, roles: map[string]uint{}}

	// Seeding happens lazily on first use; list the roles as the owner.
	w := f.do("GET", "/api/v1/rbac/roles", nil, owner)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list struct{ Data []RoleResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	for _, r := range list.Data {
		f.roles[r.Attributes.Slug] = uintFromString(t, r.ID)
	}
	require.Len(t, f.roles, 5)

	f.bind(t, "user", viewer.ID, f.roles[models.SystemRoleViewer])
	f.bind(t, "user", editor.ID, f.roles[models.SystemRoleEditor])
	return f
}

func (f *rbacFixture) do(method, path string, body interface{}, as *models.User) *httptest.ResponseRecorder {
	return apitest.PerformAuthRequest(f.api.router, method, path, body, as.APIKey)
}

func (f *rbacFixture) bind(t *testing.T, subjectType string, subjectID, roleID uint) uint {
	t.Helper()
	w := f.do("POST", "/api/v1/rbac/bindings", map[string]interface{}{
		"subject_type": subjectType, "subject_id": subjectID, "role_id": roleID,
	}, f.owner)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct{ Data RoleBindingResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return uintFromString(t, resp.Data.ID)
}

func uintFromString(t *testing.T, s string) uint {
	t.Helper()
	var v uint
	_, err := fmt.Sscanf(s, "%d", &v)
	require.NoError(t, err)
	return v
}

func TestRBAC_ViewerIsReadOnly(t *testing.T) {
	f := setupRBACFixture(t)

	w := f.do("GET", "/api/v1/llms", nil, f.viewer)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = f.do("POST", "/api/v1/llms", map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"name": "x"}}}, f.viewer)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, authz.CodePermissionDenied, decodeErrorCode(t, w.Body.Bytes()))
	assert.Contains(t, w.Body.String(), "llms:write")

	// Sensitive reads are withheld from Viewer.
	w = f.do("GET", "/api/v1/audit/records", nil, f.viewer)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = f.do("GET", "/api/v1/analytics/proxy-logs-for-app", nil, f.viewer)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// Admin-surface routes are open to any role holder.
	w = f.do("GET", "/api/v1/rbac/permissions", nil, f.viewer)
	assert.Equal(t, http.StatusOK, w.Code)
	w = f.do("GET", "/api/v1/rbac/me", nil, f.viewer)
	require.Equal(t, http.StatusOK, w.Code)
	var me EffectiveAccessResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &me))
	assert.True(t, me.Enabled)
	assert.True(t, me.HasAdminAccess)
	assert.False(t, me.IsFullAdmin)
	assert.Contains(t, me.Permissions, "llms:read")
	assert.NotContains(t, me.Permissions, "llms:write")
	require.Len(t, me.Roles, 1)
	assert.Equal(t, models.SystemRoleViewer, me.Roles[0].Slug)
	assert.Equal(t, "direct", me.Roles[0].Via)
}

func TestRBAC_EditorWritesButCannotManageAccess(t *testing.T) {
	f := setupRBACFixture(t)

	w := f.do("POST", "/api/v1/tags", map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"name": "editor-tag"}}}, f.editor)
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w = f.do("GET", "/api/v1/users", nil, f.editor)
	assert.Equal(t, http.StatusOK, w.Code, "editors may read users")
	w = f.do("POST", "/api/v1/rbac/roles", map[string]interface{}{"name": "x"}, f.editor)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = f.do("GET", "/api/v1/rbac/roles", nil, f.editor)
	assert.Equal(t, http.StatusForbidden, w.Code, "editors do not see role management")
	w = f.do("GET", "/api/v1/sso-profiles", nil, f.editor)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = f.do("POST", "/api/v1/plugins", map[string]interface{}{}, f.editor)
	assert.Equal(t, http.StatusForbidden, w.Code, "installing plugins is not an editor task")
}

// TestRBAC_PublishIsSeparateFromWrite covers the submitter / reviewer
// workflow: a role with llms:write drafts providers but cannot make one
// live, a role with llms:publish (and no write) can only flip the switch.
func TestRBAC_PublishIsSeparateFromWrite(t *testing.T) {
	f := setupRBACFixture(t)
	db := f.api.service.DB

	newUserWithRole := func(email string, perms ...string) *models.User {
		u := models.NewUser()
		u.Email = email
		u.Name = email
		u.Password = "hash"
		u.EmailVerified = true
		require.NoError(t, u.Create(db))
		w := f.do("POST", "/api/v1/rbac/roles", map[string]interface{}{"name": "role-" + email, "permissions": perms}, f.owner)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp struct{ Data RoleResponse }
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		f.bind(t, "user", u.ID, uintFromString(t, resp.Data.ID))
		return u
	}
	submitter := newUserWithRole("submitter@tyk.io", "llms:read", "llms:write")
	reviewer := newUserWithRole("reviewer@tyk.io", "llms:read", "llms:write", "llms:publish")
	approver := newUserWithRole("approver@tyk.io", "llms:publish")

	llmBody := func(attrs map[string]interface{}) map[string]interface{} {
		base := map[string]interface{}{"name": "gpt", "vendor": "openai", "api_key": "k", "api_endpoint": "https://api.openai.com", "default_model": "gpt-4o"}
		for k, v := range attrs {
			base[k] = v
		}
		return map[string]interface{}{"data": map[string]interface{}{"attributes": base}}
	}

	// Creating live needs publish; creating a draft is plain write.
	w := f.do("POST", "/api/v1/llms", llmBody(map[string]interface{}{"active": true}), submitter)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Equal(t, authz.CodePermissionDenied, decodeErrorCode(t, w.Body.Bytes()))
	assert.Contains(t, w.Body.String(), "llms:publish")

	w = f.do("POST", "/api/v1/llms", llmBody(map[string]interface{}{"active": false}), submitter)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct{ Data LLMResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	id := created.Data.ID
	assert.False(t, created.Data.Attributes.Active)

	// The submitter may keep editing, but not flip the switch.
	w = f.do("PATCH", "/api/v1/llms/"+id, map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"short_description": "draft"}}}, submitter)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = f.do("PATCH", "/api/v1/llms/"+id, map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"active": true}}}, submitter)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "llms:publish")
	w = f.do("POST", "/api/v1/llms/"+id+"/activate", nil, submitter)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	// An approver-only role can release but not edit.
	w = f.do("PATCH", "/api/v1/llms/"+id, map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"short_description": "x"}}}, approver)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "llms:write")
	w = f.do("POST", "/api/v1/llms/"+id+"/activate", nil, approver)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var activated struct{ Data LLMResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &activated))
	assert.True(t, activated.Data.Attributes.Active)

	// Editing an already-live provider without touching the switch stays write.
	w = f.do("PATCH", "/api/v1/llms/"+id, map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"short_description": "live edit"}}}, submitter)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The reviewer holds both and can deactivate through PATCH.
	w = f.do("PATCH", "/api/v1/llms/"+id, map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"active": false}}}, reviewer)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Viewer cannot publish; Editor can (system role shape).
	w = f.do("POST", "/api/v1/llms/"+id+"/activate", nil, f.viewer)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = f.do("POST", "/api/v1/llms/"+id+"/activate", nil, f.editor)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The catalogue advertises publish only where there is a live switch.
	w = f.do("GET", "/api/v1/rbac/permissions", nil, f.viewer)
	require.Equal(t, http.StatusOK, w.Code)
	var cat PermissionCatalogueResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cat))
	assert.Contains(t, cat.Actions, authz.ActionPublish)
	for _, r := range cat.Resources {
		offers := false
		for _, a := range r.Actions {
			offers = offers || a == authz.ActionPublish
		}
		switch r.Key {
		case "llms", "tools", "datasources", "apps", "agents", "model-routers", "plugins", "metadata":
			assert.True(t, offers, r.Key)
		default:
			assert.False(t, offers, r.Key)
		}
	}
}

// TestRBAC_PerPluginGrants covers the per-plugin resources: a role holding
// only plugin:<key>:read opens that plugin's configuration but not the
// plugin list, cannot call its RPC methods, and Editor (plugins:execute)
// reaches everything through the umbrella rule.
func TestRBAC_PerPluginGrants(t *testing.T) {
	f := setupRBACFixture(t)
	db := f.api.service.DB

	plugin := &models.Plugin{
		Name: "Asset catalog", Command: "/bin/true", IsActive: true,
		HookType: models.HookTypeStudioUI, HookTypes: []string{models.HookTypeStudioUI},
		Manifest: map[string]interface{}{"id": "com.example.assets"},
	}
	require.NoError(t, plugin.Create(db))
	other := &models.Plugin{
		Name: "Other", Command: "/bin/true", IsActive: true,
		HookType: models.HookTypeStudioUI, HookTypes: []string{models.HookTypeStudioUI},
		Manifest: map[string]interface{}{"id": "com.example.other"},
	}
	require.NoError(t, other.Create(db))
	f.api.service.SyncPluginPermissions(plugin)
	f.api.service.SyncPluginPermissions(other)
	const key = "plugin:com.example.assets"
	t.Cleanup(func() { authz.UnregisterPlugin(key); authz.UnregisterPlugin("plugin:com.example.other") })

	// The catalogue lists the plugin under the Plugins group.
	w := f.do("GET", "/api/v1/rbac/permissions", nil, f.viewer)
	require.Equal(t, http.StatusOK, w.Code)
	var cat PermissionCatalogueResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cat))
	found := false
	for _, r := range cat.Resources {
		if r.Key == key {
			found = true
			assert.Equal(t, "Plugins", r.Group)
			assert.Equal(t, "Asset catalog", r.Label)
			assert.True(t, r.Dynamic)
			assert.Equal(t, key, r.Plugin)
		}
	}
	assert.True(t, found, "plugin resource in catalogue")
	assert.NotEmpty(t, w.Header().Get("ETag"))

	newUserWithRole := func(email string, perms ...string) *models.User {
		u := models.NewUser()
		u.Email = email
		u.Name = email
		u.Password = "hash"
		u.EmailVerified = true
		require.NoError(t, u.Create(db))
		w := f.do("POST", "/api/v1/rbac/roles", map[string]interface{}{"name": "role-" + email, "permissions": perms}, f.owner)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp struct{ Data RoleResponse }
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		f.bind(t, "user", u.ID, uintFromString(t, resp.Data.ID))
		return u
	}
	reader := newUserWithRole("plugin-reader@tyk.io", key+":read")
	writer := newUserWithRole("plugin-writer@tyk.io", key+":write")
	pid := fmt.Sprintf("%d", plugin.ID)
	oid := fmt.Sprintf("%d", other.ID)

	// Reader: this plugin's detail yes, the list and other plugins no.
	w = f.do("GET", "/api/v1/plugins/"+pid, nil, reader)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"permission_key":"`+key+`"`)
	w = f.do("GET", "/api/v1/plugins", nil, reader)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = f.do("GET", "/api/v1/plugins/"+oid, nil, reader)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "plugins:read", "denials report the platform permission")

	// Reader cannot call RPC (write) nor patch; writer may patch config only.
	w = f.do("POST", "/api/v1/plugins/"+pid+"/rpc/admin_stats", map[string]interface{}{}, reader)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	w = f.do("PATCH", "/api/v1/plugins/"+pid, map[string]interface{}{"config": map[string]interface{}{"k": "v"}}, reader)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = f.do("PATCH", "/api/v1/plugins/"+pid, map[string]interface{}{"config": map[string]interface{}{"k": "v"}, "description": "d"}, writer)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = f.do("PATCH", "/api/v1/plugins/"+pid, map[string]interface{}{"command": "/bin/false"}, writer)
	assert.Equal(t, http.StatusForbidden, w.Code, "command changes need the platform permission")
	assert.Contains(t, w.Body.String(), "plugins:write")
	w = f.do("PATCH", "/api/v1/plugins/"+pid, map[string]interface{}{"is_active": false}, writer)
	assert.Equal(t, http.StatusForbidden, w.Code, "enable/disable needs plugins:publish")
	// The writer passes the route for RPC; the plugin is not loaded, so 404 (not 403).
	w = f.do("POST", "/api/v1/plugins/"+pid+"/rpc/admin_stats", map[string]interface{}{}, writer)
	assert.NotEqual(t, http.StatusForbidden, w.Code, w.Body.String())

	// Editor holds plugins:execute and reaches the same routes (umbrella rule).
	w = f.do("POST", "/api/v1/plugins/"+pid+"/rpc/admin_stats", map[string]interface{}{}, f.editor)
	assert.NotEqual(t, http.StatusForbidden, w.Code, w.Body.String())
	// Viewer reads plugin pages but cannot call RPC.
	w = f.do("GET", "/api/v1/plugins/"+pid, nil, f.viewer)
	assert.Equal(t, http.StatusOK, w.Code)
	w = f.do("POST", "/api/v1/plugins/"+pid+"/rpc/admin_stats", map[string]interface{}{}, f.viewer)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// /rbac/me spells out the per-plugin grant and the role shows no orphans.
	w = f.do("GET", "/api/v1/rbac/me", nil, reader)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), key+":read")

	// Uninstall: the grant becomes an orphan on the role but the role stays saveable.
	f.api.service.RemovePluginPermissions(plugin)
	w = f.do("GET", "/api/v1/rbac/roles", nil, f.owner)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"orphaned_permissions":["`+key+`:read"]`)
	w = f.do("GET", "/api/v1/plugins/"+pid, nil, reader)
	assert.Equal(t, http.StatusForbidden, w.Code, "orphaned grants are not evaluated")
}

func TestRBAC_UsersListMasksAPIKeysForNonManagers(t *testing.T) {
	f := setupRBACFixture(t)

	type users struct{ Data []UserResponse }
	w := f.do("GET", "/api/v1/users", nil, f.editor)
	require.Equal(t, http.StatusOK, w.Code)
	var forEditor users
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &forEditor))
	for _, u := range forEditor.Data {
		if uintFromString(t, u.ID) == f.editor.ID {
			assert.NotEmpty(t, u.Attributes.APIKey, "your own key is visible")
			continue
		}
		assert.Empty(t, u.Attributes.APIKey, "%s: keys are masked without users:write", u.Attributes.Email)
		assert.True(t, u.Attributes.HasAPIKey)
		assert.Len(t, u.Attributes.APIKeyHint, 4)
	}

	w = f.do("GET", "/api/v1/users", nil, f.owner)
	require.Equal(t, http.StatusOK, w.Code)
	var forOwner users
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &forOwner))
	for _, u := range forOwner.Data {
		assert.NotEmpty(t, u.Attributes.APIKey, "owners see every key")
		switch uintFromString(t, u.ID) {
		case f.viewer.ID:
			require.Len(t, u.Attributes.Roles, 1)
			assert.Equal(t, models.SystemRoleViewer, u.Attributes.Roles[0].Slug)
		case f.owner.ID:
			assert.Len(t, u.Attributes.Roles, 2, "owner holds Owner and Administrator")
		}
	}
}

func TestRBAC_MeCarriesPermissionsAndRoles(t *testing.T) {
	f := setupRBACFixture(t)
	w := f.do("GET", "/common/me?skip_entitlements=true", nil, f.editor)
	require.Equal(t, http.StatusOK, w.Code)
	var me UserWithEntitlementsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &me))
	assert.False(t, me.Attributes.IsAdmin)
	assert.True(t, me.Attributes.HasAdminAccess)
	assert.True(t, me.Attributes.RBACEnabled)
	assert.Contains(t, me.Attributes.Permissions, "llms:write")
	require.Len(t, me.Attributes.Roles, 1)
	assert.Equal(t, models.SystemRoleEditor, me.Attributes.Roles[0].Slug)
}

func TestRBAC_TeamRolesAndAdminFlag(t *testing.T) {
	f := setupRBACFixture(t)
	db := f.api.service.DB

	team := &models.Group{Name: "Platform"}
	require.NoError(t, db.Create(team).Error)
	require.NoError(t, db.Model(team).Association("Users").Append(f.viewer))

	// Binding Administrator to the team makes the viewer a full admin.
	bindingID := f.bind(t, "group", team.ID, f.roles[models.SystemRoleAdministrator])
	w := f.do("POST", "/api/v1/tags", map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"name": "via-team"}}}, f.viewer)
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var fresh models.User
	require.NoError(t, db.First(&fresh, f.viewer.ID).Error)
	assert.True(t, fresh.IsAdmin, "is_admin follows the team binding")

	// Group payloads show the roles bound to the team.
	w = f.do("GET", fmt.Sprintf("/api/v1/groups/%d", team.ID), nil, f.owner)
	require.Equal(t, http.StatusOK, w.Code)
	var g struct{ Data GroupResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &g))
	require.Len(t, g.Data.Attributes.Roles, 1)
	assert.Equal(t, models.SystemRoleAdministrator, g.Data.Attributes.Roles[0].Slug)

	// Removing the binding takes it away again.
	w = f.do("DELETE", fmt.Sprintf("/api/v1/rbac/bindings/%d", bindingID), nil, f.owner)
	assert.Equal(t, http.StatusNoContent, w.Code)
	w = f.do("POST", "/api/v1/tags", map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"name": "gone"}}}, f.viewer)
	assert.Equal(t, http.StatusForbidden, w.Code)
	require.NoError(t, db.First(&fresh, f.viewer.ID).Error)
	assert.False(t, fresh.IsAdmin)
}

func TestRBAC_RoleManagementRules(t *testing.T) {
	f := setupRBACFixture(t)

	// System roles cannot be edited or deleted.
	w := f.do("PATCH", fmt.Sprintf("/api/v1/rbac/roles/%d", f.roles[models.SystemRoleViewer]), map[string]interface{}{"name": "Peeker"}, f.owner)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, authz.CodeSystemRole, decodeErrorCode(t, w.Body.Bytes()))
	w = f.do("DELETE", fmt.Sprintf("/api/v1/rbac/roles/%d", f.roles[models.SystemRoleViewer]), nil, f.owner)
	assert.Equal(t, http.StatusConflict, w.Code)

	// Clone, then customise the copy.
	w = f.do("POST", fmt.Sprintf("/api/v1/rbac/roles/%d/clone", f.roles[models.SystemRoleViewer]), map[string]interface{}{"name": "Viewer plus tags"}, f.owner)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct{ Data RoleResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.False(t, created.Data.Attributes.IsSystem)
	cloneID := uintFromString(t, created.Data.ID)

	perms := append(created.Data.Attributes.Permissions, "tags:write")
	w = f.do("PATCH", fmt.Sprintf("/api/v1/rbac/roles/%d", cloneID), map[string]interface{}{"name": "Viewer plus tags", "permissions": perms}, f.owner)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Assign it through the user payload and watch the effect.
	w = f.do("PATCH", fmt.Sprintf("/api/v1/users/%d", f.viewer.ID), map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{
		"email": f.viewer.Email, "name": f.viewer.Name, "show_chat": true, "show_portal": true, "email_verified": true,
		"role_ids": []uint{cloneID},
	}}}, f.owner)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var updated struct{ Data UserResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	require.Len(t, updated.Data.Attributes.Roles, 1)
	assert.Equal(t, "viewer-plus-tags", updated.Data.Attributes.Roles[0].Slug)
	assert.False(t, updated.Data.Attributes.IsAdmin)
	w = f.do("POST", "/api/v1/tags", map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"name": "custom-role"}}}, f.viewer)
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	// Only an Owner may grant Owner; the last Owner cannot be removed.
	admin := models.NewUser()
	admin.Email = "admin2@tyk.io"
	admin.Name = "Admin Two"
	admin.Password = "hash"
	admin.EmailVerified = true
	require.NoError(t, admin.Create(f.api.service.DB))
	f.bind(t, "user", admin.ID, f.roles[models.SystemRoleAdministrator])
	w = f.do("POST", "/api/v1/rbac/bindings", map[string]interface{}{"subject_type": "user", "subject_id": f.editor.ID, "role_id": f.roles[models.SystemRoleOwner]}, admin)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, authz.CodeOwnerRequired, decodeErrorCode(t, w.Body.Bytes()))

	w = f.do("GET", fmt.Sprintf("/api/v1/rbac/bindings?subject_type=user&subject_id=%d&role_id=%d", f.owner.ID, f.roles[models.SystemRoleOwner]), nil, f.owner)
	require.Equal(t, http.StatusOK, w.Code)
	var bindings struct{ Data []RoleBindingResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &bindings))
	require.Len(t, bindings.Data, 1)
	w = f.do("DELETE", "/api/v1/rbac/bindings/"+bindings.Data[0].ID, nil, f.owner)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, authz.CodeLastOwner, decodeErrorCode(t, w.Body.Bytes()))

	// The legacy is_admin toggle still works and is refused for the last Owner.
	w = f.do("PATCH", fmt.Sprintf("/api/v1/users/%d", f.owner.ID), map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{
		"email": f.owner.Email, "name": f.owner.Name, "is_admin": false, "email_verified": true,
	}}}, f.owner)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	// Deleting a custom role removes its bindings.
	w = f.do("DELETE", fmt.Sprintf("/api/v1/rbac/roles/%d", cloneID), nil, f.owner)
	assert.Equal(t, http.StatusNoContent, w.Code)
	w = f.do("POST", "/api/v1/tags", map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"name": "after-delete"}}}, f.viewer)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
