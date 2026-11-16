package services

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// assertServiceAccountRefusal checks a 422 with code service_account.
func assertServiceAccountRefusal(t *testing.T, err error) {
	t.Helper()
	var he helpers.ErrorResponse
	require.True(t, errors.As(err, &he), "want a helpers.ErrorResponse, got %v", err)
	assert.Equal(t, http.StatusUnprocessableEntity, he.StatusCode, he.Message)
	assert.Equal(t, ServiceAccountCode, he.Code)
}

func createServiceAccount(t *testing.T, s *Service, name string) *models.User {
	t.Helper()
	user, err := s.CreateUser(UserDTO{Name: name, AccountType: models.AccountTypeService})
	require.NoError(t, err)
	return user
}

func TestCreateServiceAccount(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)
	// A Default team exists; a service account still joins none.
	_, err := models.GetOrCreateDefaultGroup(db)
	require.NoError(t, err)

	user := createServiceAccount(t, s, "Tyk Operator")

	var stored models.User
	require.NoError(t, db.Preload("Groups").First(&stored, user.ID).Error)
	assert.Equal(t, models.AccountTypeService, stored.AccountType)
	assert.True(t, stored.IsServiceAccount())
	assert.True(t, strings.HasPrefix(stored.Email, "svc-tyk-operator-"), stored.Email)
	assert.True(t, strings.HasSuffix(stored.Email, "@"+models.ServiceAccountEmailDomain), stored.Email)
	assert.Empty(t, stored.Password, "no password, not even a hash of the empty string")
	assert.NotEmpty(t, stored.APIKey, "the key is issued at creation")
	assert.Equal(t, user.APIKey, stored.APIKey)
	assert.True(t, stored.EmailVerified, "userForAPIKey refuses unverified accounts")
	assert.False(t, stored.ShowPortal)
	assert.False(t, stored.ShowChat)
	assert.False(t, stored.NotificationsEnabled)
	assert.False(t, stored.AccessToSSOConfig)
	assert.Empty(t, stored.Groups, "no team, not even Default")
	assert.Equal(t, models.AuthSourceAdmin, stored.AuthSource)
	assert.Equal(t, models.RoleServiceAccount, stored.GetRole())

	// Generated addresses are unique.
	other := createServiceAccount(t, s, "Tyk Operator")
	assert.NotEqual(t, stored.Email, other.Email)

	// A person created alongside still gets the Default team.
	person, err := s.CreateUser(UserDTO{Email: "person@example.com", Name: "Person", Password: "Sup3r-secret-pw!", ShowChat: true})
	require.NoError(t, err)
	var storedPerson models.User
	require.NoError(t, db.Preload("Groups").First(&storedPerson, person.ID).Error)
	assert.Len(t, storedPerson.Groups, 1)
	assert.Equal(t, models.AccountTypeUser, storedPerson.AccountType)

	counts, err := models.GetUserCounts(db)
	require.NoError(t, err)
	assert.Equal(t, int64(2), counts.ServiceAccountCount)
	assert.Equal(t, int64(1), counts.ChatUserCount)
}

func TestCreateServiceAccount_Refusals(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)
	group, err := models.GetOrCreateDefaultGroup(db)
	require.NoError(t, err)

	for name, dto := range map[string]UserDTO{
		"email":         {Email: "ops@example.com"},
		"password":      {Password: "Sup3r-secret-pw!"},
		"portal":        {ShowPortal: true},
		"chat":          {ShowChat: true},
		"notifications": {NotificationsEnabled: true, IsAdmin: true},
		"idp config":    {AccessToSSOConfig: true, IsAdmin: true},
		"teams":         {Groups: []uint{group.ID}},
	} {
		t.Run(name, func(t *testing.T) {
			dto.Name = "Bot"
			dto.AccountType = models.AccountTypeService
			_, err := s.CreateUser(dto)
			assertServiceAccountRefusal(t, err)
		})
	}

	_, err = s.CreateUser(UserDTO{Name: "Bot", AccountType: "robot"})
	var he helpers.ErrorResponse
	require.True(t, errors.As(err, &he))
	assert.Equal(t, http.StatusUnprocessableEntity, he.StatusCode)

	var n int64
	require.NoError(t, db.Model(&models.User{}).Count(&n).Error)
	assert.Zero(t, n, "no refusal leaves a user behind")
}

func TestUpdateServiceAccount_Invariants(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)
	group, err := models.GetOrCreateDefaultGroup(db)
	require.NoError(t, err)
	svc := createServiceAccount(t, s, "Bot")

	base := func() UserDTO {
		return UserDTO{Email: svc.Email, Name: svc.Name, EmailVerified: true}
	}
	for name, mutate := range map[string]func(*UserDTO){
		"email":        func(d *UserDTO) { d.Email = "ops@example.com" },
		"portal":       func(d *UserDTO) { d.ShowPortal = true },
		"chat":         func(d *UserDTO) { d.ShowChat = true },
		"teams":        func(d *UserDTO) { d.Groups = []uint{group.ID} },
		"account type": func(d *UserDTO) { d.AccountType = "x" },
	} {
		t.Run(name, func(t *testing.T) {
			stored, err := s.GetUserByID(svc.ID, "Groups")
			require.NoError(t, err)
			dto := base()
			mutate(&dto)
			_, err = s.UpdateUser(stored, dto)
			assertServiceAccountRefusal(t, err)
		})
	}

	// A rename is fine.
	stored, err := s.GetUserByID(svc.ID, "Groups")
	require.NoError(t, err)
	dto := base()
	dto.Name = "Renamed bot"
	updated, err := s.UpdateUser(stored, dto)
	require.NoError(t, err)
	assert.Equal(t, "Renamed bot", updated.Name)
	assert.True(t, updated.IsServiceAccount())

	// A person cannot be turned into a service account.
	person, err := s.CreateUser(UserDTO{Email: "p@example.com", Name: "P", Password: "Sup3r-secret-pw!"})
	require.NoError(t, err)
	stored, err = s.GetUserByID(person.ID, "Groups")
	require.NoError(t, err)
	_, err = s.UpdateUser(stored, UserDTO{Email: "p@example.com", Name: "P", AccountType: models.AccountTypeService, Groups: stored.ExtractGroupIDs()})
	assertServiceAccountRefusal(t, err)
}

// A plugin's before-hook cannot change the account type (either way), nor
// give a service account an email, Portal or Chat.
func TestObjectHooks_CannotChangeAccountType(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)
	plugin := &hookPlugin{modify: func(obj map[string]any) {
		if obj["account_type"] == models.AccountTypeService {
			obj["account_type"] = ""
			obj["email"] = "hijack@example.com"
			obj["ShowPortal"] = true
		} else {
			obj["account_type"] = models.AccountTypeService
		}
	}}
	registry := NewHookRegistry()
	require.NoError(t, registry.RegisterHooks(1, "flipper", []*pb.ObjectHookRegistration{{
		ObjectType: string(ObjectTypeUser),
		HookTypes:  []string{string(HookBeforeCreate), string(HookBeforeUpdate)},
	}}))
	manager := &AIStudioPluginManager{loadedPlugins: map[uint]*LoadedAIStudioPlugin{1: {ID: 1, Name: "flipper", GRPCClient: plugin, IsHealthy: true}}}
	s.HookManager = NewHookManager(registry, manager)

	svc := createServiceAccount(t, s, "Bot")
	var stored models.User
	require.NoError(t, db.First(&stored, svc.ID).Error)
	assert.True(t, stored.IsServiceAccount())
	assert.True(t, strings.HasSuffix(stored.Email, "@"+models.ServiceAccountEmailDomain))
	assert.False(t, stored.ShowPortal)

	updated, err := s.UpdateUser(&stored, UserDTO{Email: stored.Email, Name: "Bot 2", EmailVerified: true})
	require.NoError(t, err)
	assert.True(t, updated.IsServiceAccount())
	var again models.User
	require.NoError(t, db.First(&again, svc.ID).Error)
	assert.True(t, again.IsServiceAccount())
	assert.True(t, strings.HasSuffix(again.Email, "@"+models.ServiceAccountEmailDomain))

	person, err := s.CreateUser(UserDTO{Email: "p@example.com", Name: "P", Password: "Sup3r-secret-pw!"})
	require.NoError(t, err)
	var storedPerson models.User
	require.NoError(t, db.First(&storedPerson, person.ID).Error)
	assert.False(t, storedPerson.IsServiceAccount(), "a hook cannot make a person a service account")
}

func TestServiceAccount_PasswordLoginRefused(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)
	svc := createServiceAccount(t, s, "Bot")
	// Even with a password planted (as an older build could), no login.
	hash, err := models.HashPassword("Sup3r-secret-pw!")
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", svc.ID).UpdateColumn("password", hash).Error)

	_, err = s.AuthenticateUser(svc.Email, "Sup3r-secret-pw!")
	assert.ErrorIs(t, err, LoginFailedError)
}

func TestServiceAccount_NoTeamOnAnyPath(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)
	group, err := models.GetOrCreateDefaultGroup(db)
	require.NoError(t, err)
	svc := createServiceAccount(t, s, "Bot")

	assertServiceAccountRefusal(t, s.AddUserToGroup(svc.ID, group.ID))
	assertServiceAccountRefusal(t, s.UpdateGroupUsers(group.ID, []uint{svc.ID}))
	_, err = s.CreateGroup("Ops", []uint{svc.ID}, nil, nil, nil)
	assertServiceAccountRefusal(t, err)
	_, err = s.UpdateGroup(group.ID, group.Name, []uint{svc.ID}, nil, nil, nil)
	assertServiceAccountRefusal(t, err)

	// The model-level writers refuse too (SSO group sync, user saves).
	stored, err := s.GetUserByID(svc.ID)
	require.NoError(t, err)
	assert.ErrorIs(t, stored.UpdateGroupMemberships(db, "1"), models.ErrServiceAccountTeams)
	assert.ErrorIs(t, stored.ReplaceGroupAssociation(db, []models.Group{*group}), models.ErrServiceAccountTeams)
	assert.ErrorIs(t, group.AddUser(db, stored), models.ErrServiceAccountTeams)

	var n int64
	require.NoError(t, db.Table("user_groups").Where("user_id = ?", svc.ID).Count(&n).Error)
	assert.Zero(t, n)
}

func TestServiceAccount_NoNotifications(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)
	svc := createServiceAccount(t, s, "Bot")
	// As if an older build had made it an admin who takes notifications.
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", svc.ID).
		Updates(map[string]interface{}{"is_admin": true, "notifications_enabled": true}).Error)
	admin := &models.User{Email: "admin@example.com", Name: "Admin", IsAdmin: true, NotificationsEnabled: true, EmailVerified: true}
	require.NoError(t, db.Create(admin).Error)

	ns := NewNotificationService(db, "test@example.com", "localhost", 25, "", "", nil)
	require.NoError(t, ns.NotifyWithOptions("fanout-1", "Title", "Body", models.NotifyAdmins, NotifyOptions{SkipEmail: true}))
	require.NoError(t, ns.NotifyWithOptions("direct-1", "Title", "Body", svc.ID, NotifyOptions{SkipEmail: true}))
	require.NoError(t, ns.Send(&models.Notification{UserID: svc.ID, NotificationID: "send-1", Title: "T", Content: "C"}))

	var toService, toAdmin int64
	require.NoError(t, db.Model(&models.Notification{}).Where("user_id = ?", svc.ID).Count(&toService).Error)
	require.NoError(t, db.Model(&models.Notification{}).Where("user_id = ?", admin.ID).Count(&toAdmin).Error)
	assert.Zero(t, toService, "nothing is ever addressed to a service account")
	assert.Equal(t, int64(1), toAdmin)
}

func TestServiceAccount_HostIdentityRefused(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)
	svc := createServiceAccount(t, s, "Bot")

	// Linking by email.
	_, err := s.ProvisionHostUser(HostIdentity{Subject: "host-1", Email: svc.Email, Name: "Bot"})
	assert.ErrorIs(t, err, ErrHostServiceAccount)

	// A subject that resolves to it (set by other means).
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", svc.ID).UpdateColumn("external_subject", "host-2").Error)
	_, err = s.ProvisionHostUser(HostIdentity{Subject: "host-2", Email: svc.Email, Name: "Bot"})
	assert.ErrorIs(t, err, ErrHostServiceAccount)
}
