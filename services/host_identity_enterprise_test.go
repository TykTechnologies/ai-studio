//go:build enterprise

package services_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/rbac"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/rbac"
)

// With Enterprise roles the host's admin flag is an Administrator binding:
// the evaluator derives is_admin from bindings, so setting the column alone
// would be undone.
func TestProvisionHostUserAdminIsARoleBinding(t *testing.T) {
	svc := newHostIdentityService(t)
	require.True(t, svc.Authz().Enabled(), "Enterprise RBAC should be active")
	require.NoError(t, svc.Authz().Seed(context.Background()))

	// A local Owner keeps the last-Owner rule out of the way.
	_, err := svc.CreateUser(services.UserDTO{Email: "owner@example.com", Name: "Owner", Password: "a-long-password-1", IsAdmin: true, EmailVerified: true})
	require.NoError(t, err)

	user, err := svc.ProvisionHostUser(services.HostIdentity{Subject: "h-1", Email: "ops@example.com", Name: "Ops", Admin: true})
	require.NoError(t, err)
	assert.True(t, user.IsAdmin)
	bindings, err := svc.Authz().ListBindings(context.Background(), rbac.BindingFilter{SubjectType: models.RoleBindingSubjectUser, SubjectID: user.ID})
	require.NoError(t, err)
	assert.NotEmpty(t, bindings, "admin is granted through a role binding")

	user, err = svc.ProvisionHostUser(services.HostIdentity{Subject: "h-1", Email: "ops@example.com", Name: "Ops"})
	require.NoError(t, err)
	assert.False(t, user.IsAdmin)
	var stored models.User
	require.NoError(t, svc.DB.First(&stored, user.ID).Error)
	assert.False(t, stored.IsAdmin, "the revocation sticks")
}
