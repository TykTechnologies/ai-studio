package openfga

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	store, err := New(ctx)
	require.NoError(t, err)
	t.Cleanup(store.Close)
	return store
}

func TestNew(t *testing.T) {
	store := newTestStore(t)
	assert.NotEmpty(t, store.storeID)
	assert.NotEmpty(t, store.modelID)
}

func TestCheck_SystemAdmin(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:1", Relation: "admin", Resource: "system:1"},
	}))

	allowed, err := store.CheckByName(ctx, 1, "admin", "system", "1")
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = store.CheckByName(ctx, 2, "admin", "system", "1")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestCheck_SSOAdmin_InheritedFromAdmin(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:1", Relation: "admin", Resource: "system:1"},
	}))

	allowed, err := store.CheckByName(ctx, 1, "sso_admin", "system", "1")
	require.NoError(t, err)
	assert.True(t, allowed, "admin should inherit sso_admin")
}

func TestCheck_GroupMembership(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:10", Relation: "member", Resource: "group:1"},
	}))

	// No direct check API for group membership in our Authorizer since groups aren't
	// directly queried—they're used transitively. But we can verify the relationship was written
	// by checking a catalogue access chain.
}

func TestCheck_UserGroupCatalogueToLLM(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:10", Relation: "member", Resource: "group:1"},
		{Subject: "group:1", Relation: "assigned_group", Resource: "catalogue:1"},
		{Subject: "catalogue:1", Relation: "parent_catalogue", Resource: "llm:5"},
	}))

	allowed, err := store.Check(ctx, 10, "can_use", "llm", 5)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = store.Check(ctx, 99, "can_use", "llm", 5)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestCheck_UserGroupDataCatalogueToDatasource(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:10", Relation: "member", Resource: "group:1"},
		{Subject: "group:1", Relation: "assigned_group", Resource: "data_catalogue:2"},
		{Subject: "data_catalogue:2", Relation: "parent_catalogue", Resource: "datasource:7"},
	}))

	allowed, err := store.Check(ctx, 10, "can_use", "datasource", 7)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestCheck_DatasourceOwner(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:5", Relation: "owner", Resource: "datasource:3"},
	}))

	allowed, err := store.Check(ctx, 5, "can_use", "datasource", 3)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = store.Check(ctx, 5, "can_admin", "datasource", 3)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = store.Check(ctx, 99, "can_use", "datasource", 3)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestCheck_UserGroupToolCatalogueToTool(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:10", Relation: "member", Resource: "group:1"},
		{Subject: "group:1", Relation: "assigned_group", Resource: "tool_catalogue:3"},
		{Subject: "tool_catalogue:3", Relation: "parent_catalogue", Resource: "tool:8"},
	}))

	allowed, err := store.Check(ctx, 10, "can_use", "tool", 8)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestCheck_AppOwnership(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:5", Relation: "owner", Resource: "app:10"},
	}))

	allowed, err := store.Check(ctx, 5, "can_use", "app", 10)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = store.Check(ctx, 5, "editor", "app", 10)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestCheck_AppSharing(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:20", Relation: "viewer", Resource: "app:10"},
	}))

	allowed, err := store.Check(ctx, 20, "can_use", "app", 10)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = store.Check(ctx, 20, "editor", "app", 10)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestCheck_AppSharingViaGroup(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:30", Relation: "member", Resource: "group:3"},
		{Subject: "group:3#member", Relation: "viewer", Resource: "app:10"},
	}))

	allowed, err := store.Check(ctx, 30, "can_use", "app", 10)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestCheck_ChatGroupAccess(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:10", Relation: "member", Resource: "group:1"},
		{Subject: "group:1", Relation: "assigned_group", Resource: "chat:5"},
	}))

	allowed, err := store.Check(ctx, 10, "viewer", "chat", 5)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestCheck_SubmissionAccess(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:10", Relation: "submitter", Resource: "submission:1"},
		{Subject: "user:20", Relation: "reviewer", Resource: "submission:1"},
	}))

	allowed, err := store.Check(ctx, 10, "can_view", "submission", 1)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = store.Check(ctx, 20, "can_view", "submission", 1)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = store.Check(ctx, 20, "can_review", "submission", 1)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = store.Check(ctx, 10, "can_review", "submission", 1)
	require.NoError(t, err)
	assert.False(t, allowed)

	allowed, err = store.Check(ctx, 99, "can_view", "submission", 1)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestCheck_PluginInstaller(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:1", Relation: "installer", Resource: "plugin:5"},
	}))

	allowed, err := store.Check(ctx, 1, "can_admin", "plugin", 5)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = store.Check(ctx, 99, "can_admin", "plugin", 5)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestListResources_LLMs(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:10", Relation: "member", Resource: "group:1"},
		{Subject: "group:1", Relation: "assigned_group", Resource: "catalogue:1"},
		{Subject: "catalogue:1", Relation: "parent_catalogue", Resource: "llm:5"},
		{Subject: "catalogue:1", Relation: "parent_catalogue", Resource: "llm:6"},
	}))

	ids, err := store.ListResources(ctx, 10, "can_use", "llm")
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{5, 6}, ids)

	ids, err = store.ListResources(ctx, 99, "can_use", "llm")
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestRevoke(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:1", Relation: "admin", Resource: "system:1"},
	}))

	allowed, err := store.CheckByName(ctx, 1, "admin", "system", "1")
	require.NoError(t, err)
	assert.True(t, allowed)

	require.NoError(t, store.Revoke(ctx, []authz.Relationship{
		{Subject: "user:1", Relation: "admin", Resource: "system:1"},
	}))

	allowed, err = store.CheckByName(ctx, 1, "admin", "system", "1")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestGrantAndRevoke_Atomic(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:1", Relation: "admin", Resource: "system:1"},
	}))

	require.NoError(t, store.GrantAndRevoke(ctx,
		[]authz.Relationship{{Subject: "user:2", Relation: "admin", Resource: "system:1"}},
		[]authz.Relationship{{Subject: "user:1", Relation: "admin", Resource: "system:1"}},
	))

	allowed, err := store.CheckByName(ctx, 1, "admin", "system", "1")
	require.NoError(t, err)
	assert.False(t, allowed)

	allowed, err = store.CheckByName(ctx, 2, "admin", "system", "1")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestCheck_MultiGroupAccess(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:10", Relation: "member", Resource: "group:1"},
		{Subject: "user:10", Relation: "member", Resource: "group:2"},
		{Subject: "group:1", Relation: "assigned_group", Resource: "catalogue:1"},
		{Subject: "group:2", Relation: "assigned_group", Resource: "catalogue:2"},
		{Subject: "catalogue:1", Relation: "parent_catalogue", Resource: "llm:5"},
		{Subject: "catalogue:2", Relation: "parent_catalogue", Resource: "llm:6"},
	}))

	ids, err := store.ListResources(ctx, 10, "can_use", "llm")
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{5, 6}, ids)
}

func TestListResourcesByName_PluginResources(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Grant(ctx, []authz.Relationship{
		{Subject: "user:10", Relation: "member", Resource: "group:1"},
		{Subject: "group:1", Relation: "assigned_group", Resource: "plugin_resource:5_srv-1"},
		{Subject: "group:1", Relation: "assigned_group", Resource: "plugin_resource:5_srv-2"},
	}))

	objects, err := store.ListResourcesByName(ctx, 10, "can_use", "plugin_resource")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"plugin_resource:5_srv-1", "plugin_resource:5_srv-2"}, objects)

	_, err = store.ListResources(ctx, 10, "can_use", "plugin_resource")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-numeric resource ID")
}

func TestStore_Enabled(t *testing.T) {
	store := newTestStore(t)
	assert.True(t, store.Enabled())
}
