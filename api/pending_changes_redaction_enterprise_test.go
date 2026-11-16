//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/rbac"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A caller with global edges:read and apps:read scoped to W1 (the Operator
// recipe) sees a pending W2 App change without its id or name, and no name
// for Workspaces it has no binding in; counts are unchanged. A global
// administrator sees everything (midsommar-clean-oztk.44).
func TestPendingChanges_RedactedForAWorkspaceScopedCaller(t *testing.T) {
	f := setupRBACFixture(t)
	db := f.api.service.DB
	_, err := models.EnsureDefaultWorkspace(db)
	require.NoError(t, err)
	w1 := &models.Workspace{Name: "W1"}
	w2 := &models.Workspace{Name: "W2"}
	require.NoError(t, db.Create(w1).Error)
	require.NoError(t, db.Create(w2).Error)

	role := func(name string, perms ...string) uint {
		w := f.do("POST", "/api/v1/rbac/roles", RoleInput{Name: name, Permissions: perms}, f.owner)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var r struct{ Data RoleResponse }
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
		return uintFromString(t, r.Data.ID)
	}
	op := models.NewUser()
	op.Email, op.Name, op.Password, op.EmailVerified = "operator@tyk.io", "Operator", "hash", true
	require.NoError(t, op.Create(db))
	f.bind(t, "user", op.ID, role("Edge reader", "edges:read"))
	// Written directly: the scoped bindings API is B5's.
	require.NoError(t, db.Create(&models.RoleBinding{
		SubjectType: models.RoleBindingSubjectUser, SubjectID: op.ID, RoleID: role("App reader", "apps:read"),
		ScopeType: rbac.ScopeTypeWorkspace, ScopeID: rbac.WorkspaceScopeID(w1.ID),
	}).Error)

	stamp := func(row interface{}, ws uint) {
		require.NoError(t, db.Create(row).Error)
		require.NoError(t, db.Model(row).UpdateColumn("workspace_id", ws).Error)
	}
	stamp(&models.App{Name: "Hidden W2 app", Namespace: "default"}, w2.ID)
	stamp(&models.App{Name: "Own W1 app", Namespace: "default"}, w1.ID)
	stamp(&models.LLM{Name: "W2 LLM", Namespace: "default"}, w2.ID)

	get := func(as *models.User, query string) services.PendingChanges {
		w := f.do("GET", "/api/v1/sync/pending-changes?namespace=default"+query, nil, as)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct{ Data services.PendingChanges }
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.Data
	}
	find := func(pc services.PendingChanges, typ string, ws uint, hidden bool) *services.PendingChange {
		for i := range pc.Changes {
			c := &pc.Changes[i]
			if c.Type == typ && c.WorkspaceID != nil && *c.WorkspaceID == ws && c.Hidden == hidden {
				return c
			}
		}
		return nil
	}

	full := get(f.owner, "&workspace_id="+uintString(w1.ID))
	scoped := get(op, "&workspace_id="+uintString(w1.ID))

	hidden := find(scoped, "app", w2.ID, true)
	require.NotNil(t, hidden, "the W2 App change is listed, redacted")
	assert.Zero(t, hidden.ID)
	assert.Empty(t, hidden.Name)
	assert.Empty(t, hidden.WorkspaceName, "no binding in W2: no name")
	own := find(scoped, "app", w1.ID, false)
	require.NotNil(t, own)
	assert.Equal(t, "Own W1 app", own.Name)
	assert.Equal(t, "W1", own.WorkspaceName)
	llm := find(scoped, "llm", w2.ID, false)
	require.NotNil(t, llm, "LLMs are read-global (D4)")
	assert.Equal(t, "W2 LLM", llm.Name)
	assert.Empty(t, llm.WorkspaceName)
	for _, b := range scoped.ByWorkspace {
		if b.WorkspaceID == w1.ID {
			assert.Equal(t, "W1", b.Name)
		} else {
			assert.Empty(t, b.Name, "workspace %d", b.WorkspaceID)
		}
	}

	// Counts are unchanged.
	assert.Equal(t, full.Total, scoped.Total)
	assert.Equal(t, len(full.Changes), len(scoped.Changes))
	assert.Equal(t, *full.OutsideTotal, *scoped.OutsideTotal)
	assert.Equal(t, *full.Stained, *scoped.Stained)
	assert.Equal(t, full.UnattributedTotal, scoped.UnattributedTotal)
	require.Len(t, scoped.ByWorkspace, len(full.ByWorkspace))
	for i := range full.ByWorkspace {
		assert.Equal(t, full.ByWorkspace[i].Total, scoped.ByWorkspace[i].Total)
	}

	// The administrator sees everything.
	all := find(full, "app", w2.ID, false)
	require.NotNil(t, all)
	assert.Equal(t, "Hidden W2 app", all.Name)
	assert.Equal(t, "W2", all.WorkspaceName)
}
