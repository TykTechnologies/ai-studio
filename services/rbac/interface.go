// Package rbac defines role-based access control for the management API:
// roles are named bundles of catalogue permissions (see pkg/authz), bound to
// users or groups; a user's effective permissions are the union across their
// direct bindings and the bindings of every group they belong to.
//
// Community Edition ships a permissive stub that reproduces the historical
// admin-or-not behaviour: an admin user resolves to the wildcard, anyone
// else to nothing, and every management call answers ErrEnterpriseFeature.
// Enterprise Edition registers the real implementation via the factory.
package rbac

import (
	"context"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
)

// ErrEnterpriseFeature is returned by Community Edition for every management call.
var ErrEnterpriseFeature = errors.New("role-based access control is an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")

// Rule and lookup errors shared by both editions.
var (
	ErrNotFound          = errors.New("not found")
	ErrSystemRole        = errors.New("system roles cannot be modified or deleted; clone the role to customise it")
	ErrLastOwner         = errors.New("the last Owner cannot be removed")
	ErrOwnerUserOnly     = errors.New("the Owner role can only be assigned to users, not teams")
	ErrOwnerRequired     = errors.New("only an Owner can assign or remove the Owner role")
	ErrInvalidPermission = errors.New("invalid permission")
	ErrInvalidSubject    = errors.New("invalid binding subject")
	ErrDuplicateRole     = errors.New("a role with that name already exists")
	ErrDuplicateBinding  = errors.New("that role is already assigned to this subject")
)

// Subject identifies who a role is bound to.
type Subject struct {
	Type string // models.RoleBindingSubjectUser | models.RoleBindingSubjectGroup
	ID   uint
}

// RoleInput is the payload for creating or updating a custom role.
type RoleInput struct {
	Name        string
	Description string
	Permissions []string
}

// BindingInput is the payload for assigning a role.
type BindingInput struct {
	Subject   Subject
	RoleID    uint
	ScopeType string
	ScopeID   string
}

// BindingFilter narrows ListBindings. Zero values mean "no filter".
type BindingFilter struct {
	SubjectType string
	SubjectID   uint
	RoleID      uint
}

// RoleSummary is the compact role shape returned alongside users, groups and
// the caller's own identity. Via and GroupID say how a user came to hold it.
type RoleSummary struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	IsSystem bool   `json:"is_system"`
	Via      string `json:"via,omitempty"`      // "direct" | "group"
	GroupID  uint   `json:"group_id,omitempty"` // when Via == "group"
}

// Effective is a user's resolved access: the permission set plus the roles
// that produced it.
type Effective struct {
	Permissions authz.Set
	Roles       []RoleSummary
}

// RoleCounts is how many users and groups hold a role directly.
type RoleCounts struct {
	Users  int64 `json:"users"`
	Groups int64 `json:"groups"`
}

// Service is the RBAC contract shared by both editions.
type Service interface {
	// Enabled reports whether fine-grained roles are active: an Enterprise
	// build whose licence carries the feature. When false, Resolve applies
	// the admin-or-not rule and management calls return ErrEnterpriseFeature.
	Enabled() bool

	// SetLicenseCheck installs the licence entitlement probe. nil means
	// "no licensing service" (tests, CE), which the enterprise implementation
	// treats as enabled and the community stub ignores.
	SetLicenseCheck(check func() bool)

	// Resolve computes the user's effective permission set. It is called at
	// most once per request by the authorization middleware.
	Resolve(ctx context.Context, user *models.User) (authz.Set, error)

	// EffectivePermissions returns the set and the roles behind it.
	EffectivePermissions(ctx context.Context, userID uint) (*Effective, error)

	// Roles.
	ListRoles(ctx context.Context) ([]models.Role, map[uint]RoleCounts, error)
	GetRole(ctx context.Context, id uint) (*models.Role, error)
	CreateRole(ctx context.Context, actor *models.User, in RoleInput) (*models.Role, error)
	UpdateRole(ctx context.Context, actor *models.User, id uint, in RoleInput) (*models.Role, error)
	DeleteRole(ctx context.Context, actor *models.User, id uint) error
	CloneRole(ctx context.Context, actor *models.User, id uint, name string) (*models.Role, error)

	// Bindings.
	ListBindings(ctx context.Context, f BindingFilter) ([]models.RoleBinding, error)
	CreateBinding(ctx context.Context, actor *models.User, in BindingInput) (*models.RoleBinding, error)
	DeleteBinding(ctx context.Context, actor *models.User, id uint) error
	// DeleteBindingsForSubject removes every binding for a user or group;
	// called when the subject itself is deleted. No-op in CE.
	DeleteBindingsForSubject(ctx context.Context, s Subject) error
	// RolesForSubjects batch-loads the directly bound roles for many subjects
	// so list endpoints avoid N+1 queries. Empty map in CE.
	RolesForSubjects(ctx context.Context, subjectType string, ids []uint) (map[uint][]models.Role, error)

	// SetFullAdmin adds or removes the Administrator binding for a user so
	// the legacy is_admin flag keeps working through the user API. No-op in
	// CE, where the user service writes the flag directly.
	SetFullAdmin(ctx context.Context, actor *models.User, userID uint, on bool) error
	// SyncAdminFlag recomputes users.is_admin from bindings for the given
	// users (all users when none given). Called after group membership
	// changes and SSO logins. No-op in CE.
	SyncAdminFlag(ctx context.Context, userIDs ...uint) error
	// IsLastOwner reports whether the user is the only Owner, and therefore
	// cannot be deleted or demoted. Always false in CE.
	IsLastOwner(ctx context.Context, userID uint) (bool, error)

	// Seed creates the system roles and migrates legacy admin flags into
	// bindings. Idempotent. No-op in CE.
	Seed(ctx context.Context) error
	// RefreshSystemRoles recomputes the permissions of the computed system
	// roles (Editor, Viewer, Auditor) from the catalogue. Called after a
	// plugin registers or removes permission resources at runtime, so the
	// roles reflect the catalogue without a restart. No-op in CE.
	RefreshSystemRoles(ctx context.Context) error
}
