package rbac

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"gorm.io/gorm"
)

// communityService reproduces the historical admin-or-not behaviour and
// answers every management call with ErrEnterpriseFeature.
type communityService struct {
	db *gorm.DB
}

func newCommunityService(db *gorm.DB) Service {
	return &communityService{db: db}
}

func (s *communityService) Enabled() bool                                   { return false }
func (s *communityService) SetLicenseCheck(func() bool)                     {}
func (s *communityService) Seed(context.Context) error                      { return nil }
func (s *communityService) RefreshSystemRoles(context.Context) error        { return nil }
func (s *communityService) SyncAdminFlag(context.Context, ...uint) error    { return nil }
func (s *communityService) IsLastOwner(context.Context, uint) (bool, error) { return false, nil }

// Resolve is the CE rule: admins hold everything, everyone else nothing.
func (s *communityService) Resolve(_ context.Context, user *models.User) (authz.Set, error) {
	return ResolveByAdminFlag(user), nil
}

// ResolveByAdminFlag is the admin-or-not rule, shared with the enterprise
// implementation for when the feature is unlicensed.
func ResolveByAdminFlag(user *models.User) authz.Set {
	if user != nil && user.IsAdmin {
		return authz.Wildcard()
	}
	return authz.NewSet()
}

func (s *communityService) EffectivePermissions(_ context.Context, userID uint) (*Effective, error) {
	if s.db == nil {
		return &Effective{Permissions: authz.NewSet()}, nil
	}
	var user models.User
	if err := user.Get(s.db, userID); err != nil {
		return nil, ErrNotFound
	}
	return &Effective{Permissions: ResolveByAdminFlag(&user)}, nil
}

func (s *communityService) ListRoles(context.Context) ([]models.Role, map[uint]RoleCounts, error) {
	return nil, nil, ErrEnterpriseFeature
}

func (s *communityService) GetRole(context.Context, uint) (*models.Role, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) CreateRole(context.Context, *models.User, RoleInput) (*models.Role, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) UpdateRole(context.Context, *models.User, uint, RoleInput) (*models.Role, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) DeleteRole(context.Context, *models.User, uint) error {
	return ErrEnterpriseFeature
}

func (s *communityService) CloneRole(context.Context, *models.User, uint, string) (*models.Role, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) ListBindings(context.Context, BindingFilter) ([]models.RoleBinding, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) CreateBinding(context.Context, *models.User, BindingInput) (*models.RoleBinding, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) DeleteBinding(context.Context, *models.User, uint) error {
	return ErrEnterpriseFeature
}

func (s *communityService) DeleteBindingsForSubject(context.Context, Subject) error {
	return nil
}

func (s *communityService) RolesForSubjects(context.Context, string, []uint) (map[uint][]models.Role, error) {
	return map[uint][]models.Role{}, nil
}

func (s *communityService) SetFullAdmin(context.Context, *models.User, uint, bool) error {
	return nil
}
