package authz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// recordingAuthorizer captures Grant/Revoke calls for testing.
type recordingAuthorizer struct {
	NoopAuthorizer
	grants      []Relationship
	revocations []Relationship
}

func (r *recordingAuthorizer) Enabled() bool { return true }

func (r *recordingAuthorizer) Grant(_ context.Context, rels []Relationship) error {
	r.grants = append(r.grants, rels...)
	return nil
}

func (r *recordingAuthorizer) Revoke(_ context.Context, rels []Relationship) error {
	r.revocations = append(r.revocations, rels...)
	return nil
}

func (r *recordingAuthorizer) GrantAndRevoke(_ context.Context, grants, revocations []Relationship) error {
	r.grants = append(r.grants, grants...)
	r.revocations = append(r.revocations, revocations...)
	return nil
}

func TestSyncer_OnUserCreated_Admin(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnUserCreated(context.Background(), 42, true, true)

	assert.Len(t, rec.grants, 3)
	assert.Contains(t, rec.grants, Relationship{Subject: "user:42", Relation: "member", Resource: "system:1"})
	assert.Contains(t, rec.grants, Relationship{Subject: "user:42", Relation: "admin", Resource: "system:1"})
	assert.Contains(t, rec.grants, Relationship{Subject: "user:42", Relation: "sso_admin", Resource: "system:1"})
}

func TestSyncer_OnUserCreated_Regular(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnUserCreated(context.Background(), 10, false, false)

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "user:10", Relation: "member", Resource: "system:1"})
}

func TestSyncer_OnUserUpdated_PromotedToAdmin(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnUserUpdated(context.Background(), 10, true, false, false, false)

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "user:10", Relation: "admin", Resource: "system:1"})
	assert.Empty(t, rec.revocations)
}

func TestSyncer_OnUserUpdated_DemotedFromAdmin(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnUserUpdated(context.Background(), 10, false, true, false, false)

	assert.Empty(t, rec.grants)
	assert.Len(t, rec.revocations, 1)
	assert.Contains(t, rec.revocations, Relationship{Subject: "user:10", Relation: "admin", Resource: "system:1"})
}

func TestSyncer_OnUserUpdated_NoChange(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnUserUpdated(context.Background(), 10, true, true, false, false)

	assert.Empty(t, rec.grants)
	assert.Empty(t, rec.revocations)
}

func TestSyncer_OnUserDeleted(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnUserDeleted(context.Background(), 42)

	assert.Len(t, rec.revocations, 3)
}

func TestSyncer_OnUserAddedToGroup(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnUserAddedToGroup(context.Background(), 10, 1)

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "user:10", Relation: "member", Resource: "group:1"})
}

func TestSyncer_OnGroupMembersReplaced(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnGroupMembersReplaced(context.Background(), 1, []uint{10, 20}, []uint{20, 30})

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "user:30", Relation: "member", Resource: "group:1"})
	assert.Len(t, rec.revocations, 1)
	assert.Contains(t, rec.revocations, Relationship{Subject: "user:10", Relation: "member", Resource: "group:1"})
}

func TestSyncer_OnCatalogueAssignedToGroup(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnCatalogueAssignedToGroup(context.Background(), "catalogue", 5, 1)

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "group:1", Relation: "assigned_group", Resource: "catalogue:5"})
}

func TestSyncer_OnOwnershipChanged(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnOwnershipChanged(context.Background(), 10, 20, "app", 5)

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "user:20", Relation: "owner", Resource: "app:5"})
	assert.Len(t, rec.revocations, 1)
	assert.Contains(t, rec.revocations, Relationship{Subject: "user:10", Relation: "owner", Resource: "app:5"})
}

func TestSyncer_OnOwnershipChanged_SameOwner(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnOwnershipChanged(context.Background(), 10, 10, "app", 5)

	assert.Empty(t, rec.grants)
	assert.Empty(t, rec.revocations)
}

func TestSyncer_OnUserRemovedFromGroup(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnUserRemovedFromGroup(context.Background(), 10, 1)

	assert.Len(t, rec.revocations, 1)
	assert.Contains(t, rec.revocations, Relationship{Subject: "user:10", Relation: "member", Resource: "group:1"})
}

func TestSyncer_OnCatalogueRemovedFromGroup(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnCatalogueRemovedFromGroup(context.Background(), "data_catalogue", 3, 1)

	assert.Len(t, rec.revocations, 1)
	assert.Contains(t, rec.revocations, Relationship{Subject: "group:1", Relation: "assigned_group", Resource: "data_catalogue:3"})
}

func TestSyncer_OnResourceAddedToCatalogue(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnResourceAddedToCatalogue(context.Background(), "catalogue", 1, "llm", 5)

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "catalogue:1", Relation: "parent_catalogue", Resource: "llm:5"})
}

func TestSyncer_OnResourceRemovedFromCatalogue(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnResourceRemovedFromCatalogue(context.Background(), "catalogue", 1, "llm", 5)

	assert.Len(t, rec.revocations, 1)
	assert.Contains(t, rec.revocations, Relationship{Subject: "catalogue:1", Relation: "parent_catalogue", Resource: "llm:5"})
}

func TestSyncer_OnOwnershipSet(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnOwnershipSet(context.Background(), 10, "datasource", 3)

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "user:10", Relation: "owner", Resource: "datasource:3"})
}

func TestSyncer_OnAppShared(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnAppShared(context.Background(), "user:20", "viewer", 10)

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "user:20", Relation: "viewer", Resource: "app:10"})
}

func TestSyncer_OnAppUnshared(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnAppUnshared(context.Background(), "user:20", "editor", 10)

	assert.Len(t, rec.revocations, 1)
	assert.Contains(t, rec.revocations, Relationship{Subject: "user:20", Relation: "editor", Resource: "app:10"})
}

func TestSyncer_OnChatGroupAssigned(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnChatGroupAssigned(context.Background(), 1, 5)

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "group:1", Relation: "assigned_group", Resource: "chat:5"})
}

func TestSyncer_OnChatGroupRemoved(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnChatGroupRemoved(context.Background(), 1, 5)

	assert.Len(t, rec.revocations, 1)
	assert.Contains(t, rec.revocations, Relationship{Subject: "group:1", Relation: "assigned_group", Resource: "chat:5"})
}

func TestSyncer_OnSubmissionCreated(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnSubmissionCreated(context.Background(), 10, 1)

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "user:10", Relation: "submitter", Resource: "submission:1"})
}

func TestSyncer_OnReviewerAssigned(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnReviewerAssigned(context.Background(), 20, 1)

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "user:20", Relation: "reviewer", Resource: "submission:1"})
}

func TestSyncer_OnPluginInstalled(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnPluginInstalled(context.Background(), 10, 5)

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "user:10", Relation: "installer", Resource: "plugin:5"})
}

func TestSyncer_OnPluginResourceGroupAssigned(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnPluginResourceGroupAssigned(context.Background(), 1, "plugin_resource:5_srv-1")

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "group:1", Relation: "assigned_group", Resource: "plugin_resource:5_srv-1"})
}

func TestSyncer_OnPluginResourceGroupRemoved(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnPluginResourceGroupRemoved(context.Background(), 1, "plugin_resource:5_srv-1")

	assert.Len(t, rec.revocations, 1)
	assert.Contains(t, rec.revocations, Relationship{Subject: "group:1", Relation: "assigned_group", Resource: "plugin_resource:5_srv-1"})
}

func TestSyncer_OnUserUpdated_SSOPromotion(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnUserUpdated(context.Background(), 10, false, false, true, false)

	assert.Len(t, rec.grants, 1)
	assert.Contains(t, rec.grants, Relationship{Subject: "user:10", Relation: "sso_admin", Resource: "system:1"})
	assert.Empty(t, rec.revocations)
}

func TestSyncer_OnUserUpdated_SSODemotion(t *testing.T) {
	rec := &recordingAuthorizer{}
	syncer := NewSyncer(rec)

	syncer.OnUserUpdated(context.Background(), 10, false, false, false, true)

	assert.Empty(t, rec.grants)
	assert.Len(t, rec.revocations, 1)
	assert.Contains(t, rec.revocations, Relationship{Subject: "user:10", Relation: "sso_admin", Resource: "system:1"})
}

func TestSyncer_NoopWhenDisabled(t *testing.T) {
	noop := &NoopAuthorizer{}
	syncer := NewSyncer(noop)

	// None of these should panic or make calls.
	ctx := context.Background()
	syncer.OnUserCreated(ctx, 1, true, true)
	syncer.OnUserUpdated(ctx, 1, true, false, false, false)
	syncer.OnUserDeleted(ctx, 1)
	syncer.OnUserAddedToGroup(ctx, 1, 1)
	syncer.OnCatalogueAssignedToGroup(ctx, "catalogue", 1, 1)
	syncer.OnOwnershipSet(ctx, 1, "app", 1)
}

func TestDiffUints(t *testing.T) {
	removed, added := diffUints([]uint{1, 2, 3}, []uint{2, 3, 4})
	assert.ElementsMatch(t, []uint{1}, removed)
	assert.ElementsMatch(t, []uint{4}, added)

	removed, added = diffUints([]uint{1, 2}, []uint{1, 2})
	assert.Empty(t, removed)
	assert.Empty(t, added)

	removed, added = diffUints(nil, []uint{1, 2})
	assert.Empty(t, removed)
	assert.ElementsMatch(t, []uint{1, 2}, added)

	removed, added = diffUints([]uint{1, 2}, nil)
	assert.ElementsMatch(t, []uint{1, 2}, removed)
	assert.Empty(t, added)
}
