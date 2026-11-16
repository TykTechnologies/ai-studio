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

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/rbac"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The push precondition (Workspaces C2): {"only_workspace_id": W} refuses a
// push while a target namespace has a pending change from outside W.

type pushPreconditionFixture struct {
	*pushTestAPI
	def, w1, w2 uint
}

func setupPushPrecondition(t *testing.T) *pushPreconditionFixture {
	t.Helper()
	p := setupPushTestAPI(t)
	db := p.api.service.DB
	def, err := models.EnsureDefaultWorkspace(db)
	require.NoError(t, err)
	w1 := &models.Workspace{Name: "W1"}
	w2 := &models.Workspace{Name: "W2"}
	require.NoError(t, db.Create(w1).Error)
	require.NoError(t, db.Create(w2).Error)
	p.edge(t, "edge-a", "ns1", models.EdgeStatusConnected)
	p.edge(t, "edge-b", "ns2", models.EdgeStatusConnected)
	// Both namespaces were pushed an hour ago; only later changes count.
	for _, ns := range []string{"ns1", "ns2"} {
		require.NoError(t, models.MarkNamespacePushed(db, ns, time.Now().Add(-time.Hour)))
	}
	return &pushPreconditionFixture{pushTestAPI: p, def: def.ID, w1: w1.ID, w2: w2.ID}
}

// llm creates an LLM in the namespace and Workspace, changed after the push.
func (f *pushPreconditionFixture) llm(t *testing.T, name, ns string, ws uint) {
	t.Helper()
	db := f.api.service.DB
	l := &models.LLM{Name: name, Vendor: models.OPENAI, Namespace: ns}
	require.NoError(t, db.Create(l).Error)
	require.NoError(t, db.Model(l).UpdateColumn("workspace_id", ws).Error)
}

func (f *pushPreconditionFixture) post(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return apitest.PerformRequest(f.router, "POST", path, body)
}

func (f *pushPreconditionFixture) state(t *testing.T, ns string) (ops int64, lastPush time.Time) {
	t.Helper()
	db := f.api.service.DB
	require.NoError(t, db.Model(&models.PushOperation{}).Count(&ops).Error)
	var st models.NamespaceSyncStatus
	require.NoError(t, st.GetByNamespace(db, ns))
	return ops, *st.LastPushAt
}

func TestPushPrecondition_StainedPushIsRefused(t *testing.T) {
	f := setupPushPrecondition(t)
	f.llm(t, "Mine", "ns1", f.w1)
	f.llm(t, "Theirs", "ns1", f.w2)
	opsBefore, pushBefore := f.state(t, "ns1")

	only := map[string]any{"only_workspace_id": f.w1}
	w := f.post(t, "/api/v1/namespaces/ns1/reload", only)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var body PushStainedBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Errors, 1)
	assert.Equal(t, CodePushStained, body.Errors[0].Code)
	assert.Equal(t, f.w1, body.WorkspaceID)
	assert.Equal(t, 1, body.OutsideTotal)
	require.Len(t, body.ByWorkspace, 1)
	assert.Equal(t, f.w2, body.ByWorkspace[0].WorkspaceID)
	assert.Equal(t, 1, body.ByWorkspace[0].Total)
	require.Len(t, body.Changes, 1)
	assert.Equal(t, "Theirs", body.Changes[0].Name)
	assert.Equal(t, "ns1", body.Changes[0].Namespace)

	ops, push := f.state(t, "ns1")
	assert.Equal(t, opsBefore, ops, "no push operation recorded")
	assert.True(t, push.Equal(pushBefore), "last_push_at unchanged")

	// The edge reload checks that edge's namespace.
	assert.Equal(t, http.StatusConflict, f.post(t, "/api/v1/edges/edge-a/reload", only).Code)
	// Without the body: pushed as today.
	assert.Equal(t, http.StatusAccepted, f.post(t, "/api/v1/namespaces/ns1/reload", nil).Code)
}

func TestPushPrecondition_OwnChangesPush(t *testing.T) {
	f := setupPushPrecondition(t)
	f.llm(t, "Mine", "ns2", f.w1)
	only := map[string]any{"only_workspace_id": f.w1}
	assert.Equal(t, http.StatusAccepted, f.post(t, "/api/v1/namespaces/ns2/reload", only).Code)
	// Nothing pending at all (the push above stamped ns2) pushes too.
	assert.Equal(t, http.StatusAccepted, f.post(t, "/api/v1/edges/edge-b/reload", only).Code)
}

func TestPushPrecondition_ReloadAllWithOneStainedNamespace(t *testing.T) {
	f := setupPushPrecondition(t)
	f.llm(t, "Mine", "ns1", f.w1)
	f.llm(t, "Theirs", "ns2", f.w2)
	opsBefore, _ := f.state(t, "ns1")
	w := f.post(t, "/api/v1/edges/reload-all", map[string]any{"only_workspace_id": f.w1})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	ops, _ := f.state(t, "ns1")
	assert.Equal(t, opsBefore, ops, "the whole push is refused")
}

func TestPushPrecondition_BadInput(t *testing.T) {
	f := setupPushPrecondition(t)
	w := f.post(t, "/api/v1/namespaces/ns1/reload", map[string]any{"only_workspace_id": 999999})
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "invalid_workspace")
	w = f.post(t, "/api/v1/namespaces/ns1/reload", "not an object")
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// The push_stained body is redacted as the pending-changes preview is
// (.44): a Workspace-scoped caller (the Operator: global edges, an Editor
// in W1) sees neither a hidden App's name nor the name of a Workspace it
// cannot name; a caller covering every Workspace sees both.
func TestPushPrecondition_StainedBodyIsRedacted(t *testing.T) {
	f := setupWSB4(t)
	db := f.api.service.DB
	now := time.Now()
	require.NoError(t, db.Create(&models.EdgeInstance{EdgeID: "edge-a", Namespace: "ns1", Status: models.EdgeStatusConnected, LastHeartbeat: &now}).Error)
	require.NoError(t, models.MarkNamespacePushed(db, "ns1", now.Add(-time.Hour)))
	app := &models.App{Name: "Hidden app", UserID: f.owner.ID, Namespace: "ns1", WorkspaceID: f.w2}
	require.NoError(t, db.Create(app).Error)

	edges := &models.Role{Name: "Edges", Slug: "edges-operator", Permissions: []string{"edges:read", "edges:execute"}}
	require.NoError(t, db.Create(edges).Error)
	op := models.NewUser()
	op.Email, op.Name, op.Password, op.EmailVerified = "operator@tyk.io", "operator", "hash", true
	require.NoError(t, op.Create(db))
	require.NoError(t, db.Create(&models.RoleBinding{SubjectType: models.RoleBindingSubjectUser, SubjectID: op.ID, RoleID: edges.ID, ScopeType: rbac.ScopeTypeGlobal}).Error)
	require.NoError(t, db.Create(&models.RoleBinding{SubjectType: models.RoleBindingSubjectUser, SubjectID: op.ID, RoleID: f.roles[models.SystemRoleEditor],
		ScopeType: rbac.ScopeTypeWorkspace, ScopeID: rbac.WorkspaceScopeID(f.w1)}).Error)

	stained := func(as *models.User) PushStainedBody {
		w := f.do("POST", "/api/v1/namespaces/ns1/reload", map[string]any{"only_workspace_id": f.w1}, as)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		var body PushStainedBody
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.NotEmpty(t, body.Changes)
		require.NotEmpty(t, body.ByWorkspace)
		return body
	}
	body := stained(op)
	assert.True(t, body.Changes[0].Hidden)
	assert.Empty(t, body.Changes[0].Name)
	assert.Empty(t, body.Changes[0].WorkspaceName)
	assert.Empty(t, body.ByWorkspace[0].Name)
	assert.NotContains(t, fmt.Sprint(body), "Hidden app")

	body = stained(f.owner)
	assert.Equal(t, "Hidden app", body.Changes[0].Name)
	assert.Equal(t, "W2", body.ByWorkspace[0].Name)

	// One name rule with GET /workspaces (coversEveryWorkspace): a global
	// read on any Workspace object type names every Workspace, without
	// revealing the hidden App.
	reader := &models.Role{Name: "Edges and LLMs", Slug: "edges-llms", Permissions: []string{"edges:read", "edges:execute", "llms:read"}}
	require.NoError(t, db.Create(reader).Error)
	u := models.NewUser()
	u.Email, u.Name, u.Password, u.EmailVerified = "reader@tyk.io", "reader", "hash", true
	require.NoError(t, u.Create(db))
	require.NoError(t, db.Create(&models.RoleBinding{SubjectType: models.RoleBindingSubjectUser, SubjectID: u.ID, RoleID: reader.ID, ScopeType: rbac.ScopeTypeGlobal}).Error)
	body = stained(u)
	assert.Equal(t, "W2", body.ByWorkspace[0].Name)
	assert.True(t, body.Changes[0].Hidden)
	w := f.do("GET", "/api/v1/workspaces", nil, u)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"W2"`, "GET /workspaces names it too")
}
