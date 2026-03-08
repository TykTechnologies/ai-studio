package authz

import (
	"context"
	"testing"

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

	// Write: user:1 is admin of system:1
	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:1", Relation: "admin", Object: "system:1"},
	}))

	allowed, err := store.CheckStr(ctx, 1, "admin", "system", "1")
	require.NoError(t, err)
	assert.True(t, allowed)

	// user:2 is not admin
	allowed, err = store.CheckStr(ctx, 2, "admin", "system", "1")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestCheck_SSOAdmin_InheritedFromAdmin(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// user:1 is admin -> should also be sso_admin (sso_admin = [user] or admin)
	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:1", Relation: "admin", Object: "system:1"},
	}))

	allowed, err := store.CheckStr(ctx, 1, "sso_admin", "system", "1")
	require.NoError(t, err)
	assert.True(t, allowed, "admin should inherit sso_admin")
}

func TestCheck_GroupMembership(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:10", Relation: "member", Object: "group:1"},
	}))

	// No direct check API for group membership in our Authorizer since groups aren't
	// directly queried—they're used transitively. But we can verify the tuple was written
	// by checking a catalogue access chain.
}

func TestCheck_UserGroupCatalogueToLLM(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// Set up the full chain: user:10 -> group:1 -> catalogue:1 -> llm:5
	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:10", Relation: "member", Object: "group:1"},
		{User: "group:1", Relation: "assigned_group", Object: "catalogue:1"},
		{User: "catalogue:1", Relation: "parent_catalogue", Object: "llm:5"},
	}))

	// user:10 should be able to use llm:5
	allowed, err := store.Check(ctx, 10, "can_use", "llm", 5)
	require.NoError(t, err)
	assert.True(t, allowed)

	// user:99 should not be able to use llm:5
	allowed, err = store.Check(ctx, 99, "can_use", "llm", 5)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestCheck_UserGroupDataCatalogueToDatasource(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:10", Relation: "member", Object: "group:1"},
		{User: "group:1", Relation: "assigned_group", Object: "data_catalogue:2"},
		{User: "data_catalogue:2", Relation: "parent_catalogue", Object: "datasource:7"},
	}))

	allowed, err := store.Check(ctx, 10, "can_use", "datasource", 7)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestCheck_DatasourceOwner(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:5", Relation: "owner", Object: "datasource:3"},
	}))

	// Owner can_use
	allowed, err := store.Check(ctx, 5, "can_use", "datasource", 3)
	require.NoError(t, err)
	assert.True(t, allowed)

	// Owner can_admin
	allowed, err = store.Check(ctx, 5, "can_admin", "datasource", 3)
	require.NoError(t, err)
	assert.True(t, allowed)

	// Non-owner cannot
	allowed, err = store.Check(ctx, 99, "can_use", "datasource", 3)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestCheck_UserGroupToolCatalogueToTool(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:10", Relation: "member", Object: "group:1"},
		{User: "group:1", Relation: "assigned_group", Object: "tool_catalogue:3"},
		{User: "tool_catalogue:3", Relation: "parent_catalogue", Object: "tool:8"},
	}))

	allowed, err := store.Check(ctx, 10, "can_use", "tool", 8)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestCheck_AppOwnership(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:5", Relation: "owner", Object: "app:10"},
	}))

	// Owner -> editor -> can_use
	allowed, err := store.Check(ctx, 5, "can_use", "app", 10)
	require.NoError(t, err)
	assert.True(t, allowed)

	// Owner -> editor
	allowed, err = store.Check(ctx, 5, "editor", "app", 10)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestCheck_AppSharing(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// Share app:10 with user:20 as viewer
	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:20", Relation: "viewer", Object: "app:10"},
	}))

	allowed, err := store.Check(ctx, 20, "can_use", "app", 10)
	require.NoError(t, err)
	assert.True(t, allowed)

	// viewer is not editor
	allowed, err = store.Check(ctx, 20, "editor", "app", 10)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestCheck_AppSharingViaGroup(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// Share app:10 with group:3 members as viewers
	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:30", Relation: "member", Object: "group:3"},
		{User: "group:3#member", Relation: "viewer", Object: "app:10"},
	}))

	allowed, err := store.Check(ctx, 30, "can_use", "app", 10)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestCheck_ChatGroupAccess(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:10", Relation: "member", Object: "group:1"},
		{User: "group:1", Relation: "assigned_group", Object: "chat:5"},
	}))

	allowed, err := store.Check(ctx, 10, "viewer", "chat", 5)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestCheck_SubmissionAccess(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:10", Relation: "submitter", Object: "submission:1"},
		{User: "user:20", Relation: "reviewer", Object: "submission:1"},
	}))

	// Submitter can view
	allowed, err := store.Check(ctx, 10, "can_view", "submission", 1)
	require.NoError(t, err)
	assert.True(t, allowed)

	// Reviewer can view
	allowed, err = store.Check(ctx, 20, "can_view", "submission", 1)
	require.NoError(t, err)
	assert.True(t, allowed)

	// Reviewer can review
	allowed, err = store.Check(ctx, 20, "can_review", "submission", 1)
	require.NoError(t, err)
	assert.True(t, allowed)

	// Submitter cannot review
	allowed, err = store.Check(ctx, 10, "can_review", "submission", 1)
	require.NoError(t, err)
	assert.False(t, allowed)

	// Random user can't view
	allowed, err = store.Check(ctx, 99, "can_view", "submission", 1)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestCheck_PluginInstaller(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:1", Relation: "installer", Object: "plugin:5"},
	}))

	allowed, err := store.Check(ctx, 1, "can_admin", "plugin", 5)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = store.Check(ctx, 99, "can_admin", "plugin", 5)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestListObjects_LLMs(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// user:10 can use llm:5 and llm:6 through group:1 -> catalogue:1
	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:10", Relation: "member", Object: "group:1"},
		{User: "group:1", Relation: "assigned_group", Object: "catalogue:1"},
		{User: "catalogue:1", Relation: "parent_catalogue", Object: "llm:5"},
		{User: "catalogue:1", Relation: "parent_catalogue", Object: "llm:6"},
	}))

	ids, err := store.ListObjects(ctx, 10, "can_use", "llm")
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{5, 6}, ids)

	// user:99 has no access
	ids, err = store.ListObjects(ctx, 99, "can_use", "llm")
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestDeleteTuples(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:1", Relation: "admin", Object: "system:1"},
	}))

	allowed, err := store.CheckStr(ctx, 1, "admin", "system", "1")
	require.NoError(t, err)
	assert.True(t, allowed)

	// Delete the tuple
	require.NoError(t, store.DeleteTuples(ctx, []Tuple{
		{User: "user:1", Relation: "admin", Object: "system:1"},
	}))

	allowed, err = store.CheckStr(ctx, 1, "admin", "system", "1")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestWriteTuplesAndDelete_Atomic(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// First write admin
	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:1", Relation: "admin", Object: "system:1"},
	}))

	// Atomically: remove user:1 admin, add user:2 admin
	require.NoError(t, store.WriteTuplesAndDelete(ctx,
		[]Tuple{{User: "user:2", Relation: "admin", Object: "system:1"}},
		[]Tuple{{User: "user:1", Relation: "admin", Object: "system:1"}},
	))

	allowed, err := store.CheckStr(ctx, 1, "admin", "system", "1")
	require.NoError(t, err)
	assert.False(t, allowed)

	allowed, err = store.CheckStr(ctx, 2, "admin", "system", "1")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestCheck_MultiGroupAccess(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// user:10 is in group:1 and group:2
	// group:1 has catalogue:1 with llm:5
	// group:2 has catalogue:2 with llm:6
	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:10", Relation: "member", Object: "group:1"},
		{User: "user:10", Relation: "member", Object: "group:2"},
		{User: "group:1", Relation: "assigned_group", Object: "catalogue:1"},
		{User: "group:2", Relation: "assigned_group", Object: "catalogue:2"},
		{User: "catalogue:1", Relation: "parent_catalogue", Object: "llm:5"},
		{User: "catalogue:2", Relation: "parent_catalogue", Object: "llm:6"},
	}))

	// Should have access to both LLMs
	ids, err := store.ListObjects(ctx, 10, "can_use", "llm")
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{5, 6}, ids)
}

func TestListObjectsStr_PluginResources(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// Plugin resource with composite ID
	require.NoError(t, store.WriteTuples(ctx, []Tuple{
		{User: "user:10", Relation: "member", Object: "group:1"},
		{User: "group:1", Relation: "assigned_group", Object: "plugin_resource:5_srv-1"},
		{User: "group:1", Relation: "assigned_group", Object: "plugin_resource:5_srv-2"},
	}))

	// ListObjectsStr works with composite IDs
	objects, err := store.ListObjectsStr(ctx, 10, "can_use", "plugin_resource")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"plugin_resource:5_srv-1", "plugin_resource:5_srv-2"}, objects)

	// ListObjects fails on composite IDs (returns error, not silent skip)
	_, err = store.ListObjects(ctx, 10, "can_use", "plugin_resource")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-numeric object ID")
}

func TestStore_Enabled(t *testing.T) {
	store := newTestStore(t)
	assert.True(t, store.Enabled())
}

func TestParseObjectStr(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"llm:42", "42", false},
		{"plugin_resource:5_srv-1", "5_srv-1", false},
		{"system:1", "1", false},
		{"invalid", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseObjectStr(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
