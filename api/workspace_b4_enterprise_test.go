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

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services/rbac"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Workspace-checked object routes (Workspaces B4), through the real
// middleware chain with the Enterprise evaluator: an Editor scoped to W1,
// one scoped to W1 and W2, and a global Owner.

type wsB4Fixture struct {
	*rbacFixture
	def, w1, w2 uint
	inW1        *models.User // Editor in W1
	inW1W2      *models.User // Editor in W1 and W2
}

func setupWSB4(t *testing.T) *wsB4Fixture {
	t.Helper()
	f := setupRBACFixture(t)
	db := f.api.service.DB
	def, err := models.EnsureDefaultWorkspace(db)
	require.NoError(t, err)
	w1 := &models.Workspace{Name: "W1"}
	w2 := &models.Workspace{Name: "W2"}
	require.NoError(t, db.Create(w1).Error)
	require.NoError(t, db.Create(w2).Error)
	user := func(email string, workspaces ...uint) *models.User {
		u := models.NewUser()
		u.Email, u.Name, u.Password, u.EmailVerified = email, email, "hash", true
		require.NoError(t, u.Create(db))
		for _, ws := range workspaces {
			require.NoError(t, db.Create(&models.RoleBinding{
				SubjectType: models.RoleBindingSubjectUser, SubjectID: u.ID, RoleID: f.roles[models.SystemRoleEditor],
				ScopeType: rbac.ScopeTypeWorkspace, ScopeID: rbac.WorkspaceScopeID(ws),
			}).Error)
		}
		return u
	}
	return &wsB4Fixture{rbacFixture: f, def: def.ID, w1: w1.ID, w2: w2.ID,
		inW1: user("w1@tyk.io", w1.ID), inW1W2: user("w12@tyk.io", w1.ID, w2.ID)}
}

func body(attrs map[string]any) map[string]any {
	return map[string]any{"data": map[string]any{"attributes": attrs}}
}

func (f *wsB4Fixture) created(t *testing.T, w *httptest.ResponseRecorder) (uint, uint) {
	t.Helper()
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct {
		Data struct {
			ID         string
			Attributes struct {
				WorkspaceID uint `json:"workspace_id"`
			}
		}
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return uintFromString(t, resp.Data.ID), resp.Data.Attributes.WorkspaceID
}

func (f *wsB4Fixture) stored(t *testing.T, model interface{}, id uint) (uint, time.Time) {
	t.Helper()
	var row struct {
		WorkspaceID uint
		UpdatedAt   time.Time
	}
	require.NoError(t, f.api.service.DB.Model(model).Select("workspace_id", "updated_at").Where("id = ?", id).Scan(&row).Error)
	return row.WorkspaceID, row.UpdatedAt
}

func assertCode(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	require.Equal(t, status, w.Code, w.Body.String())
	if code != "" {
		assert.Equal(t, code, decodeErrorCode(t, w.Body.Bytes()))
	}
}

func llmAttrs(name string, extra map[string]any) map[string]any {
	a := map[string]any{"name": name, "vendor": "openai", "api_endpoint": "https://api.openai.example", "privacy_score": 10}
	for k, v := range extra {
		a[k] = v
	}
	return a
}

func TestWorkspaceB4_LLMs(t *testing.T) {
	f := setupWSB4(t)

	// Landing rule.
	mine, ws := f.created(t, f.do("POST", "/api/v1/llms", body(llmAttrs("Mine", nil)), f.inW1))
	assert.Equal(t, f.w1, ws, "one Workspace: lands there")
	assertCode(t, f.do("POST", "/api/v1/llms", body(llmAttrs("Elsewhere", map[string]any{"workspace_id": f.w2})), f.inW1),
		http.StatusUnprocessableEntity, authz.CodeInvalidWorkspace)
	assertCode(t, f.do("POST", "/api/v1/llms", body(llmAttrs("Ambiguous", nil)), f.inW1W2),
		http.StatusUnprocessableEntity, authz.CodeWorkspaceRequired)
	_, ws = f.created(t, f.do("POST", "/api/v1/llms", body(llmAttrs("Global", nil)), f.owner))
	assert.Equal(t, f.def, ws, "a global writer lands in Default")
	theirs, ws := f.created(t, f.do("POST", "/api/v1/llms", body(llmAttrs("Theirs", map[string]any{"workspace_id": f.w2})), f.owner))
	assert.Equal(t, f.w2, ws)

	// Reads are global (D4); actions need the object's Workspace.
	path := func(id uint, suffix string) string { return fmt.Sprintf("/api/v1/llms/%d%s", id, suffix) }
	assert.Equal(t, http.StatusOK, f.do("GET", path(theirs, ""), nil, f.inW1).Code)
	w := f.do("GET", "/api/v1/llms?all=true", nil, f.inW1)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"Theirs"`)
	for _, req := range []struct{ method, path string }{
		{"PATCH", path(theirs, "")}, {"DELETE", path(theirs, "")},
		{"POST", path(theirs, "/activate")}, {"POST", path(theirs, "/deactivate")},
		{"POST", path(theirs, "/resource-signing-key/rotate")},
		{"PUT", path(theirs, "/plugins")},
	} {
		var b any
		if req.method == "PATCH" {
			b = body(map[string]any{"short_description": "x"})
		}
		if req.method == "PUT" {
			b = map[string]any{"plugin_ids": []uint{}}
		}
		assertCode(t, f.do(req.method, req.path, b, f.inW1), http.StatusForbidden, authz.CodePermissionDenied)
	}
	assert.Equal(t, http.StatusOK, f.do("PATCH", path(mine, ""), body(map[string]any{"short_description": "edited"}), f.inW1).Code)
	assert.Equal(t, http.StatusOK, f.do("POST", path(mine, "/activate"), nil, f.inW1).Code)

	// ?workspace_id= filters the list.
	w = f.do("GET", fmt.Sprintf("/api/v1/llms?all=true&workspace_id=%d", f.w2), nil, f.owner)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"Theirs"`)
	assert.NotContains(t, w.Body.String(), `"Mine"`)

	// Moves need write in both Workspaces, and bump updated_at.
	assertCode(t, f.do("PATCH", path(mine, ""), body(map[string]any{"workspace_id": f.w2}), f.inW1),
		http.StatusUnprocessableEntity, authz.CodeInvalidWorkspace)
	ws, before := f.stored(t, &models.LLM{}, mine)
	assert.Equal(t, f.w1, ws, "a refused move changes nothing")
	time.Sleep(10 * time.Millisecond)
	w = f.do("PATCH", path(mine, ""), body(map[string]any{"workspace_id": f.w2}), f.inW1W2)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	ws, after := f.stored(t, &models.LLM{}, mine)
	assert.Equal(t, f.w2, ws, "moved")
	assert.True(t, after.After(before), "updated_at advanced")

	// Delete in the own Workspace.
	assert.Equal(t, http.StatusNoContent, f.do("DELETE", path(mine, ""), nil, f.inW1W2).Code)
}

// Secret references on a Workspace object (.38): the secret must be in the
// object's Workspace and covered by the caller's secrets:read there; $ENV/
// needs it globally; an unknown name is the same 403; a move re-checks.
func TestWorkspaceB4_SecretReferences(t *testing.T) {
	f := setupWSB4(t)
	db := f.api.service.DB
	secret := func(name string, ws uint) {
		s := &secrets.Secret{VarName: name, Value: "v", WorkspaceID: ws}
		require.NoError(t, secrets.CreateSecret(db, s))
	}
	secret("W1KEY", f.w1)
	secret("W2KEY", f.w2)

	assertCode(t, f.do("POST", "/api/v1/llms", body(llmAttrs("Cross", map[string]any{"api_key": "$SECRET/W2KEY"})), f.inW1),
		http.StatusForbidden, authz.CodePermissionDenied)
	assertCode(t, f.do("POST", "/api/v1/llms", body(llmAttrs("Unknown", map[string]any{"api_key": "$SECRET/NOPE"})), f.inW1),
		http.StatusForbidden, authz.CodePermissionDenied)
	assertCode(t, f.do("POST", "/api/v1/llms", body(llmAttrs("Env", map[string]any{"api_key": "$ENV/HOME"})), f.inW1),
		http.StatusForbidden, authz.CodePermissionDenied)
	id, _ := f.created(t, f.do("POST", "/api/v1/llms", body(llmAttrs("Same", map[string]any{"api_key": "$SECRET/W1KEY"})), f.inW1))

	// Even a global writer cannot point a W2 object at a W1 secret.
	assertCode(t, f.do("POST", "/api/v1/llms", body(llmAttrs("Owner cross", map[string]any{"api_key": "$SECRET/W1KEY", "workspace_id": f.w2})), f.owner),
		http.StatusForbidden, authz.CodePermissionDenied)

	// Moving the provider to W2 would leave it pointing at a W1 secret.
	assertCode(t, f.do("PATCH", fmt.Sprintf("/api/v1/llms/%d", id), body(map[string]any{"workspace_id": f.w2}), f.inW1W2),
		http.StatusUnprocessableEntity, authz.CodeInvalidWorkspace)
	ws, _ := f.stored(t, &models.LLM{}, id)
	assert.Equal(t, f.w1, ws)
}

func toolAttrs(name string, extra map[string]any) map[string]any {
	a := map[string]any{"name": name, "tool_type": models.ToolTypeREST, "privacy_score": 10,
		"oas_spec": "eyJvcGVuYXBpIjoiMy4wLjAiLCJpbmZvIjp7InRpdGxlIjoidCIsInZlcnNpb24iOiIxIn0sInNlcnZlcnMiOlt7InVybCI6Imh0dHBzOi8vYXBpLmV4YW1wbGUifV0sInBhdGhzIjp7fX0="}
	for k, v := range extra {
		a[k] = v
	}
	return a
}

func TestWorkspaceB4_Tools(t *testing.T) {
	f := setupWSB4(t)

	mine, ws := f.created(t, f.do("POST", "/api/v1/tools", body(toolAttrs("Mine", nil)), f.inW1))
	assert.Equal(t, f.w1, ws)
	assertCode(t, f.do("POST", "/api/v1/tools", body(toolAttrs("Elsewhere", map[string]any{"workspace_id": f.w2})), f.inW1),
		http.StatusUnprocessableEntity, authz.CodeInvalidWorkspace)
	assertCode(t, f.do("POST", "/api/v1/tools", body(toolAttrs("Ambiguous", nil)), f.inW1W2),
		http.StatusUnprocessableEntity, authz.CodeWorkspaceRequired)
	_, ws = f.created(t, f.do("POST", "/api/v1/tools", body(toolAttrs("Global", nil)), f.owner))
	assert.Equal(t, f.def, ws)
	theirs, _ := f.created(t, f.do("POST", "/api/v1/tools", body(toolAttrs("Theirs", map[string]any{"workspace_id": f.w2})), f.owner))

	path := func(id uint, suffix string) string { return fmt.Sprintf("/api/v1/tools/%d%s", id, suffix) }
	assert.Equal(t, http.StatusOK, f.do("GET", path(theirs, ""), nil, f.inW1).Code)
	for _, req := range []struct {
		method, path string
		body         any
	}{
		{"PATCH", path(theirs, ""), body(map[string]any{"description": "x"})},
		{"DELETE", path(theirs, ""), nil},
		{"POST", path(theirs, "/deactivate"), nil},
		{"POST", path(theirs, "/operations"), map[string]any{"operation": "x"}},
		{"PUT", path(theirs, "/filters"), map[string]any{"data": []any{}}},
		{"POST", path(theirs, "/call-operation"), map[string]any{}},
	} {
		assertCode(t, f.do(req.method, req.path, req.body, f.inW1), http.StatusForbidden, authz.CodePermissionDenied)
	}
	assert.Equal(t, http.StatusOK, f.do("PATCH", path(mine, ""), body(map[string]any{"description": "edited"}), f.inW1).Code)
	assert.Equal(t, http.StatusOK, f.do("POST", path(mine, "/deactivate"), nil, f.inW1).Code)

	for _, q := range []string{"/api/v1/tools?all=true", "/api/v1/tools/search?query=Theirs", "/api/v1/tools/by-type?type=" + models.ToolTypeREST} {
		w := f.do("GET", fmt.Sprintf("%s&workspace_id=%d", q, f.w1), nil, f.owner)
		require.Equal(t, http.StatusOK, w.Code, q)
		assert.NotContains(t, w.Body.String(), `"Theirs"`, q)
	}

	assertCode(t, f.do("PATCH", path(mine, ""), body(map[string]any{"workspace_id": f.w2}), f.inW1),
		http.StatusUnprocessableEntity, authz.CodeInvalidWorkspace)
	_, before := f.stored(t, &models.Tool{}, mine)
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, http.StatusOK, f.do("PATCH", path(mine, ""), body(map[string]any{"workspace_id": f.w2}), f.inW1W2).Code)
	ws, after := f.stored(t, &models.Tool{}, mine)
	assert.Equal(t, f.w2, ws)
	assert.True(t, after.After(before))
}
