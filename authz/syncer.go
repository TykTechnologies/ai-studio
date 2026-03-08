package authz

import (
	"context"

	"github.com/rs/zerolog/log"
)

// Syncer keeps the authorization store in sync with database mutations.
// Service-layer code calls these methods after successful writes to the database.
// When the authorizer is disabled (NoopAuthorizer), all methods are no-ops.
type Syncer struct {
	authz Authorizer
}

// NewSyncer creates a new Syncer that writes incremental changes to the authorizer.
func NewSyncer(authz Authorizer) *Syncer {
	return &Syncer{authz: authz}
}

// --- User mutations ---

// OnUserCreated grants the new user system membership and any admin/sso roles.
func (s *Syncer) OnUserCreated(ctx context.Context, userID uint, isAdmin, accessToSSOConfig bool) {
	var grants []Relationship
	grants = append(grants, Relationship{Subject: SubjectUser(userID), Relation: "member", Resource: "system:1"})
	if isAdmin {
		grants = append(grants, Relationship{Subject: SubjectUser(userID), Relation: "admin", Resource: "system:1"})
	}
	if accessToSSOConfig {
		grants = append(grants, Relationship{Subject: SubjectUser(userID), Relation: "sso_admin", Resource: "system:1"})
	}
	s.grant(ctx, grants)
}

// OnUserUpdated reconciles admin and SSO roles after a user update.
func (s *Syncer) OnUserUpdated(ctx context.Context, userID uint, isAdmin, wasAdmin, accessToSSOConfig, hadSSOConfig bool) {
	var grants, revocations []Relationship

	if isAdmin && !wasAdmin {
		grants = append(grants, Relationship{Subject: SubjectUser(userID), Relation: "admin", Resource: "system:1"})
	} else if !isAdmin && wasAdmin {
		revocations = append(revocations, Relationship{Subject: SubjectUser(userID), Relation: "admin", Resource: "system:1"})
	}

	if accessToSSOConfig && !hadSSOConfig {
		grants = append(grants, Relationship{Subject: SubjectUser(userID), Relation: "sso_admin", Resource: "system:1"})
	} else if !accessToSSOConfig && hadSSOConfig {
		revocations = append(revocations, Relationship{Subject: SubjectUser(userID), Relation: "sso_admin", Resource: "system:1"})
	}

	s.grantAndRevoke(ctx, grants, revocations)
}

// OnUserDeleted revokes all system-level roles for the user.
func (s *Syncer) OnUserDeleted(ctx context.Context, userID uint) {
	s.revoke(ctx, []Relationship{
		{Subject: SubjectUser(userID), Relation: "member", Resource: "system:1"},
		{Subject: SubjectUser(userID), Relation: "admin", Resource: "system:1"},
		{Subject: SubjectUser(userID), Relation: "sso_admin", Resource: "system:1"},
	})
}

// --- Group membership mutations ---

// OnUserAddedToGroup grants group membership.
func (s *Syncer) OnUserAddedToGroup(ctx context.Context, userID, groupID uint) {
	s.grant(ctx, []Relationship{
		{Subject: SubjectUser(userID), Relation: "member", Resource: SubjectGroup(groupID)},
	})
}

// OnUserRemovedFromGroup revokes group membership.
func (s *Syncer) OnUserRemovedFromGroup(ctx context.Context, userID, groupID uint) {
	s.revoke(ctx, []Relationship{
		{Subject: SubjectUser(userID), Relation: "member", Resource: SubjectGroup(groupID)},
	})
}

// OnGroupMembersReplaced replaces all members of a group.
func (s *Syncer) OnGroupMembersReplaced(ctx context.Context, groupID uint, oldUserIDs, newUserIDs []uint) {
	removed, added := diffUints(oldUserIDs, newUserIDs)
	var grants, revocations []Relationship
	for _, id := range added {
		grants = append(grants, Relationship{Subject: SubjectUser(id), Relation: "member", Resource: SubjectGroup(groupID)})
	}
	for _, id := range removed {
		revocations = append(revocations, Relationship{Subject: SubjectUser(id), Relation: "member", Resource: SubjectGroup(groupID)})
	}
	s.grantAndRevoke(ctx, grants, revocations)
}

// --- Catalogue assignment mutations ---

// OnCatalogueAssignedToGroup grants catalogue access for a group.
func (s *Syncer) OnCatalogueAssignedToGroup(ctx context.Context, catalogueType string, catalogueID, groupID uint) {
	s.grant(ctx, []Relationship{
		{Subject: SubjectGroup(groupID), Relation: "assigned_group", Resource: ResourceID(catalogueType, catalogueID)},
	})
}

// OnCatalogueRemovedFromGroup revokes catalogue access for a group.
func (s *Syncer) OnCatalogueRemovedFromGroup(ctx context.Context, catalogueType string, catalogueID, groupID uint) {
	s.revoke(ctx, []Relationship{
		{Subject: SubjectGroup(groupID), Relation: "assigned_group", Resource: ResourceID(catalogueType, catalogueID)},
	})
}

// --- Resource-catalogue mutations ---

// OnResourceAddedToCatalogue grants parent catalogue relationship for a resource.
func (s *Syncer) OnResourceAddedToCatalogue(ctx context.Context, catalogueType string, catalogueID uint, resourceType string, resourceID uint) {
	s.grant(ctx, []Relationship{
		{Subject: ResourceID(catalogueType, catalogueID), Relation: "parent_catalogue", Resource: ResourceID(resourceType, resourceID)},
	})
}

// OnResourceRemovedFromCatalogue revokes parent catalogue relationship for a resource.
func (s *Syncer) OnResourceRemovedFromCatalogue(ctx context.Context, catalogueType string, catalogueID uint, resourceType string, resourceID uint) {
	s.revoke(ctx, []Relationship{
		{Subject: ResourceID(catalogueType, catalogueID), Relation: "parent_catalogue", Resource: ResourceID(resourceType, resourceID)},
	})
}

// --- Ownership mutations ---

// OnOwnershipSet grants ownership of a resource to a user.
func (s *Syncer) OnOwnershipSet(ctx context.Context, userID uint, resourceType string, resourceID uint) {
	s.grant(ctx, []Relationship{
		{Subject: SubjectUser(userID), Relation: "owner", Resource: ResourceID(resourceType, resourceID)},
	})
}

// OnOwnershipChanged replaces ownership of a resource.
func (s *Syncer) OnOwnershipChanged(ctx context.Context, oldOwnerID, newOwnerID uint, resourceType string, resourceID uint) {
	if oldOwnerID == newOwnerID {
		return
	}
	var grants, revocations []Relationship
	if newOwnerID > 0 {
		grants = append(grants, Relationship{Subject: SubjectUser(newOwnerID), Relation: "owner", Resource: ResourceID(resourceType, resourceID)})
	}
	if oldOwnerID > 0 {
		revocations = append(revocations, Relationship{Subject: SubjectUser(oldOwnerID), Relation: "owner", Resource: ResourceID(resourceType, resourceID)})
	}
	s.grantAndRevoke(ctx, grants, revocations)
}

// --- App sharing mutations ---

// OnAppShared grants a viewer or editor relation on an app.
func (s *Syncer) OnAppShared(ctx context.Context, subject string, relation string, appID uint) {
	s.grant(ctx, []Relationship{
		{Subject: subject, Relation: relation, Resource: ResourceID("app", appID)},
	})
}

// OnAppUnshared revokes a viewer or editor relation on an app.
func (s *Syncer) OnAppUnshared(ctx context.Context, subject string, relation string, appID uint) {
	s.revoke(ctx, []Relationship{
		{Subject: subject, Relation: relation, Resource: ResourceID("app", appID)},
	})
}

// --- Chat group mutations ---

// OnChatGroupAssigned grants group access to a chat.
func (s *Syncer) OnChatGroupAssigned(ctx context.Context, groupID, chatID uint) {
	s.grant(ctx, []Relationship{
		{Subject: SubjectGroup(groupID), Relation: "assigned_group", Resource: ResourceID("chat", chatID)},
	})
}

// OnChatGroupRemoved revokes group access from a chat.
func (s *Syncer) OnChatGroupRemoved(ctx context.Context, groupID, chatID uint) {
	s.revoke(ctx, []Relationship{
		{Subject: SubjectGroup(groupID), Relation: "assigned_group", Resource: ResourceID("chat", chatID)},
	})
}

// --- Submission mutations ---

// OnSubmissionCreated grants submitter relation.
func (s *Syncer) OnSubmissionCreated(ctx context.Context, submitterID, submissionID uint) {
	s.grant(ctx, []Relationship{
		{Subject: SubjectUser(submitterID), Relation: "submitter", Resource: ResourceID("submission", submissionID)},
	})
}

// OnReviewerAssigned grants reviewer relation on a submission.
func (s *Syncer) OnReviewerAssigned(ctx context.Context, reviewerID, submissionID uint) {
	s.grant(ctx, []Relationship{
		{Subject: SubjectUser(reviewerID), Relation: "reviewer", Resource: ResourceID("submission", submissionID)},
	})
}

// --- Plugin mutations ---

// OnPluginInstalled grants installer relation on a plugin.
func (s *Syncer) OnPluginInstalled(ctx context.Context, installerID, pluginID uint) {
	s.grant(ctx, []Relationship{
		{Subject: SubjectUser(installerID), Relation: "installer", Resource: ResourceID("plugin", pluginID)},
	})
}

// --- Plugin resource mutations ---

// OnPluginResourceGroupAssigned grants group access to a plugin resource.
func (s *Syncer) OnPluginResourceGroupAssigned(ctx context.Context, groupID uint, resourceIdentifier string) {
	s.grant(ctx, []Relationship{
		{Subject: SubjectGroup(groupID), Relation: "assigned_group", Resource: resourceIdentifier},
	})
}

// OnPluginResourceGroupRemoved revokes group access from a plugin resource.
func (s *Syncer) OnPluginResourceGroupRemoved(ctx context.Context, groupID uint, resourceIdentifier string) {
	s.revoke(ctx, []Relationship{
		{Subject: SubjectGroup(groupID), Relation: "assigned_group", Resource: resourceIdentifier},
	})
}

// --- internal helpers ---

func (s *Syncer) grant(ctx context.Context, rels []Relationship) {
	if !s.authz.Enabled() || len(rels) == 0 {
		return
	}
	if err := s.authz.Grant(ctx, rels); err != nil {
		log.Error().Err(err).Msg("authz/syncer: failed to grant relationships")
	}
}

func (s *Syncer) revoke(ctx context.Context, rels []Relationship) {
	if !s.authz.Enabled() || len(rels) == 0 {
		return
	}
	if err := s.authz.Revoke(ctx, rels); err != nil {
		log.Error().Err(err).Msg("authz/syncer: failed to revoke relationships")
	}
}

func (s *Syncer) grantAndRevoke(ctx context.Context, grants, revocations []Relationship) {
	if !s.authz.Enabled() || (len(grants) == 0 && len(revocations) == 0) {
		return
	}
	if err := s.authz.GrantAndRevoke(ctx, grants, revocations); err != nil {
		log.Error().Err(err).Msg("authz/syncer: failed to grant/revoke relationships")
	}
}

// diffUints returns elements removed from old and added in new.
func diffUints(old, new []uint) (removed, added []uint) {
	oldSet := make(map[uint]struct{}, len(old))
	for _, v := range old {
		oldSet[v] = struct{}{}
	}
	newSet := make(map[uint]struct{}, len(new))
	for _, v := range new {
		newSet[v] = struct{}{}
	}
	for _, v := range old {
		if _, ok := newSet[v]; !ok {
			removed = append(removed, v)
		}
	}
	for _, v := range new {
		if _, ok := oldSet[v]; !ok {
			added = append(added, v)
		}
	}
	return
}
