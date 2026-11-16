//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/rbac"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Workspaces B4c: Apps are hidden outside the caller's Workspaces
// (workspace_full), and their API and MCP credentials follow the App
// (workspace_parent). A hidden object answers exactly as a missing one.

func appAttrs(name string, userID uint, extra map[string]any) map[string]any {
	a := map[string]any{"name": name, "description": "d", "user_id": userID}
	for k, v := range extra {
		a[k] = v
	}
	return a
}

// sameNotFound asserts that the hidden and the missing request both answer
// 404 with byte-identical bodies (no existence oracle).
func sameNotFound(t *testing.T, hidden, missing *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, http.StatusNotFound, hidden.Code, hidden.Body.String())
	assert.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())
	assert.Equal(t, missing.Body.String(), hidden.Body.String(), "hidden and missing bodies differ")
}

func appNames(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data []struct {
			Attributes struct {
				Name string `json:"name"`
			}
		}
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	var out []string
	for _, d := range resp.Data {
		out = append(out, d.Attributes.Name)
	}
	return out
}

func TestWorkspaceB4c_Apps(t *testing.T) {
	f := setupWSB4(t)
	const missing = 999999

	mine, ws := f.created(t, f.do("POST", "/api/v1/apps", body(appAttrs("Mine", f.owner.ID, nil)), f.inW1))
	assert.Equal(t, f.w1, ws, "a W1-only writer lands in W1")
	theirs, ws := f.created(t, f.do("POST", "/api/v1/apps", body(appAttrs("Theirs", f.owner.ID, map[string]any{"workspace_id": f.w2})), f.owner))
	assert.Equal(t, f.w2, ws)
	assertCode(t, f.do("POST", "/api/v1/apps", body(appAttrs("Nope", f.owner.ID, map[string]any{"workspace_id": f.w2})), f.inW1),
		http.StatusUnprocessableEntity, authz.CodeInvalidWorkspace)

	// Lists, search, count and by-name show only visible Apps; totals follow.
	w := f.do("GET", "/api/v1/apps?all=true", nil, f.inW1)
	assert.Equal(t, []string{"Mine"}, appNames(t, w))
	assert.Equal(t, "1", w.Header().Get("X-Total-Count"))
	assert.Equal(t, []string{"Mine"}, appNames(t, f.do("GET", "/api/v1/apps?search=i&all=true", nil, f.inW1)))
	assert.Empty(t, appNames(t, f.do("GET", "/api/v1/apps/search?search_term=Theirs", nil, f.inW1)))
	w = f.do("GET", "/api/v1/apps/count", nil, f.inW1)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"count":1}`, w.Body.String())
	sameNotFound(t, f.do("GET", "/api/v1/apps/by-name?name=Theirs", nil, f.inW1), f.do("GET", "/api/v1/apps/by-name?name=Nothing", nil, f.inW1))
	assert.ElementsMatch(t, []string{"Mine", "Theirs"}, appNames(t, f.do("GET", "/api/v1/apps?all=true", nil, f.owner)), "a global reader sees every App")
	assert.Equal(t, []string{"Theirs"}, appNames(t, f.do("GET", fmt.Sprintf("/api/v1/apps?all=true&workspace_id=%d", f.w2), nil, f.owner)))

	// Every action on a hidden App answers as on a missing one.
	for _, r := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/v1/apps/%d", nil},
		{"PATCH", "/api/v1/apps/%d", body(map[string]any{"description": "x"})},
		{"DELETE", "/api/v1/apps/%d", nil},
		{"POST", "/api/v1/apps/%d/activate", nil},
		{"POST", "/api/v1/apps/%d/deactivate", nil},
		{"POST", "/api/v1/apps/%d/activate-credential", nil},
		{"POST", "/api/v1/apps/%d/reset-budget", nil},
		{"GET", "/api/v1/apps/%d/dependents", nil},
		{"GET", "/api/v1/apps/%d/tools", nil},
		{"GET", "/api/v1/apps/%d/prompt-templates", nil},
		{"GET", "/api/v1/apps/%d/guardrail-exceptions", nil},
		{"GET", "/api/v1/apps/%d/plugin-resources", nil},
	} {
		sameNotFound(t, f.do(r.method, fmt.Sprintf(r.path, theirs), r.body, f.inW1), f.do(r.method, fmt.Sprintf(r.path, missing), r.body, f.inW1))
	}
	assert.Equal(t, http.StatusOK, f.do("GET", fmt.Sprintf("/api/v1/apps/%d", mine), nil, f.inW1).Code)

	// A move needs write in both Workspaces and advances updated_at.
	assertCode(t, f.do("PATCH", fmt.Sprintf("/api/v1/apps/%d", mine), body(map[string]any{"workspace_id": f.w2}), f.inW1),
		http.StatusUnprocessableEntity, authz.CodeInvalidWorkspace)
	_, before := f.stored(t, &models.App{}, mine)
	w = f.do("PATCH", fmt.Sprintf("/api/v1/apps/%d", mine), body(map[string]any{"workspace_id": f.w2}), f.inW1W2)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	ws, after := f.stored(t, &models.App{}, mine)
	assert.Equal(t, f.w2, ws)
	assert.True(t, after.After(before), "the move advanced updated_at")
	sameNotFound(t, f.do("GET", fmt.Sprintf("/api/v1/apps/%d", mine), nil, f.inW1), f.do("GET", fmt.Sprintf("/api/v1/apps/%d", missing), nil, f.inW1))
}

// API credentials follow their App; a standalone one (no App) is only for
// global holders.
func TestWorkspaceB4c_Credentials(t *testing.T) {
	f := setupWSB4(t)
	db := f.api.service.DB
	theirs, _ := f.created(t, f.do("POST", "/api/v1/apps", body(appAttrs("Theirs", f.owner.ID, map[string]any{"workspace_id": f.w2})), f.owner))
	mine, _ := f.created(t, f.do("POST", "/api/v1/apps", body(appAttrs("Mine", f.owner.ID, nil)), f.inW1))
	var theirsApp, mineApp models.App
	require.NoError(t, db.First(&theirsApp, theirs).Error)
	require.NoError(t, db.First(&mineApp, mine).Error)
	require.NotZero(t, theirsApp.CredentialID)
	orphan, err := f.api.service.CreateCredential()
	require.NoError(t, err)
	var theirsCred models.Credential
	require.NoError(t, db.First(&theirsCred, theirsApp.CredentialID).Error)

	const missing = 999999
	for _, r := range []struct {
		method, path string
	}{
		{"GET", "/api/v1/credentials/%d"},
		{"PATCH", "/api/v1/credentials/%d"},
		{"DELETE", "/api/v1/credentials/%d"},
		{"POST", "/api/v1/credentials/%d/activate"},
		{"POST", "/api/v1/credentials/%d/deactivate"},
	} {
		for _, hidden := range []uint{theirsApp.CredentialID, orphan.ID} {
			sameNotFound(t, f.do(r.method, fmt.Sprintf(r.path, hidden), map[string]any{}, f.inW1), f.do(r.method, fmt.Sprintf(r.path, missing), map[string]any{}, f.inW1))
		}
	}
	sameNotFound(t, f.do("GET", "/api/v1/credentials/key/"+theirsCred.KeyID, nil, f.inW1), f.do("GET", "/api/v1/credentials/key/no-such-key", nil, f.inW1))
	assert.Equal(t, http.StatusOK, f.do("GET", fmt.Sprintf("/api/v1/credentials/%d", mineApp.CredentialID), nil, f.inW1).Code)
	assert.Equal(t, http.StatusOK, f.do("GET", fmt.Sprintf("/api/v1/credentials/%d", orphan.ID), nil, f.owner).Code)

	ids := func(w *httptest.ResponseRecorder) []string {
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct{ Data []struct{ ID string } }
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		var out []string
		for _, d := range resp.Data {
			out = append(out, d.ID)
		}
		return out
	}
	assert.Equal(t, []string{fmt.Sprint(mineApp.CredentialID)}, ids(f.do("GET", "/api/v1/credentials?all=true", nil, f.inW1)))
	assert.Len(t, ids(f.do("GET", "/api/v1/credentials?all=true", nil, f.owner)), 3, "a global reader sees all, the standalone one included")
	assert.Equal(t, []string{fmt.Sprint(theirsApp.CredentialID)}, ids(f.do("GET", fmt.Sprintf("/api/v1/credentials?all=true&workspace_id=%d", f.w2), nil, f.owner)))
}

// MCP credentials follow their App too.
func TestWorkspaceB4c_MCPCredentials(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "workspace-b4c-test-key")
	f := setupWSB4(t)
	db := f.api.service.DB
	f.api.service.InitTykMCP(config.TykMCPConfig{Enabled: true, SyncMinInterval: 10 * time.Second, RequestTimeout: 2 * time.Second, RateLimitPerSecond: 100}, "test")
	t.Cleanup(f.api.service.TykMCP.Stop)
	theirs, _ := f.created(t, f.do("POST", "/api/v1/apps", body(appAttrs("Theirs", f.owner.ID, map[string]any{"workspace_id": f.w2})), f.owner))
	cred := &models.MCPCredential{ID: "00000000-0000-4000-8000-000000000001", AppID: theirs, ConnectionID: 1, Status: "active"}
	require.NoError(t, db.Create(cred).Error)

	const missing = "00000000-0000-4000-8000-0000000000ff"
	for _, path := range []string{"/api/v1/mcp-credentials/%s", "/api/v1/mcp-credentials/%s/rotate", "/api/v1/mcp-credentials/%s/suspend",
		"/api/v1/mcp-credentials/%s/resume", "/api/v1/mcp-credentials/%s/revoke", "/api/v1/mcp-credentials/%s/apply-drift"} {
		method := "POST"
		if path == "/api/v1/mcp-credentials/%s" {
			method = "GET"
		}
		sameNotFound(t, f.do(method, fmt.Sprintf(path, cred.ID), map[string]any{}, f.inW1), f.do(method, fmt.Sprintf(path, missing), map[string]any{}, f.inW1))
	}
	// Lists and the access report hold only visible Apps' credentials.
	total := func(as *models.User) int64 {
		w := f.do("GET", "/api/v1/mcp-credentials", nil, as)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var list struct {
			Total int64 `json:"total"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
		return list.Total
	}
	assert.EqualValues(t, 0, total(f.inW1))
	assert.EqualValues(t, 1, total(f.owner))
	assert.Equal(t, http.StatusOK, f.do("GET", "/api/v1/mcp-access-report", nil, f.inW1).Code)

	// Minting for a hidden App answers as for a missing one.
	sameNotFound(t, f.do("POST", "/api/v1/mcp-credentials", map[string]any{"app_id": theirs, "connection_id": 1}, f.inW1),
		f.do("POST", "/api/v1/mcp-credentials", map[string]any{"app_id": 999999, "connection_id": 1}, f.inW1))
}

// A portal App lands in the Default workspace; the portal shows no
// Workspace.
func TestWorkspaceB4c_PortalAppLandsInDefault(t *testing.T) {
	f := setupWSB4(t)
	w := f.do("POST", "/common/apps", map[string]any{"name": "Portal app", "description": "d", "data_source_ids": []uint{}, "llm_ids": []uint{}, "tool_ids": []uint{}}, f.inW1)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "workspace_id")
	var app models.App
	require.NoError(t, f.api.service.DB.Where("name = ?", "Portal app").First(&app).Error)
	assert.Equal(t, f.def, app.WorkspaceID)
}

// Router grants on an App (.49): a router the caller cannot see is refused
// as a missing one (never granted through the "any active router" path),
// and an update keeps the stored grants the caller cannot see.
func TestWorkspaceB4c_AppRouterGrantsRespectVisibility(t *testing.T) {
	f := setupWSB4(t)
	db := f.api.service.DB
	hidden := &models.ModelRouter{Name: "Hidden", Slug: "hidden", Active: true, WorkspaceID: f.w2}
	require.NoError(t, db.Create(hidden).Error)
	hiddenSR := &models.SemanticRouter{Name: "Hidden SR", Slug: "hidden-sr", Active: true, WorkspaceID: f.w2}
	require.NoError(t, db.Create(hiddenSR).Error)

	// Granting a hidden router answers as granting a missing one.
	sameNotFound(t,
		f.do("POST", "/api/v1/apps", body(appAttrs("Grab", f.owner.ID, map[string]any{"model_router_ids": []uint{hidden.ID}})), f.inW1),
		f.do("POST", "/api/v1/apps", body(appAttrs("Grab", f.owner.ID, map[string]any{"model_router_ids": []uint{999999}})), f.inW1))
	sameNotFound(t,
		f.do("POST", "/api/v1/apps", body(appAttrs("Grab", f.owner.ID, map[string]any{"semantic_router_ids": []uint{hiddenSR.ID}})), f.inW1),
		f.do("POST", "/api/v1/apps", body(appAttrs("Grab", f.owner.ID, map[string]any{"semantic_router_ids": []uint{999999}})), f.inW1))
	var n int64
	require.NoError(t, db.Model(&models.App{}).Where("name = ?", "Grab").Count(&n).Error)
	assert.Zero(t, n, "nothing created")

	// A W1 App that an administrator granted the hidden routers keeps them
	// when a W1 writer replaces the grants it can see.
	id, _ := f.created(t, f.do("POST", "/api/v1/apps", body(appAttrs("Shared", f.owner.ID, nil)), f.inW1))
	app := &models.App{}
	require.NoError(t, db.First(app, id).Error)
	require.NoError(t, db.Model(app).Association("ModelRouters").Append(hidden))
	require.NoError(t, db.Model(app).Association("SemanticRouters").Append(hiddenSR))
	w := f.do("PATCH", fmt.Sprintf("/api/v1/apps/%d", id), body(map[string]any{"model_router_ids": []uint{}, "semantic_router_ids": []uint{}}), f.inW1)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var mr, sr []uint
	require.NoError(t, db.Table("app_model_routers").Where("app_id = ?", id).Pluck("model_router_id", &mr).Error)
	require.NoError(t, db.Table("app_semantic_routers").Where("app_id = ?", id).Pluck("semantic_router_id", &sr).Error)
	assert.Equal(t, []uint{hidden.ID}, mr, "the hidden Model Router grant survives")
	assert.Equal(t, []uint{hiddenSR.ID}, sr, "the hidden Semantic Router grant survives")
}

// Linking LLMs, tools and datasources needs read on them somewhere.
func TestWorkspaceB4c_AppMembersNeedRead(t *testing.T) {
	f := setupWSB4(t)
	db := f.api.service.DB
	// A role bound in W1 with apps write but no llms read.
	role := &models.Role{Name: "App writer", Slug: "app-writer", Permissions: []string{"apps:read", "apps:write"}}
	require.NoError(t, db.Create(role).Error)
	u := models.NewUser()
	u.Email, u.Name, u.Password, u.EmailVerified = "aw@tyk.io", "aw", "hash", true
	require.NoError(t, u.Create(db))
	require.NoError(t, db.Create(&models.RoleBinding{SubjectType: models.RoleBindingSubjectUser, SubjectID: u.ID, RoleID: role.ID,
		ScopeType: rbac.ScopeTypeWorkspace, ScopeID: rbac.WorkspaceScopeID(f.w1)}).Error)
	w := f.do("POST", "/api/v1/apps", body(appAttrs("Linked", f.owner.ID, map[string]any{"llm_ids": []uint{1}})), u)
	assertCode(t, w, http.StatusForbidden, authz.CodePermissionDenied)
	assert.Contains(t, w.Body.String(), "llms:read")
}
