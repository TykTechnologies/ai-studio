package services_test

import (
	"fmt"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Workspace-aware pending changes (midsommar-clean-oztk.14): each change
// carries its owning Workspace (a stored 0 read as Default), totals by
// Workspace cover every change, and the stain summary for one Workspace
// counts what a push limited to it would carry from outside it.

type wsFixture struct {
	db          *gorm.DB
	svc         *services.SyncStatusService
	def, w1, w2 uint
	push        time.Time
}

func newWSFixture(t *testing.T) *wsFixture {
	t.Helper()
	db := apitest.SetupTestDB(t)
	def, err := models.EnsureDefaultWorkspace(db)
	require.NoError(t, err)
	w1 := &models.Workspace{Name: "W1"}
	w2 := &models.Workspace{Name: "W2"}
	require.NoError(t, db.Create(w1).Error)
	require.NoError(t, db.Create(w2).Error)
	push := time.Now().Add(-time.Hour)
	require.NoError(t, models.MarkNamespacePushed(db, "edge-1", push))
	return &wsFixture{db: db, svc: services.NewSyncStatusService(db), def: def.ID, w1: w1.ID, w2: w2.ID, push: push}
}

// changed creates row, stamps it with workspace (0 = as an older replica
// wrote it) and makes it a change after the push.
func (f *wsFixture) changed(t *testing.T, row interface{}, workspace uint) {
	t.Helper()
	require.NoError(t, f.db.Create(row).Error)
	if _, stamped := row.(interface{ BeforeUpdate(*gorm.DB) error }); stamped {
		require.NoError(t, f.db.Model(row).UpdateColumn("workspace_id", workspace).Error)
	}
	at := f.push.Add(10 * time.Minute)
	require.NoError(t, f.db.Model(row).UpdateColumns(map[string]interface{}{"created_at": at, "updated_at": at}).Error)
}

func byKey(pc *services.PendingChanges) map[string]services.PendingChange {
	out := map[string]services.PendingChange{}
	for _, c := range pc.Changes {
		out[c.Type+":"+c.Name] = c
	}
	return out
}

func TestPendingChanges_WorkspaceAttributionAndStain(t *testing.T) {
	f := newWSFixture(t)

	w1LLM := &models.LLM{Name: "W1 LLM", Namespace: "edge-1"}
	f.changed(t, w1LLM, f.w1)
	w2LLM := &models.LLM{Name: "W2 LLM", Namespace: "edge-1"}
	f.changed(t, w2LLM, f.w2)
	oldReplicaApp := &models.App{Name: "Old replica app", Namespace: "edge-1"}
	f.changed(t, oldReplicaApp, 0)
	plugin := &models.Plugin{Name: "Plug", Command: "file:///bin/true", HookType: "post_auth", Namespace: "edge-1"}
	f.changed(t, plugin, f.w1)
	price := &models.ModelPrice{ModelName: "gpt-x", Vendor: "openai"}
	f.changed(t, price, 0)
	client := &models.OAuthClient{ClientID: "c1", ClientSecret: "s", ClientName: "Client", RedirectURIs: "https://x"}
	f.changed(t, client, 0)
	token := &models.AccessToken{Token: "t1", ClientID: "c1", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)}
	f.changed(t, token, 0)
	// A W2 tool soft-deleted after the push: a pending deletion, foreign.
	gone := &models.Tool{Name: "W2 gone", Namespace: "edge-1"}
	require.NoError(t, f.db.Create(gone).Error)
	require.NoError(t, f.db.Model(gone).UpdateColumn("workspace_id", f.w2).Error)
	old := f.push.Add(-time.Hour)
	require.NoError(t, f.db.Model(gone).UpdateColumns(map[string]interface{}{"created_at": old, "updated_at": old}).Error)
	require.NoError(t, f.db.Delete(gone).Error)

	pc, err := f.svc.GetPendingChanges("edge-1")
	require.NoError(t, err)
	keys := byKey(pc)
	require.Len(t, keys, 8, "%v", keys)

	ws := func(key string) *uint { return keys[key].WorkspaceID }
	require.NotNil(t, ws("llm:W1 LLM"))
	assert.Equal(t, f.w1, *ws("llm:W1 LLM"))
	assert.Equal(t, "W1", keys["llm:W1 LLM"].WorkspaceName)
	assert.Equal(t, f.w2, *ws("llm:W2 LLM"))
	require.NotNil(t, ws("app:Old replica app"))
	assert.Equal(t, f.def, *ws("app:Old replica app"), "a stored 0 reads as Default")
	assert.Equal(t, models.DefaultWorkspaceName, keys["app:Old replica app"].WorkspaceName)
	assert.Equal(t, f.w2, *ws("tool:W2 gone"))
	assert.Equal(t, services.PendingChangeDeleted, keys["tool:W2 gone"].Change)
	for _, k := range []string{"plugin:Plug", "model_price:openai/gpt-x", "oauth_client:Client", "access_token:c1"} {
		assert.Nil(t, ws(k), "%s has no owning Workspace", k)
	}

	assert.Equal(t, []services.PendingWorkspaceTotal{
		{WorkspaceID: f.def, Name: models.DefaultWorkspaceName, Total: 1},
		{WorkspaceID: f.w1, Name: "W1", Total: 1},
		{WorkspaceID: f.w2, Name: "W2", Total: 2},
	}, pc.ByWorkspace, "no 0 bucket")
	assert.Equal(t, 4, pc.UnattributedTotal)
	assert.Nil(t, pc.Stained, "no stain summary unless asked")
	assert.Nil(t, pc.OutsideTotal)

	stain := func(workspaceID uint) (int, bool) {
		t.Helper()
		pc, err := f.svc.GetPendingChangesFor("edge-1", &workspaceID)
		require.NoError(t, err)
		require.NotNil(t, pc.OutsideTotal)
		require.NotNil(t, pc.Stained)
		assert.Equal(t, workspaceID, *pc.WorkspaceID)
		return *pc.OutsideTotal, *pc.Stained
	}
	// W1: the W2 LLM and tool, the Default app, the plugin and the price.
	n, stained := stain(f.w1)
	assert.Equal(t, 5, n)
	assert.True(t, stained)
	// Default: the 0 row is its own; W1, W2 x2, plugin, price are foreign.
	n, stained = stain(f.def)
	assert.Equal(t, 5, n)
	assert.True(t, stained)
}

func TestPendingChanges_OwnWorkspaceAndOAuthOnlyIsNotStained(t *testing.T) {
	f := newWSFixture(t)
	f.changed(t, &models.LLM{Name: "Mine", Namespace: "edge-1"}, f.w1)
	f.changed(t, &models.OAuthClient{ClientID: "c1", ClientSecret: "s", ClientName: "Client", RedirectURIs: "https://x"}, 0)
	f.changed(t, &models.AccessToken{Token: "t1", ClientID: "c1", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)}, 0)

	for _, id := range []uint{f.w1} {
		pc, err := f.svc.GetPendingChangesFor("edge-1", &id)
		require.NoError(t, err)
		assert.Equal(t, 0, *pc.OutsideTotal)
		assert.False(t, *pc.Stained, "own changes and OAuth artifacts never stain")
	}

	// A 0 row (an older replica's) is not foreign to Default and is to W1.
	f2 := newWSFixture(t)
	f2.changed(t, &models.Filter{Name: "Old replica filter", Namespace: "edge-1"}, 0)
	pc, err := f2.svc.GetPendingChangesFor("edge-1", &f2.def)
	require.NoError(t, err)
	assert.False(t, *pc.Stained)
	pc, err = f2.svc.GetPendingChangesFor("edge-1", &f2.w1)
	require.NoError(t, err)
	assert.True(t, *pc.Stained)
	assert.Equal(t, 1, *pc.OutsideTotal)
}

// Totals and the stain summary cover every change, not only the 200 listed.
func TestPendingChanges_WorkspaceTotalsBeyondTheCap(t *testing.T) {
	f := newWSFixture(t)
	const foreign = 205
	for i := 0; i < foreign; i++ {
		f.changed(t, &models.LLM{Name: fmt.Sprintf("w2 llm %d", i), Namespace: "edge-1"}, f.w2)
	}
	f.changed(t, &models.LLM{Name: "w1 llm", Namespace: "edge-1"}, f.w1)

	pc, err := f.svc.GetPendingChangesFor("edge-1", &f.w1)
	require.NoError(t, err)
	require.Len(t, pc.Changes, 200)
	assert.Equal(t, foreign+1, pc.Total)
	assert.Equal(t, []services.PendingWorkspaceTotal{
		{WorkspaceID: f.w1, Name: "W1", Total: 1},
		{WorkspaceID: f.w2, Name: "W2", Total: foreign},
	}, pc.ByWorkspace)
	assert.Equal(t, foreign, *pc.OutsideTotal)
}

// A row soft-deleted before the push and touched after it is no pending
// change: the totals leave it out, as the listing does.
func TestPendingChanges_TotalsAgreeWithTheListing(t *testing.T) {
	f := newWSFixture(t)
	tool := &models.Tool{Name: "Long gone", Namespace: "edge-1"}
	require.NoError(t, f.db.Create(tool).Error)
	require.NoError(t, f.db.Model(tool).UpdateColumn("workspace_id", f.w2).Error)
	require.NoError(t, f.db.Model(tool).UpdateColumns(map[string]interface{}{
		"deleted_at": f.push.Add(-time.Minute), "updated_at": f.push.Add(time.Minute),
	}).Error)

	pc, err := f.svc.GetPendingChangesFor("edge-1", &f.w1)
	require.NoError(t, err)
	assert.Empty(t, pc.Changes)
	assert.Empty(t, pc.ByWorkspace)
	assert.Equal(t, 0, *pc.OutsideTotal)
	assert.False(t, *pc.Stained)
}

func TestPendingChangeIsForeign(t *testing.T) {
	w1, w2 := uint(1), uint(2)
	assert.False(t, services.PendingChangeIsForeign("llm", &w1, w1))
	assert.True(t, services.PendingChangeIsForeign("llm", &w2, w1))
	assert.True(t, services.PendingChangeIsForeign("plugin", nil, w1))
	assert.True(t, services.PendingChangeIsForeign("model_price", nil, w1))
	assert.False(t, services.PendingChangeIsForeign("oauth_client", nil, w1))
	assert.False(t, services.PendingChangeIsForeign("access_token", nil, w1))
	assert.True(t, services.PendingChangeIsForeign("a_source_added_later", nil, w1), "fail closed")
}

// A first push (no push, no edge ack): every live row is a change. The
// totals count OAuth clients and access tokens with a plain COUNT(*) rather
// than through the union, and still agree with the listing.
func TestPendingChanges_FirstPushTotalsAgreeWithTheListing(t *testing.T) {
	f := newWSFixture(t)
	const ns = "edge-new"
	for _, row := range []struct {
		llm *models.LLM
		ws  uint
	}{
		{&models.LLM{Name: "w1 llm", Namespace: ns}, f.w1},
		{&models.LLM{Name: "w2 llm", Namespace: ns}, f.w2},
		{&models.LLM{Name: "legacy llm", Namespace: ns}, 0},
	} {
		require.NoError(t, f.db.Create(row.llm).Error)
		require.NoError(t, f.db.Model(row.llm).UpdateColumn("workspace_id", row.ws).Error)
	}
	require.NoError(t, f.db.Create(&models.Plugin{Name: "Guard", Command: "file:///bin/true", HookType: "post_auth", Namespace: ns}).Error)
	require.NoError(t, f.db.Create(&models.OAuthClient{ClientID: "c1", ClientSecret: "s", ClientName: "Client", RedirectURIs: "https://x"}).Error)
	require.NoError(t, f.db.Create(&models.AccessToken{Token: "t1", ClientID: "c1", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)}).Error)
	gone := &models.AccessToken{Token: "t2", ClientID: "c1", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, f.db.Create(gone).Error)
	require.NoError(t, f.db.Delete(gone).Error)

	pc, err := f.svc.GetPendingChangesFor(ns, &f.w1)
	require.NoError(t, err)
	require.Nil(t, pc.Since, "first push: no reference point")

	listed := map[string]int{}
	attributed := map[uint]int{}
	unattributed := 0
	for _, c := range pc.Changes {
		listed[c.Type]++
		if c.WorkspaceID != nil {
			attributed[*c.WorkspaceID]++
		} else {
			unattributed++
		}
	}
	assert.Equal(t, map[string]int{"llm": 3, "plugin": 1, "oauth_client": 1, "access_token": 1}, listed)
	totals := map[uint]int{}
	for _, w := range pc.ByWorkspace {
		totals[w.WorkspaceID] = w.Total
	}
	assert.Equal(t, attributed, totals, "by_workspace matches the listing")
	assert.Equal(t, map[uint]int{f.def: 1, f.w1: 1, f.w2: 1}, totals)
	assert.Equal(t, unattributed, pc.UnattributedTotal, "plugin + OAuth client + live token")
	assert.Equal(t, 3, pc.UnattributedTotal)
	assert.Equal(t, 3, *pc.OutsideTotal, "Default llm, W2 llm and the plugin; OAuth never stains")
	assert.True(t, *pc.Stained)
}
