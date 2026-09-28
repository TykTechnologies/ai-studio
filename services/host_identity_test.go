package services_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

func newHostIdentityService(t *testing.T) *services.Service {
	t.Helper()
	return services.NewService(apitest.SetupTestDB(t))
}

func groupNames(u *models.User) []string {
	var names []string
	for _, g := range u.Groups {
		names = append(names, g.Name)
	}
	return names
}

func TestProvisionHostUserCreatesOnFirstSight(t *testing.T) {
	svc := newHostIdentityService(t)

	user, err := svc.ProvisionHostUser(services.HostIdentity{Subject: "u-1", Email: "ada@example.com", Name: "Ada"})
	require.NoError(t, err)

	assert.Equal(t, models.AuthSourceHost, user.AuthSource)
	assert.Equal(t, "u-1", user.ExternalSubject)
	assert.True(t, user.EmailVerified)
	assert.False(t, user.IsAdmin)
	assert.Equal(t, models.LoginMethodHost, user.LastLoginMethod)
	assert.NotNil(t, user.LastLoginAt)
	assert.True(t, user.IsExternallyManaged())

	again, err := svc.ProvisionHostUser(services.HostIdentity{Subject: "u-1", Email: "ada@example.com", Name: "Ada"})
	require.NoError(t, err)
	assert.Equal(t, user.ID, again.ID, "the subject finds the same user")

	var count int64
	svc.DB.Model(&models.User{}).Count(&count)
	assert.EqualValues(t, 1, count)
}

func TestProvisionHostUserFollowsTheHost(t *testing.T) {
	svc := newHostIdentityService(t)
	engineering := &models.Group{Name: "Engineering"}
	require.NoError(t, svc.DB.Create(engineering).Error)

	user, err := svc.ProvisionHostUser(services.HostIdentity{Subject: "u-1", Email: "ada@example.com", Name: "Ada"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{models.DefaultGroupName}, groupNames(user))

	user, err = svc.ProvisionHostUser(services.HostIdentity{
		Subject: "u-1", Email: "ada.lovelace@example.com", Name: "Ada Lovelace", Admin: true,
		Groups: []string{"Engineering", "No Such Team"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ada.lovelace@example.com", user.Email)
	assert.Equal(t, "Ada Lovelace", user.Name)
	assert.True(t, user.IsAdmin)
	assert.ElementsMatch(t, []string{models.DefaultGroupName, "Engineering"}, groupNames(user), "unknown groups are skipped and Default is kept")

	// Nil groups leave memberships to Studio; admin is revoked when the
	// host says so.
	user, err = svc.ProvisionHostUser(services.HostIdentity{Subject: "u-1", Email: "ada.lovelace@example.com", Name: "Ada Lovelace"})
	require.NoError(t, err)
	assert.False(t, user.IsAdmin)
	assert.ElementsMatch(t, []string{models.DefaultGroupName, "Engineering"}, groupNames(user))
}

func TestProvisionHostUserLinksAnExistingAccountByEmail(t *testing.T) {
	svc := newHostIdentityService(t)
	existing, err := svc.CreateUser(services.UserDTO{Email: "grace@example.com", Name: "Grace", Password: "a-long-password-1", EmailVerified: true})
	require.NoError(t, err)

	user, err := svc.ProvisionHostUser(services.HostIdentity{Subject: "u-2", Email: "grace@example.com", Name: "Grace"})
	require.NoError(t, err)
	assert.Equal(t, existing.ID, user.ID)
	assert.Equal(t, "u-2", user.ExternalSubject)
	assert.Equal(t, models.AuthSourceAdmin, user.AuthSource, "provenance records how the account was created")

	_, err = svc.ProvisionHostUser(services.HostIdentity{Subject: "u-3", Email: "grace@example.com"})
	assert.ErrorIs(t, err, services.ErrHostIdentityConflict, "an email linked to another subject is not taken over")
}

func TestProvisionHostUserRefusesDisabledAndInvalidIdentities(t *testing.T) {
	svc := newHostIdentityService(t)

	_, err := svc.ProvisionHostUser(services.HostIdentity{Email: "no-subject@example.com"})
	assert.ErrorIs(t, err, services.ErrHostIdentityInvalid)
	_, err = svc.ProvisionHostUser(services.HostIdentity{Subject: "u-4"})
	assert.ErrorIs(t, err, services.ErrHostIdentityInvalid)

	user, err := svc.ProvisionHostUser(services.HostIdentity{Subject: "u-5", Email: "eve@example.com"})
	require.NoError(t, err)
	require.NoError(t, svc.DB.Model(user).Update("disabled", true).Error)

	_, err = svc.ProvisionHostUser(services.HostIdentity{Subject: "u-5", Email: "eve@example.com"})
	assert.ErrorIs(t, err, services.ErrHostUserDisabled)
}

// A deleted host user comes back as a new account: the soft-deleted row does
// not hold the subject.
func TestProvisionHostUserAfterDeletion(t *testing.T) {
	svc := newHostIdentityService(t)

	first, err := svc.ProvisionHostUser(services.HostIdentity{Subject: "u-6", Email: "sam@example.com"})
	require.NoError(t, err)
	require.NoError(t, svc.DB.Delete(first).Error)

	second, err := svc.ProvisionHostUser(services.HostIdentity{Subject: "u-6", Email: "sam@example.com"})
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, second.ID)
	assert.Equal(t, "u-6", second.ExternalSubject)
}
