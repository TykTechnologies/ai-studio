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
