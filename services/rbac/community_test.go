package rbac

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCommunity_ResolveByAdminFlag(t *testing.T) {
	s := newCommunityService(nil)
	assert.False(t, s.Enabled())

	set, err := s.Resolve(context.Background(), &models.User{IsAdmin: true})
	require.NoError(t, err)
	assert.True(t, set.IsFullAdmin())

	set, err = s.Resolve(context.Background(), &models.User{IsAdmin: false})
	require.NoError(t, err)
	assert.True(t, set.IsEmpty())

	assert.True(t, ResolveByAdminFlag(nil).IsEmpty())
}

func TestCommunity_ManagementIsEnterpriseOnly(t *testing.T) {
	s := newCommunityService(nil)
	ctx := context.Background()
	_, _, err := s.ListRoles(ctx)
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = s.CreateRole(ctx, nil, RoleInput{})
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = s.ListBindings(ctx, BindingFilter{})
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	assert.ErrorIs(t, s.DeleteBinding(ctx, nil, 1), ErrEnterpriseFeature)

	// Hooks the core calls unconditionally are no-ops.
	assert.NoError(t, s.Seed(ctx))
	assert.NoError(t, s.SyncAdminFlag(ctx, 1))
	assert.NoError(t, s.SetFullAdmin(ctx, nil, 1, true))
	assert.NoError(t, s.DeleteBindingsForSubject(ctx, Subject{Type: models.RoleBindingSubjectUser, ID: 1}))
	roles, err := s.RolesForSubjects(ctx, models.RoleBindingSubjectUser, []uint{1})
	assert.NoError(t, err)
	assert.Empty(t, roles)
}

func TestCommunity_EffectivePermissionsFromDB(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	admin := models.NewUser()
	admin.Email = "a@tyk.io"
	admin.IsAdmin = true
	require.NoError(t, admin.Create(db))

	s := NewService(db)
	if IsEnterpriseAvailable() {
		t.Skip("enterprise build registers its own factory")
	}
	eff, err := s.EffectivePermissions(context.Background(), admin.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{string(authz.FullAdmin)}, eff.Permissions.List())

	_, err = s.EffectivePermissions(context.Background(), 999)
	assert.ErrorIs(t, err, ErrNotFound)
}
