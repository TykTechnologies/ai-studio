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
		ForSection:   AdminSection,
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
				ForSection:   AdminSection,
				EmailAddress: "test@example.com",
			},
			expectError: false,
		},
		{
			name: "Valid user section",
			request: &NonceTokenRequest{
				ForSection:   UserSection,
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
	user, err := ssoService.createUserWithTx(db, "test@example.com", "Test User", "password", "sso-key", true)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "test@example.com", user.Email)
	assert.Equal(t, "Test User", user.Name)
	assert.Equal(t, "sso-key", user.SSOKey)
	assert.True(t, user.IsAdmin)
	assert.True(t, user.ShowChat)
	assert.True(t, user.ShowPortal)
	assert.True(t, user.EmailVerified)
	assert.True(t, user.NotificationsEnabled)
}

func TestAddUserToGroupWithTx(t *testing.T) {
	// Setup
	db := setupSSOTestDB(t)
	ssoService := &SSOService{db: db}

	// Create a test group
	group := &models.Group{
		Name: "Test Group",
	}
	err := db.Create(group).Error
	require.NoError(t, err)

	// Create a test user
	user := &models.User{
		Email: "test@example.com",
		Name:  "Test User",
	}
	err = db.Create(user).Error
	require.NoError(t, err)

	t.Run("Empty group ID", func(t *testing.T) {
		// Test directly with the DB
		err := ssoService.addUserToGroupWithTx(db, user.ID, "")

		// Assertions
		assert.NoError(t, err)
	})

	t.Run("Invalid group ID", func(t *testing.T) {
		// Test directly with the DB
		err := ssoService.addUserToGroupWithTx(db, user.ID, "invalid")

		// Assertions
		assert.Error(t, err)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, errResp.StatusCode)
	})

	t.Run("Valid group ID", func(t *testing.T) {
		// Get the default group
		defaultGroup := &models.Group{}
		err := db.First(defaultGroup, 1).Error
		require.NoError(t, err)

		// Test directly with the DB
		err = ssoService.addUserToGroupWithTx(db, user.ID, "1")

		// Assertions
		assert.NoError(t, err)

		// Verify user is in group using direct SQL query
		var count int64
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, defaultGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})
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
		user, err := ssoService.HandleSSO("test1@example.com", "Test User 1", "1", false, AdminSection)

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
		user, err := ssoService.HandleSSO("test2@example.com", "Test User 2", "2", false, AdminSection)

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
		user, err := ssoService.HandleSSO("existing@example.com", "Updated Name", "2", false, AdminSection)

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
		user, err := ssoService.HandleSSO("unregistered@example.com", "Unregistered User", "1", true, AdminSection)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, user)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, http.StatusForbidden, errResp.StatusCode)
	})
}

func TestCreateSSOUser(t *testing.T) {
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
		user, err := ssoService.CreateSSOUser("test1@example.com", "Test User 1", "password", "sso-key-1", "1")

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "test1@example.com", user.Email)
		assert.Equal(t, "Test User 1", user.Name)
		assert.Equal(t, "sso-key-1", user.SSOKey)

		// Verify user is in default group only using direct SQL query
		var count int64
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", user.ID, defaultGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)

		// Verify user is not in any other group
		err = db.Table("user_groups").Where("user_id = ? AND group_id != ?", user.ID, defaultGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})

	t.Run("New user with additional group", func(t *testing.T) {
		// Test
		user, err := ssoService.CreateSSOUser("test2@example.com", "Test User 2", "password", "sso-key-2", "2")

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "test2@example.com", user.Email)
		assert.Equal(t, "Test User 2", user.Name)
		assert.Equal(t, "sso-key-2", user.SSOKey)

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
		// Test with an existing SSO key
		user, err := ssoService.CreateSSOUser("updated@example.com", "Updated User", "password", "sso-key-1", "2")

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "test1@example.com", user.Email) // Email should not be updated
		assert.Equal(t, "Test User 1", user.Name)        // Name should not be updated
		assert.Equal(t, "sso-key-1", user.SSOKey)
	})
}

func TestUpdateSSOUser(t *testing.T) {
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

	// Create a test user
	user := &models.User{
		Email:  "test@example.com",
		Name:   "Test User",
		SSOKey: "sso-key",
	}
	err = db.Create(user).Error
	require.NoError(t, err)

	// Add user to default group
	err = db.Model(defaultGroup).Association("Users").Append(user)
	require.NoError(t, err)

	t.Run("Update email and group", func(t *testing.T) {
		// Test
		updatedUser, err := ssoService.UpdateSSOUser("sso-key", "updated@example.com", "", "2")

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, updatedUser)
		assert.Equal(t, "updated@example.com", updatedUser.Email)
		assert.Equal(t, "Test User", updatedUser.Name) // Name should not be updated
		assert.Equal(t, "sso-key", updatedUser.SSOKey)

		// Verify user is in the additional group
		var count int64
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", updatedUser.ID, additionalGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)

		// The current implementation doesn't remove the user from the default group
		// So we expect the user to be in both groups
		err = db.Table("user_groups").Where("user_id = ? AND group_id = ?", updatedUser.ID, defaultGroup.ID).Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("Update password", func(t *testing.T) {
		// Test
		updatedUser, err := ssoService.UpdateSSOUser("sso-key", "updated@example.com", "new-password", "2")

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, updatedUser)
		assert.Equal(t, "updated@example.com", updatedUser.Email)
		assert.Equal(t, "Test User", updatedUser.Name) // Name should not be updated
		assert.Equal(t, "sso-key", updatedUser.SSOKey)

		// Verify password was updated (can't check directly, but we can check it's not empty)
		assert.NotEmpty(t, updatedUser.Password)
	})

	t.Run("Non-existent user", func(t *testing.T) {
		// Test
		updatedUser, err := ssoService.UpdateSSOUser("non-existent-key", "updated@example.com", "", "2")

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, updatedUser)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, http.StatusNotFound, errResp.StatusCode)
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

	t.Run("Notify admin creation", func(t *testing.T) {
		// Clear previous notifications
		notificationSvc.ClearNotifications()

		// Test
		ssoService.notifyUserCreation(user, true)

		// Get notifications
		notifications := notificationSvc.GetNotifications()
		if assert.Equal(t, 1, len(notifications), "Expected 1 notification") {
			assert.Contains(t, notifications[0].NotificationID, "new_admin_2_")
			assert.Equal(t, "New Admin Created via SSO", notifications[0].Title)
			assert.Equal(t, uint(1), notifications[0].UserID) // Should notify super-admin (ID 1)
		}
	})

	t.Run("Notify regular user creation", func(t *testing.T) {
		// Clear previous notifications
		notificationSvc.ClearNotifications()

		// Test
		ssoService.notifyUserCreation(user, false)

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
		ssoService.notifyUserCreation(user, true)

		// Get notifications (should be empty)
		notifications := notificationSvc.GetNotifications()
		assert.Equal(t, 0, len(notifications))
	})
}

func TestGetUserBySSOKey(t *testing.T) {
	// Setup
	db := setupSSOTestDB(t)
	ssoService := &SSOService{db: db}

	// Create a test user
	user := &models.User{
		Email:  "test@example.com",
		Name:   "Test User",
		SSOKey: "sso-key",
	}
	err := db.Create(user).Error
	require.NoError(t, err)

	t.Run("Existing user", func(t *testing.T) {
		// Test
		developer, err := ssoService.GetUserBySSOKey("sso-key")

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, developer)
		assert.Equal(t, "test@example.com", developer.Email)
		assert.Equal(t, "sso-key", developer.SSOKey)
	})

	t.Run("Non-existent user", func(t *testing.T) {
		// Test
		developer, err := ssoService.GetUserBySSOKey("non-existent-key")

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, developer)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, http.StatusNotFound, errResp.StatusCode)
	})
}
