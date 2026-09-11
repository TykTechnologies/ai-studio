package services

import (
	"net/http"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSSOTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Initialize models - this will create all necessary tables and relationships
	err = models.InitModels(db)
	assert.NoError(t, err)

	// Create a default group
	defaultGroup := &models.Group{
		Name: "Default Group",
	}
	err = db.Create(defaultGroup).Error
	assert.NoError(t, err)
	assert.Equal(t, uint(1), defaultGroup.ID) // Default group should have ID 1

	return db
}

func TestNewSSOService(t *testing.T) {
	// Setup
	config := &Config{
		APISecret: "test-secret",
		LogLevel:  "info",
	}
	router := gin.Default()
	db := setupSSOTestDB(t)
	var notificationSvc *NotificationService = nil // Use nil for the test

	// Test
	ssoService := NewSSOService(config, router, db, notificationSvc)

	// Assertions
	assert.NotNil(t, ssoService)
	assert.Equal(t, config, ssoService.config)
	assert.Equal(t, router, ssoService.router)
	assert.Equal(t, db, ssoService.db)
	assert.Nil(t, ssoService.notificationSvc)
}

func TestGenerateAndResolveNonce(t *testing.T) {
	// Setup
	db := setupSSOTestDB(t)
	ssoService := &SSOService{
		db: db,
		InternalTIB: &InternalTIB{
			kvStore: models.NewGormKVStore(db),
		},
	}

	// Test data
	request := NonceTokenRequest{
		ForSection:   DashboardSection,
		EmailAddress: "test@example.com",
		GroupID:      "1",
		DisplayName:  "Test User",
	}

	// Test GenerateNonce
	t.Run("Generate nonce token", func(t *testing.T) {
		nonceToken, err := ssoService.GenerateNonce(request)
		assert.NoError(t, err)
		assert.NotNil(t, nonceToken)
		assert.Len(t, *nonceToken, NonceLength)
	})

	// Test ResolveNonce
	t.Run("Resolve nonce token without consuming", func(t *testing.T) {
		// First generate a token
		nonceToken, err := ssoService.GenerateNonce(request)
		assert.NoError(t, err)
		assert.NotNil(t, nonceToken)

		// Resolve without consuming
		tokenData, err := ssoService.ResolveNonce(*nonceToken, false)
		assert.NoError(t, err)
		assert.NotNil(t, tokenData)
		assert.Equal(t, request.ForSection, tokenData.ForSection)
		assert.Equal(t, request.EmailAddress, tokenData.EmailAddress)
		assert.Equal(t, request.GroupID, tokenData.GroupID)
		assert.Equal(t, request.DisplayName, tokenData.DisplayName)

		// Should be able to resolve again since we didn't consume
		tokenData2, err := ssoService.ResolveNonce(*nonceToken, false)
		assert.NoError(t, err)
		assert.NotNil(t, tokenData2)
	})

	t.Run("Resolve and consume nonce token", func(t *testing.T) {
		// First generate a token
		nonceToken, err := ssoService.GenerateNonce(request)
		assert.NoError(t, err)
		assert.NotNil(t, nonceToken)

		// Resolve and consume
		tokenData, err := ssoService.ResolveNonce(*nonceToken, true)
		assert.NoError(t, err)
		assert.NotNil(t, tokenData)
		assert.Equal(t, request.ForSection, tokenData.ForSection)
		assert.Equal(t, request.EmailAddress, tokenData.EmailAddress)
		assert.Equal(t, request.GroupID, tokenData.GroupID)
		assert.Equal(t, request.DisplayName, tokenData.DisplayName)

		// Should not be able to resolve again since we consumed it
		_, err = ssoService.ResolveNonce(*nonceToken, false)
		assert.Error(t, err)
		assert.IsType(t, helpers.ErrorResponse{}, err)
		assert.Equal(t, http.StatusNotFound, err.(helpers.ErrorResponse).StatusCode)
	})

	t.Run("Resolve expired token", func(t *testing.T) {
		// Generate a token with an expired ExpiresAt
		request.ExpiresAt = time.Now().Add(-1 * time.Hour)
		nonceToken := helpers.GenerateRandomString(NonceLength)
		err := ssoService.InternalTIB.kvStore.SetKey(nonceToken, "", request)
		assert.NoError(t, err)

		// Try to resolve expired token
		_, err = ssoService.ResolveNonce(nonceToken, false)
		assert.Error(t, err)
		assert.IsType(t, helpers.ErrorResponse{}, err)
		assert.Equal(t, http.StatusBadRequest, err.(helpers.ErrorResponse).StatusCode)
	})
}

func TestValidateNonceRequest(t *testing.T) {
	// Setup
	db := setupSSOTestDB(t)
	ssoService := &SSOService{db: db}

	tests := []struct {
		name        string
		request     *NonceTokenRequest
		expectError bool
		errorType   string
	}{
		{
			name: "Valid admin section",
			request: &NonceTokenRequest{
				ForSection:   DashboardSection,
				EmailAddress: "test@example.com",
			},
			expectError: false,
		},
		{
			name: "Valid user section",
			request: &NonceTokenRequest{
				ForSection:   DashboardSection,
				EmailAddress: "test@example.com",
			},
			expectError: false,
		},
		{
			name: "Invalid section",
			request: &NonceTokenRequest{
				ForSection:   "invalid-section",
				EmailAddress: "test@example.com",
			},
			expectError: true,
			errorType:   "Bad Request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ssoService.ValidateNonceRequest(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != "" {
					errResp, ok := err.(helpers.ErrorResponse)
					assert.True(t, ok)
					assert.Equal(t, tt.errorType, errResp.Title)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateUserWithTx(t *testing.T) {
	// Setup
	db := setupSSOTestDB(t)
	ssoService := &SSOService{db: db}

	// Test directly with the DB
	user, err := ssoService.createUserWithTx(db, "test@example.com", "Test User", nil)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "test@example.com", user.Email)
	assert.Equal(t, "Test User", user.Name)
	assert.False(t, user.IsAdmin)
	// No profile: same defaults as an admin-created user.
	assert.True(t, user.ShowChat)
	assert.True(t, user.ShowPortal)
	assert.True(t, user.EmailVerified)
	assert.False(t, user.NotificationsEnabled)

	t.Run("profile provisioning defaults are applied", func(t *testing.T) {
		profile := &models.Profile{NewUserShowPortal: true, NewUserShowChat: false}
		user, err := ssoService.createUserWithTx(db, "portal-only@example.com", "Portal Only", profile)
		require.NoError(t, err)
		assert.True(t, user.ShowPortal)
		assert.False(t, user.ShowChat)

		var stored models.User
		require.NoError(t, db.First(&stored, user.ID).Error)
		assert.True(t, stored.ShowPortal)
		assert.False(t, stored.ShowChat, "an explicit false must survive the insert")
	})
}

func TestProvisioningProfile(t *testing.T) {
	db := setupSSOTestDB(t)
	require.NoError(t, models.MigrateProfiles(db))
	ssoService := &SSOService{db: db}

	profile := models.NewProfile()
	profile.ProfileID = "okta"
	profile.Name = "Okta"
	profile.NewUserShowChat = false
	require.NoError(t, profile.Create(db))

	assert.Nil(t, ssoService.provisioningProfile(db, ""), "empty ID means no profile")
	assert.Nil(t, ssoService.provisioningProfile(db, "does-not-exist"), "unknown ID falls back to defaults")

	found := ssoService.provisioningProfile(db, "okta")
	require.NotNil(t, found)
	showPortal, showChat := found.ProvisioningDefaults()
	assert.True(t, showPortal)
	assert.False(t, showChat)
}

func TestHandleSSO(t *testing.T) {
	// Setup
	db := setupSSOTestDB(t)
	ssoService := &SSOService{db: db}

	// Get the default group
	defaultGroup := &models.Group{}
	err := db.First(defaultGroup, 1).Error
	require.NoError(t, err)
	assert.Equal(t, uint(1), defaultGroup.ID) // Default group should have ID 1

	// Create additional group
	additionalGroup := &models.Group{
		Name: "Additional Group",
	}
	err = db.Create(additionalGroup).Error
	require.NoError(t, err)
	assert.Equal(t, uint(2), additionalGroup.ID)

	t.Run("New user with default group only", func(t *testing.T) {
		// Test
		user, err := ssoService.HandleSSO(&NonceTokenRequest{EmailAddress: "test1@example.com", DisplayName: "Test User 1", GroupID: "1"})

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "test1@example.com", user.Email)
		assert.Equal(t, "Test User 1", user.Name)

		// Debug: Print user ID and default group ID
		t.Logf("User ID: %d, Default Group ID: %d", user.ID, defaultGroup.ID)

		// Debug: Check all user_groups entries
		var userGroups []struct {
			UserID  uint
			GroupID uint
		}
		err = db.Table("user_groups").Find(&userGroups).Error
		assert.NoError(t, err)
		for _, ug := range userGroups {
			t.Logf("User Group Entry: UserID=%d, GroupID=%d", ug.UserID, ug.GroupID)
		}

		// Verify user is in default group only using direct SQL query
		var count int64
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, defaultGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		t.Logf("Count of user in default group: %d", count)
		assert.Equal(t, int64(1), count)

		// Verify user is not in any other group
		err = db.Table("user_groups").Where("user_id = ? AND group_id != ?", user.ID, defaultGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		t.Logf("Count of user in other groups: %d", count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("New user with additional group", func(t *testing.T) {
		// Test
		user, err := ssoService.HandleSSO(&NonceTokenRequest{EmailAddress: "test2@example.com", DisplayName: "Test User 2", GroupID: "2"})

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "test2@example.com", user.Email)
		assert.Equal(t, "Test User 2", user.Name)

		// Verify user is in default group
		var count int64
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, defaultGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)

		// Verify user is in additional group
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, additionalGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("Existing user", func(t *testing.T) {
		// Create an existing user
		existingUser := &models.User{
			Email: "existing@example.com",
			Name:  "Existing User",
		}
		err := db.Create(existingUser).Error
		require.NoError(t, err)

		// Test
		user, err := ssoService.HandleSSO(&NonceTokenRequest{EmailAddress: "existing@example.com", DisplayName: "Updated Name", GroupID: "2"})

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "existing@example.com", user.Email)
		assert.Equal(t, existingUser.ID, user.ID)

		// Verify user is in default group
		var count int64
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, defaultGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)

		// Verify user is in additional group
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, additionalGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("SSO only for registered users", func(t *testing.T) {
		// Test
		user, err := ssoService.HandleSSO(&NonceTokenRequest{EmailAddress: "unregistered@example.com", DisplayName: "Unregistered User", GroupID: "1", SSOOnlyForRegisteredUsers: true})

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, user)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, http.StatusForbidden, errResp.StatusCode)
	})

	t.Run("New user with multiple groups", func(t *testing.T) {
		// Create another group
		thirdGroup := &models.Group{
			Name: "Third Group",
		}
		err = db.Create(thirdGroup).Error
		require.NoError(t, err)
		assert.Equal(t, uint(3), thirdGroup.ID)

		// Test with multiple group IDs
		user, err := ssoService.HandleSSO(&NonceTokenRequest{EmailAddress: "multi-group@example.com", DisplayName: "Multi Group User", GroupsIDs: []string{"2", "3"}})

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "multi-group@example.com", user.Email)
		assert.Equal(t, "Multi Group User", user.Name)

		// Verify user is in default group
		var count int64
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, defaultGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)

		// Verify user is in second group
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, additionalGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)

		// Verify user is in third group
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, thirdGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("Existing user with name update and email verification", func(t *testing.T) {
		// Create an existing user with EmailVerified = false
		existingUserUnverified := &models.User{
			Email:         "unverified@example.com",
			Name:          "Unverified User",
			EmailVerified: false,
		}
		err := db.Create(existingUserUnverified).Error
		require.NoError(t, err)

		// Test
		user, err := ssoService.HandleSSO(&NonceTokenRequest{EmailAddress: "unverified@example.com", DisplayName: "Updated Unverified User", GroupID: "2"})

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "unverified@example.com", user.Email)
		assert.Equal(t, "Updated Unverified User", user.Name)
		assert.Equal(t, existingUserUnverified.ID, user.ID)
		assert.True(t, user.EmailVerified, "User should be marked as email verified after SSO")

		// Verify in database that EmailVerified was updated
		var updatedUser models.User
		err = db.First(&updatedUser, existingUserUnverified.ID).Error
		assert.NoError(t, err)
		assert.True(t, updatedUser.EmailVerified, "EmailVerified should be true in the database")
		assert.Equal(t, "Updated Unverified User", updatedUser.Name, "Name should be updated in the database")

		// Verify user is in default group
		var count int64
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, defaultGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)

		// Verify user is in additional group
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, additionalGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("Existing user with no changes needed", func(t *testing.T) {
		// Create an existing user with EmailVerified = true and same name that will be passed
		existingUserVerified := &models.User{
			Email:         "already-verified@example.com",
			Name:          "Already Verified User",
			EmailVerified: true,
		}
		err := db.Create(existingUserVerified).Error
		require.NoError(t, err)

		// Test with same name
		user, err := ssoService.HandleSSO(&NonceTokenRequest{EmailAddress: "already-verified@example.com", DisplayName: "Already Verified User", GroupID: "2"})

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "already-verified@example.com", user.Email)
		assert.Equal(t, "Already Verified User", user.Name)
		assert.Equal(t, existingUserVerified.ID, user.ID)
		assert.True(t, user.EmailVerified, "User should remain email verified")

		// Verify user is in default group
		var count int64
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, defaultGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)

		// Verify user is in additional group
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, additionalGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("New user takes the profile's provisioning defaults", func(t *testing.T) {
		require.NoError(t, models.MigrateProfiles(db))
		profile := models.NewProfile()
		profile.ProfileID = "chat-only"
		profile.Name = "Chat only"
		profile.NewUserShowPortal = false
		profile.NewUserShowChat = true
		require.NoError(t, profile.Create(db))

		user, err := ssoService.HandleSSO(&NonceTokenRequest{
			EmailAddress: "chat-only@example.com",
			DisplayName:  "Chat Only",
			GroupID:      "2",
			ProfileID:    "chat-only",
		})
		require.NoError(t, err)
		assert.False(t, user.ShowPortal)
		assert.True(t, user.ShowChat)

		// Team membership still follows the claim mapping, not the profile flags.
		var count int64
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, additionalGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("Unknown profile falls back to showing everything", func(t *testing.T) {
		user, err := ssoService.HandleSSO(&NonceTokenRequest{
			EmailAddress: "orphan@example.com",
			DisplayName:  "Orphan",
			ProfileID:    "deleted-profile",
		})
		require.NoError(t, err)
		assert.True(t, user.ShowPortal)
		assert.True(t, user.ShowChat)
	})

	t.Run("Existing user keeps admin-set flags on later logins", func(t *testing.T) {
		require.NoError(t, models.MigrateProfiles(db))
		profile := models.NewProfile()
		profile.ProfileID = "hide-all"
		profile.Name = "Hide all"
		profile.NewUserShowPortal = false
		profile.NewUserShowChat = false
		require.NoError(t, profile.Create(db))

		existing := &models.User{Email: "keep-flags@example.com", Name: "Keep Flags", ShowPortal: true, ShowChat: false}
		require.NoError(t, db.Create(existing).Error)

		user, err := ssoService.HandleSSO(&NonceTokenRequest{
			EmailAddress: "keep-flags@example.com",
			DisplayName:  "Keep Flags",
			ProfileID:    "hide-all",
		})
		require.NoError(t, err)
		assert.Equal(t, existing.ID, user.ID)
		assert.True(t, user.ShowPortal, "profile defaults must not rewrite an existing user")
		assert.False(t, user.ShowChat)
	})
}

func TestNotifyUserCreation(t *testing.T) {
	// Setup
	db := setupSSOTestDB(t)

	// Create admin user (ID 1)
	adminUser := &models.User{
		Email:                "admin@example.com",
		Name:                 "Admin User",
		IsAdmin:              true,
		NotificationsEnabled: true,
	}
	err := db.Create(adminUser).Error
	require.NoError(t, err)
	require.Equal(t, uint(1), adminUser.ID) // Ensure admin has ID 1

	notificationSvc := NewNotificationService(db, "test@example.com", "localhost", 25, "", "", nil)
	ssoService := &SSOService{
		db:              db,
		notificationSvc: notificationSvc,
	}

	// Create test user
	user := &models.User{
		Email: "test@example.com",
		Name:  "Test User",
		ID:    2,
	}
	err = db.Create(user).Error
	require.NoError(t, err)

	t.Run("Notify user creation", func(t *testing.T) {
		// Clear previous notifications
		notificationSvc.ClearNotifications()

		// Test
		ssoService.notifyUserCreation(user)

		// Get notifications
		notifications := notificationSvc.GetNotifications()
		if assert.Equal(t, 1, len(notifications), "Expected 1 notification") {
			assert.Contains(t, notifications[0].NotificationID, "new_user_sso_2_")
			assert.Equal(t, "New User Created via SSO", notifications[0].Title)
		}
	})

	t.Run("No notification when service is nil", func(t *testing.T) {
		// Clear previous notifications
		notificationSvc.ClearNotifications()

		// Set notification service to nil
		ssoService.notificationSvc = nil

		// Test
		ssoService.notifyUserCreation(user)

		// Get notifications (should be empty)
		notifications := notificationSvc.GetNotifications()
		assert.Equal(t, 0, len(notifications))
	})
}
