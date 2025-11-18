package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	// Auto-migrate schema
	err = db.AutoMigrate(
		&models.User{}, &models.Group{}, &models.Catalogue{}, &models.LLM{},
		&models.DataCatalogue{}, &models.ToolCatalogue{}, &models.Datasource{},
		&models.Tool{}, &models.Tag{}, &models.Chat{}, // Removed ChatMessage and ChatInteraction
	)
	if err != nil {
		t.Fatalf("Failed to auto-migrate schema: %v", err)
	}

	// Create a default group (ID 1) that will be used for automatic user assignments
	service := NewService(db)
	defaultGroup, err := service.CreateGroup(models.DefaultGroupName, []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)
	assert.Equal(t, models.DefaultGroupID, defaultGroup.ID) // Ensure it has ID 1

	// Create a superadmin user (ID 1) to avoid issues with user deletion in tests
	_, err = service.CreateUser(UserDTO{
		Email:                "admin@example.com",
		Name:                 "Admin User",
		Password:             "password123",
		IsAdmin:              true,
		ShowChat:             true,
		ShowPortal:           true,
		EmailVerified:        true,
		NotificationsEnabled: true,
		AccessToSSOConfig:    true,
		Groups:               []uint{},
	})
	assert.NoError(t, err)

	return db
}

func TestUserService(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	t.Run("Basic CRUD operations", func(t *testing.T) {
		// Test CreateUser - this will be our regular test user
		user, err := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: false, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: false, AccessToSSOConfig: false, Groups: []uint{}})
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.NotZero(t, user.ID)

		// Test GetUserByID
		fetchedUser, err := service.GetUserByID(user.ID)
		assert.NoError(t, err)
		assert.Equal(t, user.Email, fetchedUser.Email)

		// Test UpdateUser
		updatedUser, err := service.UpdateUser(user, UserDTO{
			Email:                "updated@example.com",
			Name:                 "Updated User",
			IsAdmin:              false,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: false,
			AccessToSSOConfig:    false,
			Groups:               []uint{},
		})
		assert.NoError(t, err)
		assert.Equal(t, "updated@example.com", updatedUser.Email)
		assert.Equal(t, "Updated User", updatedUser.Name)

		// Test AuthenticateUser
		authenticatedUser, err := service.AuthenticateUser("updated@example.com", "password123")
		assert.NoError(t, err)
		assert.NotNil(t, authenticatedUser)

		// Test ListUsers
		params := ListUsersParams{
			PageSize:   10,
			PageNumber: 1,
			All:        true,
			Sort:       "id",
		}
		users, _, _, err := service.ListUsers(params)
		assert.NoError(t, err)
		assert.Len(t, users, 2) // Now expecting 2 users (superadmin + test user)

		// Test SearchUsersByEmailStub
		searchedUsers, err := service.SearchUsersByEmailStub("updat")
		assert.NoError(t, err)
		assert.Len(t, searchedUsers, 1)
		assert.Equal(t, "updated@example.com", searchedUsers[0].Email)

		// Test DeleteUser
		err = service.DeleteUser(user)
		assert.NoError(t, err)

		// Verify user is deleted
		_, err = service.GetUserByID(user.ID)
		assert.Error(t, err)
	})

	t.Run("Email format validation", func(t *testing.T) {
		// Temporarily configure FilterSignupDomains to enable email validation
		appConfig := config.Get()
		originalDomains := appConfig.FilterSignupDomains
		appConfig.FilterSignupDomains = []string{"example.com"}            // Only allow example.com
		defer func() { appConfig.FilterSignupDomains = originalDomains }() // Restore original config

		// Try to create a user with an invalid email format (missing @ symbol)
		_, err := service.CreateUser(UserDTO{
			Email:                "notanemailaddress",
			Name:                 "Invalid Email User",
			Password:             "password123",
			IsAdmin:              false,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: false,
			AccessToSSOConfig:    false,
			Groups:               []uint{},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid email address")

		// Try with invalid domain
		_, err = service.CreateUser(UserDTO{
			Email:                "valid@invaliddomain.com",
			Name:                 "Invalid Domain User",
			Password:             "password123",
			IsAdmin:              false,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: false,
			AccessToSSOConfig:    false,
			Groups:               []uint{},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "email domain 'invaliddomain.com' is not permitted")

		// Create a user with valid email and domain
		user, err := service.CreateUser(UserDTO{
			Email:                "validemail@example.com",
			Name:                 "Valid Email User",
			Password:             "password123",
			IsAdmin:              false,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: false,
			AccessToSSOConfig:    false,
			Groups:               []uint{},
		})
		assert.NoError(t, err)

		// Try updating a user with an invalid email format
		_, err = service.UpdateUser(user, UserDTO{
			Email:                "invalidemailformat", // No @ symbol
			Name:                 user.Name,
			IsAdmin:              false,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: false,
			AccessToSSOConfig:    false,
			Groups:               []uint{},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid email address")

		// Try updating with invalid domain
		_, err = service.UpdateUser(user, UserDTO{
			Email:                "newemail@invaliddomain.com",
			Name:                 user.Name,
			IsAdmin:              false,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: false,
			AccessToSSOConfig:    false,
			Groups:               []uint{},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "email domain 'invaliddomain.com' is not permitted")

		// Clean up
		service.DeleteUser(user)
	})

	t.Run("Notifications validation", func(t *testing.T) {
		// Admin user with notifications enabled (should succeed)
		adminUser, err := service.CreateUser(UserDTO{
			Email:                "admin_notif@example.com",
			Name:                 "Admin with Notifications",
			Password:             "password123",
			IsAdmin:              true,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: true,
			AccessToSSOConfig:    false,
			Groups:               []uint{},
		})
		assert.NoError(t, err)
		assert.True(t, adminUser.NotificationsEnabled)

		// Non-admin user with notifications enabled (should fail)
		_, err = service.CreateUser(UserDTO{
			Email:                "regular_notif@example.com",
			Name:                 "Regular User with Notifications",
			Password:             "password123",
			IsAdmin:              false,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: true,
			AccessToSSOConfig:    false,
			Groups:               []uint{},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "notifications can only be enabled for admin users")

		// Regular user (no notifications) should succeed
		regularUser, err := service.CreateUser(UserDTO{
			Email:                "regular_no_notif@example.com",
			Name:                 "Regular User without Notifications",
			Password:             "password123",
			IsAdmin:              false,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: false,
			AccessToSSOConfig:    false,
			Groups:               []uint{},
		})
		assert.NoError(t, err)
		assert.False(t, regularUser.NotificationsEnabled)

		// Test updating a regular user to have notifications (should fail)
		_, err = service.UpdateUser(regularUser, UserDTO{
			Email:                regularUser.Email,
			Name:                 regularUser.Name,
			IsAdmin:              false,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: true,
			AccessToSSOConfig:    false,
			Groups:               []uint{},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "notifications can only be enabled for admin users")

		// Clean up
		service.DeleteUser(adminUser)
		service.DeleteUser(regularUser)
	})

	t.Run("SSO config access validation", func(t *testing.T) {
		// Admin user with SSO config access (should succeed)
		adminUser, err := service.CreateUser(UserDTO{
			Email:                "admin_sso@example.com",
			Name:                 "Admin with SSO Access",
			Password:             "password123",
			IsAdmin:              true,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: false,
			AccessToSSOConfig:    true,
			Groups:               []uint{},
		})
		assert.NoError(t, err)
		assert.True(t, adminUser.AccessToSSOConfig)

		// Non-admin user with SSO config access (should fail)
		_, err = service.CreateUser(UserDTO{
			Email:                "regular_sso@example.com",
			Name:                 "Regular User with SSO Access",
			Password:             "password123",
			IsAdmin:              false,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: false,
			AccessToSSOConfig:    true,
			Groups:               []uint{},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "access to IdP configuration can only be enabled for admin users")

		// Clean up
		service.DeleteUser(adminUser)
	})

	t.Run("Super admin deletion prevention", func(t *testing.T) {
		// Try to delete the super admin user (ID 1)
		superAdmin, err := service.GetUserByID(1)
		assert.NoError(t, err)

		// Ensure the user is actually a super admin
		assert.True(t, superAdmin.IsAdmin)

		// Attempt to delete should fail
		err = service.DeleteUser(superAdmin)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "super admin user cannot be deleted")

		// Verify super admin still exists
		_, err = service.GetUserByID(1)
		assert.NoError(t, err)
	})

	t.Run("Group validation", func(t *testing.T) {
		// First, create some groups for testing
		group1, err := service.CreateGroup("Group Validation Test 1", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)
		group2, err := service.CreateGroup("Group Validation Test 2", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)

		// Test creating a user with valid groups
		user, err := service.CreateUser(UserDTO{
			Email:                "group_test@example.com",
			Name:                 "Group Test User",
			Password:             "password123",
			IsAdmin:              false,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: false,
			AccessToSSOConfig:    false,
			Groups:               []uint{group1.ID, group2.ID},
		})
		assert.NoError(t, err)
		assert.NotNil(t, user)

		// Get the user with groups preloaded to verify group assignments
		userWithGroups, err := service.GetUserByID(user.ID, "Groups")
		assert.NoError(t, err)
		assert.Len(t, userWithGroups.Groups, 3) // 2 assigned groups + default group

		// Extract group IDs for easier assertion
		var groupIDs []uint
		for _, g := range userWithGroups.Groups {
			groupIDs = append(groupIDs, g.ID)
		}
		assert.Contains(t, groupIDs, group1.ID)
		assert.Contains(t, groupIDs, group2.ID)
		assert.Contains(t, groupIDs, models.DefaultGroupID)

		// Test creating a user with invalid groups
		_, err = service.CreateUser(UserDTO{
			Email:                "invalid_group@example.com",
			Name:                 "Invalid Group User",
			Password:             "password123",
			IsAdmin:              false,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: false,
			AccessToSSOConfig:    false,
			Groups:               []uint{9999}, // Non-existent group ID
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "groups not found")

		// Test updating a user with valid groups
		updatedUser, err := service.UpdateUser(user, UserDTO{
			Email:                user.Email,
			Name:                 user.Name,
			IsAdmin:              user.IsAdmin,
			ShowChat:             user.ShowChat,
			ShowPortal:           user.ShowPortal,
			EmailVerified:        user.EmailVerified,
			NotificationsEnabled: user.NotificationsEnabled,
			AccessToSSOConfig:    user.AccessToSSOConfig,
			Groups:               []uint{group1.ID}, // Only keep group1
		})
		assert.NoError(t, err)

		// Get the updated user with groups preloaded
		updatedUserWithGroups, err := service.GetUserByID(updatedUser.ID, "Groups")
		assert.NoError(t, err)
		assert.Len(t, updatedUserWithGroups.Groups, 1) // Only 1 group now
		assert.Equal(t, group1.ID, updatedUserWithGroups.Groups[0].ID)

		// Test updating a user with invalid groups
		_, err = service.UpdateUser(user, UserDTO{
			Email:                user.Email,
			Name:                 user.Name,
			IsAdmin:              user.IsAdmin,
			ShowChat:             user.ShowChat,
			ShowPortal:           user.ShowPortal,
			EmailVerified:        user.EmailVerified,
			NotificationsEnabled: user.NotificationsEnabled,
			AccessToSSOConfig:    user.AccessToSSOConfig,
			Groups:               []uint{9999}, // Non-existent group ID
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "groups not found")

		// Clean up
		service.DeleteUser(user)
		service.DeleteGroup(group1.ID)
		service.DeleteGroup(group2.ID)
	})
}

func TestGroupService(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Test CreateGroup with empty associations
	t.Run("CreateGroup with empty associations", func(t *testing.T) {
		group, err := service.CreateGroup("Test Group", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)
		assert.NotNil(t, group)
		assert.NotZero(t, group.ID)
		assert.Equal(t, "Test Group", group.Name)

		// Verify associations are empty
		fetchedGroup, err := service.GetGroupByID(group.ID, "Users", "Catalogues", "DataCatalogues", "ToolCatalogues")
		assert.NoError(t, err)
		assert.Empty(t, fetchedGroup.Users)
		assert.Empty(t, fetchedGroup.Catalogues)
		assert.Empty(t, fetchedGroup.DataCatalogues)
		assert.Empty(t, fetchedGroup.ToolCatalogues)
	})

	// Test CreateGroup with associations
	t.Run("CreateGroup with associations", func(t *testing.T) {
		// Create user, catalogue, data catalogue and tool catalogue
		user1, err := service.CreateUser(UserDTO{Email: "user1@example.com", Name: "User 1", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
		assert.NoError(t, err)
		user2, err := service.CreateUser(UserDTO{Email: "user2@example.com", Name: "User 2", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
		assert.NoError(t, err)

		catalogue, err := service.CreateCatalogue("Test Catalogue")
		assert.NoError(t, err)

		dataCatalogue, err := service.CreateDataCatalogue("Test Data Catalogue", "description", "long description", "icon-name")
		assert.NoError(t, err)

		toolCatalogue, err := service.CreateToolCatalogue("Test Tool Catalogue", "description", "long description", "icon-name")
		assert.NoError(t, err)

		// Create group with associations
		group, err := service.CreateGroup(
			"Associated Group",
			[]uint{user1.ID, user2.ID},
			[]uint{catalogue.ID},
			[]uint{dataCatalogue.ID},
			[]uint{toolCatalogue.ID},
		)
		assert.NoError(t, err)
		assert.NotNil(t, group)
		assert.NotZero(t, group.ID)

		// Verify associations
		fetchedGroup, err := service.GetGroupByID(group.ID, "Users", "Catalogues", "DataCatalogues", "ToolCatalogues")
		assert.NoError(t, err)
		assert.Len(t, fetchedGroup.Users, 2)
		assert.Len(t, fetchedGroup.Catalogues, 1)
		assert.Len(t, fetchedGroup.DataCatalogues, 1)
		assert.Len(t, fetchedGroup.ToolCatalogues, 1)
	})

	// Test GetGroupByID with different preload options
	t.Run("GetGroupByID with preloads", func(t *testing.T) {
		// Create a group with a user
		user, err := service.CreateUser(UserDTO{Email: "preload@example.com", Name: "Preload User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
		assert.NoError(t, err)

		group, err := service.CreateGroup("Preload Group", []uint{user.ID}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)

		// Test with no preloads
		fetchedGroup, err := service.GetGroupByID(group.ID)
		assert.NoError(t, err)
		assert.Equal(t, group.Name, fetchedGroup.Name)
		assert.Empty(t, fetchedGroup.Users) // Users not preloaded

		// Test with Users preload
		fetchedGroupWithUsers, err := service.GetGroupByID(group.ID, "Users")
		assert.NoError(t, err)
		assert.Equal(t, group.Name, fetchedGroupWithUsers.Name)
		assert.Len(t, fetchedGroupWithUsers.Users, 1)
		assert.Equal(t, user.ID, fetchedGroupWithUsers.Users[0].ID)

		// Test with invalid ID
		_, err = service.GetGroupByID(9999)
		assert.Error(t, err)
	})

	// Test UpdateGroup
	t.Run("UpdateGroup", func(t *testing.T) {
		// Create users
		user1, err := service.CreateUser(UserDTO{Email: "update1@example.com", Name: "Update User 1", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
		assert.NoError(t, err)
		user2, err := service.CreateUser(UserDTO{Email: "update2@example.com", Name: "Update User 2", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
		assert.NoError(t, err)
		user3, err := service.CreateUser(UserDTO{Email: "update3@example.com", Name: "Update User 3", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
		assert.NoError(t, err)

		// Create catalogues
		catalogue1, err := service.CreateCatalogue("Update Catalogue 1")
		assert.NoError(t, err)
		catalogue2, err := service.CreateCatalogue("Update Catalogue 2")
		assert.NoError(t, err)

		// Create data catalogues
		dataCatalogue1, err := service.CreateDataCatalogue("Update Data Catalogue 1", "desc1", "long desc1", "icon1")
		assert.NoError(t, err)
		dataCatalogue2, err := service.CreateDataCatalogue("Update Data Catalogue 2", "desc2", "long desc2", "icon2")
		assert.NoError(t, err)

		// Create tool catalogues
		toolCatalogue1, err := service.CreateToolCatalogue("Update Tool Catalogue 1", "desc1", "long desc1", "icon1")
		assert.NoError(t, err)
		toolCatalogue2, err := service.CreateToolCatalogue("Update Tool Catalogue 2", "desc2", "long desc2", "icon2")
		assert.NoError(t, err)

		// Create initial group with some associations
		group, err := service.CreateGroup(
			"Initial Group",
			[]uint{user1.ID},
			[]uint{catalogue1.ID},
			[]uint{dataCatalogue1.ID},
			[]uint{toolCatalogue1.ID},
		)
		assert.NoError(t, err)

		// Update the group with different name and associations
		updatedGroup, err := service.UpdateGroup(
			group.ID,
			"Updated Group",
			[]uint{user2.ID, user3.ID},                   // Replace user1 with user2 and user3
			[]uint{catalogue1.ID, catalogue2.ID},         // Add catalogue2
			[]uint{dataCatalogue2.ID},                    // Replace dataCatalogue1 with dataCatalogue2
			[]uint{toolCatalogue1.ID, toolCatalogue2.ID}, // Add toolCatalogue2
		)
		assert.NoError(t, err)
		assert.Equal(t, "Updated Group", updatedGroup.Name)

		// Verify updated associations
		fetchedGroup, err := service.GetGroupByID(group.ID, "Users", "Catalogues", "DataCatalogues", "ToolCatalogues")
		assert.NoError(t, err)

		// Verify name is updated
		assert.Equal(t, "Updated Group", fetchedGroup.Name)

		// Verify users (user1 is replaced with user2 and user3)
		assert.Len(t, fetchedGroup.Users, 2)
		userIDs := []uint{fetchedGroup.Users[0].ID, fetchedGroup.Users[1].ID}
		assert.Contains(t, userIDs, user2.ID)
		assert.Contains(t, userIDs, user3.ID)
		assert.NotContains(t, userIDs, user1.ID)

		// Verify catalogues (catalogue2 is added)
		assert.Len(t, fetchedGroup.Catalogues, 2)
		catalogueIDs := []uint{fetchedGroup.Catalogues[0].ID, fetchedGroup.Catalogues[1].ID}
		assert.Contains(t, catalogueIDs, catalogue1.ID)
		assert.Contains(t, catalogueIDs, catalogue2.ID)

		// Verify data catalogues (dataCatalogue1 is replaced with dataCatalogue2)
		assert.Len(t, fetchedGroup.DataCatalogues, 1)
		assert.Equal(t, dataCatalogue2.ID, fetchedGroup.DataCatalogues[0].ID)

		// Verify tool catalogues (toolCatalogue2 is added)
		assert.Len(t, fetchedGroup.ToolCatalogues, 2)
		toolCatalogueIDs := []uint{fetchedGroup.ToolCatalogues[0].ID, fetchedGroup.ToolCatalogues[1].ID}
		assert.Contains(t, toolCatalogueIDs, toolCatalogue1.ID)
		assert.Contains(t, toolCatalogueIDs, toolCatalogue2.ID)

		// Test updating non-existent group
		_, err = service.UpdateGroup(9999, "Non-existent", []uint{}, []uint{}, []uint{}, []uint{})
		assert.Error(t, err)
	})

	// Test Error Cases
	t.Run("GroupService_ErrorCases", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewService(db)

		// Test GetGroupByID with non-existent ID
		_, err := service.GetGroupByID(9999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Test UpdateGroup with non-existent ID
		_, err = service.UpdateGroup(9999, "Non-existent", []uint{}, []uint{}, []uint{}, []uint{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Test DeleteGroup with non-existent ID
		err = service.DeleteGroup(9999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Test AddUserToGroup with non-existent user
		err = service.AddUserToGroup(9999, 1) // Assuming group ID 1 exists
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Create a group to test errors with
		group, err := service.CreateGroup("Error Test Group", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)

		// Test AddUserToGroup with non-existent user
		err = service.AddUserToGroup(9999, group.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Test RemoveUserFromGroup with non-existent user
		err = service.RemoveUserFromGroup(9999, group.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Test RemoveUserFromGroup with non-existent group
		user, err := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: false, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: false, AccessToSSOConfig: false, Groups: []uint{}})
		assert.NoError(t, err)

		err = service.RemoveUserFromGroup(user.ID, 9999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Test GetGroupUsers with non-existent group
		users, totalCount, totalPages, err := service.GetGroupUsers(9999, 10, 1, false)
		assert.NoError(t, err)
		assert.Empty(t, users)
		assert.Equal(t, int64(0), totalCount)
		assert.Equal(t, 0, totalPages)

		// Additional association error tests

		// Test AddCatalogueToGroup with non-existent catalogue
		err = service.AddCatalogueToGroup(9999, group.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Test RemoveCatalogueFromGroup with non-existent catalogue
		err = service.RemoveCatalogueFromGroup(9999, group.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Test RemoveCatalogueFromGroup with non-existent group
		catalogue, err := service.CreateCatalogue("Error Test Catalogue")
		assert.NoError(t, err)

		err = service.RemoveCatalogueFromGroup(catalogue.ID, 9999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Test GetGroupCatalogues with non-existent group
		_, err = service.GetGroupCatalogues(9999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Test Data Catalogue association errors
		err = service.AddDataCatalogueToGroup(9999, group.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		err = service.RemoveDataCatalogueFromGroup(9999, group.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Create a data catalogue for testing
		dataCatalogue, err := service.CreateDataCatalogue("Error Data Catalogue", "short", "long", "icon")
		assert.NoError(t, err)

		err = service.RemoveDataCatalogueFromGroup(dataCatalogue.ID, 9999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		_, err = service.GetGroupDataCatalogues(9999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Test Tool Catalogue association errors
		err = service.AddToolCatalogueToGroup(9999, group.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		err = service.RemoveToolCatalogueFromGroup(9999, group.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Create a tool catalogue for testing
		toolCatalogue, err := service.CreateToolCatalogue("Error Tool Catalogue", "short", "long", "icon")
		assert.NoError(t, err)

		err = service.RemoveToolCatalogueFromGroup(toolCatalogue.ID, 9999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		_, _, _, err = service.GetGroupToolCatalogues(9999, 10, 1, true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")

		// Clean up
		err = service.DeleteGroup(group.ID)
		assert.NoError(t, err)
		err = service.DeleteUser(user)
		assert.NoError(t, err)
		err = service.DeleteCatalogue(catalogue.ID)
		assert.NoError(t, err)
		err = service.DeleteDataCatalogue(dataCatalogue.ID)
		assert.NoError(t, err)
		err = service.DeleteToolCatalogue(toolCatalogue.ID)
		assert.NoError(t, err)
	})

	// Test DeleteGroup
	t.Run("DeleteGroup", func(t *testing.T) {
		// Create a group with associations
		user, err := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
		assert.NoError(t, err)

		catalogue, err := service.CreateCatalogue("Delete Catalogue")
		assert.NoError(t, err)

		group, err := service.CreateGroup("Delete Group", []uint{user.ID}, []uint{catalogue.ID}, []uint{}, []uint{})
		assert.NoError(t, err)

		// Verify group exists with associations
		fetchedGroup, err := service.GetGroupByID(group.ID, "Users", "Catalogues")
		assert.NoError(t, err)
		assert.Len(t, fetchedGroup.Users, 1)
		assert.Len(t, fetchedGroup.Catalogues, 1)

		// Delete the group
		err = service.DeleteGroup(group.ID)
		assert.NoError(t, err)

		// Verify group is deleted
		_, err = service.GetGroupByID(group.ID)
		assert.Error(t, err)

		// But user and catalogue should still exist
		fetchedUser, err := service.GetUserByID(user.ID)
		assert.NoError(t, err)
		assert.Equal(t, user.ID, fetchedUser.ID)

		fetchedCatalogue, err := service.GetCatalogueByID(catalogue.ID)
		assert.NoError(t, err)
		assert.Equal(t, catalogue.ID, fetchedCatalogue.ID)

		// Test deleting non-existent group
		err = service.DeleteGroup(9999)
		assert.Error(t, err)
	})

	// Test other group functionality
	t.Run("Group search and listing", func(t *testing.T) {
		// Create an isolated test DB for this specific test to avoid interference from other tests
		testDB := setupTestDB(t)
		testService := NewService(testDB)

		// Create groups with different names
		group1, err := testService.CreateGroup("Alpha Group", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)
		group2, err := testService.CreateGroup("Beta Group", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)
		group3, err := testService.CreateGroup("Gamma Group", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)

		// Test GetAllGroups
		groups, count, pages, err := testService.GetAllGroups(10, 1, true, "id")
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(groups), 3) // We created 3 groups in this test
		assert.GreaterOrEqual(t, count, int64(3))
		assert.GreaterOrEqual(t, pages, 1)

		// Test pagination
		groupsPage1, _, _, err := testService.GetAllGroups(2, 1, false, "id")
		assert.NoError(t, err)
		assert.Len(t, groupsPage1, 2)

		groupsPage2, _, _, err := testService.GetAllGroups(2, 2, false, "id")
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(groupsPage2), 1)

		// Test SearchGroupsByNameStub
		// Test search for a specific group
		alphaGroups, err := testService.SearchGroupsByNameStub("Alpha")
		assert.NoError(t, err)
		assert.Len(t, alphaGroups, 1)
		assert.Equal(t, "Alpha Group", alphaGroups[0].Name)

		// Test search functionality works (not testing exact counts)
		betaGroups, err := testService.SearchGroupsByNameStub("Beta")
		assert.NoError(t, err)
		assert.NotEmpty(t, betaGroups)
		assert.Equal(t, "Beta Group", betaGroups[0].Name)

		gammaGroups, err := testService.SearchGroupsByNameStub("Gamma")
		assert.NoError(t, err)
		assert.NotEmpty(t, gammaGroups)
		assert.Equal(t, "Gamma Group", gammaGroups[0].Name)

		// Clean up
		_ = testService.DeleteGroup(group1.ID)
		_ = testService.DeleteGroup(group2.ID)
		_ = testService.DeleteGroup(group3.ID)
		_ = testService.DeleteGroup(group1.ID)
		_ = testService.DeleteGroup(group2.ID)
		_ = testService.DeleteGroup(group3.ID)
	})
}

func TestLLMService(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Test CreateLLM
	llm, err := service.CreateLLM("TestLLM", "test-api-key", "https://api.test.com", 75, "Short desc", "Long desc", "https://logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, llm)
	assert.NotZero(t, llm.ID)
	assert.Equal(t, "TestLLM", llm.Name)
	assert.Equal(t, "test-api-key", llm.APIKey)
	assert.Equal(t, "https://api.test.com", llm.APIEndpoint)
	assert.Equal(t, 75, llm.PrivacyScore)
	assert.Equal(t, "Short desc", llm.ShortDescription)
	assert.Equal(t, "Long desc", llm.LongDescription)
	assert.Equal(t, "https://logo.com", llm.LogoURL)

	// Test GetLLMByID
	fetchedLLM, err := service.GetLLMByID(llm.ID)
	assert.NoError(t, err)
	assert.Equal(t, llm.Name, fetchedLLM.Name)
	assert.Equal(t, llm.APIKey, fetchedLLM.APIKey)
	assert.Equal(t, llm.APIEndpoint, fetchedLLM.APIEndpoint)
	assert.Equal(t, llm.PrivacyScore, fetchedLLM.PrivacyScore)
	assert.Equal(t, llm.ShortDescription, fetchedLLM.ShortDescription)
	assert.Equal(t, llm.LongDescription, fetchedLLM.LongDescription)
	assert.Equal(t, llm.LogoURL, fetchedLLM.LogoURL)

	// Test UpdateLLM
	updatedLLM, err := service.UpdateLLM(llm.ID, "UpdatedLLM", "updated-api-key", "https://updated-api.test.com", 80,
		"Updated short", "Updated long", "https://updated-logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil, "")
	assert.NoError(t, err)
	assert.Equal(t, "UpdatedLLM", updatedLLM.Name)
	assert.Equal(t, "updated-api-key", updatedLLM.APIKey)
	assert.Equal(t, "https://updated-api.test.com", updatedLLM.APIEndpoint)
	assert.Equal(t, 80, updatedLLM.PrivacyScore)
	assert.Equal(t, "Updated short", updatedLLM.ShortDescription)
	assert.Equal(t, "Updated long", updatedLLM.LongDescription)
	assert.Equal(t, "https://updated-logo.com", updatedLLM.LogoURL)

	// Test GetLLMByName
	fetchedLLMByName, err := service.GetLLMByName("UpdatedLLM")
	assert.NoError(t, err)
	assert.Equal(t, updatedLLM.ID, fetchedLLMByName.ID)
	assert.Equal(t, updatedLLM.Name, fetchedLLMByName.Name)

	// Test GetAllLLMs
	allLLMs, _, _, err := service.GetAllLLMs(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, allLLMs, 1)
	assert.Equal(t, updatedLLM.ID, (allLLMs)[0].ID)

	// Test GetLLMsByNameStub
	stubLLMs, err := service.GetLLMsByNameStub("Updated")
	assert.NoError(t, err)
	assert.Len(t, stubLLMs, 1)
	assert.Equal(t, updatedLLM.ID, (stubLLMs)[0].ID)

	// Test DeleteLLM
	err = service.DeleteLLM(llm.ID)
	assert.NoError(t, err)

	// Verify LLM is deleted
	_, err = service.GetLLMByID(llm.ID)
	assert.Error(t, err)

	// Test creating multiple LLMs and searching
	llm1, _ := service.CreateLLM("GPT-3", "key1", "https://api1.com", 70, "GPT-3 short", "GPT-3 long", "https://gpt3-logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	llm2, _ := service.CreateLLM("GPT-4", "key2", "https://api2.com", 85, "GPT-4 short", "GPT-4 long", "https://gpt4-logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	service.CreateLLM("BERT", "key3", "https://api3.com", 60, "BERT short", "BERT long", "https://bert-logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)

	allLLMs, _, _, err = service.GetAllLLMs(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, allLLMs, 3)

	gptLLMs, err := service.GetLLMsByNameStub("GPT")
	assert.NoError(t, err)
	assert.Len(t, gptLLMs, 2)
	assert.Contains(t, []uint{llm1.ID, llm2.ID}, (gptLLMs)[0].ID)
	assert.Contains(t, []uint{llm1.ID, llm2.ID}, (gptLLMs)[1].ID)
}

func createTestLLMs(t *testing.T, db *gorm.DB) {
	llms := []models.LLM{
		{Name: "LLM1", APIKey: "key1", APIEndpoint: "https://api1.com", PrivacyScore: 50, ShortDescription: "Short 1", LongDescription: "Long 1", LogoURL: "https://logo1.com"},
		{Name: "LLM2", APIKey: "key2", APIEndpoint: "https://api2.com", PrivacyScore: 75, ShortDescription: "Short 2", LongDescription: "Long 2", LogoURL: "https://logo2.com"},
		{Name: "LLM3", APIKey: "key3", APIEndpoint: "https://api3.com", PrivacyScore: 90, ShortDescription: "Short 3", LongDescription: "Long 3", LogoURL: "https://logo3.com"},
		{Name: "LLM4", APIKey: "key4", APIEndpoint: "https://api4.com", PrivacyScore: 30, ShortDescription: "Short 4", LongDescription: "Long 4", LogoURL: "https://logo4.com"},
		{Name: "LLM5", APIKey: "key5", APIEndpoint: "https://api5.com", PrivacyScore: 60, ShortDescription: "Short 5", LongDescription: "Long 5", LogoURL: "https://logo5.com"},
	}

	for _, llm := range llms {
		err := db.Create(&llm).Error
		assert.NoError(t, err)
	}
}

func TestService_GetLLMsByMaxPrivacyScore(t *testing.T) {
	db := setupTestDB(t)
	createTestLLMs(t, db)
	service := NewService(db)

	testCases := []struct {
		maxScore      int
		expectedCount int
		expectedNames []string
	}{
		{100, 5, []string{"LLM1", "LLM2", "LLM3", "LLM4", "LLM5"}},
		{80, 4, []string{"LLM1", "LLM2", "LLM4", "LLM5"}},
		{60, 3, []string{"LLM1", "LLM4", "LLM5"}},
		{40, 1, []string{"LLM4"}},
		{20, 0, []string{}},
	}

	for _, tc := range testCases {
		llms, err := service.GetLLMsByMaxPrivacyScore(tc.maxScore)
		assert.NoError(t, err)
		assert.Len(t, llms, tc.expectedCount)

		var names []string
		for _, llm := range llms {
			names = append(names, llm.Name)
			assert.LessOrEqual(t, llm.PrivacyScore, tc.maxScore)
		}
		assert.ElementsMatch(t, tc.expectedNames, names)
	}
}

func TestService_GetLLMsByMinPrivacyScore(t *testing.T) {
	db := setupTestDB(t)
	createTestLLMs(t, db)
	service := NewService(db)

	testCases := []struct {
		minScore      int
		expectedCount int
		expectedNames []string
	}{
		{0, 5, []string{"LLM1", "LLM2", "LLM3", "LLM4", "LLM5"}},
		{40, 4, []string{"LLM1", "LLM2", "LLM3", "LLM5"}},
		{70, 2, []string{"LLM2", "LLM3"}},
		{80, 1, []string{"LLM3"}},
		{95, 0, []string{}},
	}

	for _, tc := range testCases {
		llms, err := service.GetLLMsByMinPrivacyScore(tc.minScore)
		assert.NoError(t, err)
		assert.Len(t, llms, tc.expectedCount)

		var names []string
		for _, llm := range llms {
			names = append(names, llm.Name)
			assert.GreaterOrEqual(t, llm.PrivacyScore, tc.minScore)
		}
		assert.ElementsMatch(t, tc.expectedNames, names)
	}
}

func TestService_GetLLMsByPrivacyScoreRange(t *testing.T) {
	db := setupTestDB(t)
	createTestLLMs(t, db)
	service := NewService(db)

	testCases := []struct {
		minScore      int
		maxScore      int
		expectedCount int
		expectedNames []string
	}{
		{0, 100, 5, []string{"LLM1", "LLM2", "LLM3", "LLM4", "LLM5"}},
		{40, 80, 3, []string{"LLM1", "LLM2", "LLM5"}},
		{70, 90, 2, []string{"LLM2", "LLM3"}},
		{30, 50, 2, []string{"LLM1", "LLM4"}},
		{95, 100, 0, []string{}},
	}

	for _, tc := range testCases {
		llms, err := service.GetLLMsByPrivacyScoreRange(tc.minScore, tc.maxScore)
		assert.NoError(t, err)
		assert.Len(t, llms, tc.expectedCount)

		var names []string
		for _, llm := range llms {
			names = append(names, llm.Name)
			assert.GreaterOrEqual(t, llm.PrivacyScore, tc.minScore)
			assert.LessOrEqual(t, llm.PrivacyScore, tc.maxScore)
		}
		assert.ElementsMatch(t, tc.expectedNames, names)
	}

	// Test invalid range
	llms, err := service.GetLLMsByPrivacyScoreRange(80, 70)
	assert.NoError(t, err)
	assert.Len(t, llms, 0)
}

func TestCatalogueService(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Test CreateCatalogue
	catalogue, err := service.CreateCatalogue("Test Catalogue")
	assert.NoError(t, err)
	assert.NotNil(t, catalogue)
	assert.NotZero(t, catalogue.ID)

	// Test GetCatalogueByID
	fetchedCatalogue, err := service.GetCatalogueByID(catalogue.ID)
	assert.NoError(t, err)
	assert.Equal(t, catalogue.Name, fetchedCatalogue.Name)

	// Test UpdateCatalogue
	updatedCatalogue, err := service.UpdateCatalogue(catalogue.ID, "Updated Catalogue")
	assert.NoError(t, err)
	assert.Equal(t, "Updated Catalogue", updatedCatalogue.Name)

	// Test GetAllCatalogues
	catalogues, _, _, err := service.GetAllCatalogues(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, catalogues, 1)

	// Test SearchCataloguesByNameStub
	searchedCatalogues, err := service.SearchCataloguesByNameStub("Update")
	assert.NoError(t, err)
	assert.Len(t, searchedCatalogues, 1)
	assert.Equal(t, "Updated Catalogue", searchedCatalogues[0].Name)

	// Test AddLLMToCatalogue
	llm, err := service.CreateLLM("TestLLM", "test-api-key", "https://api.test.com", 70, "Short desc", "Long desc", "https://logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	assert.NoError(t, err)

	err = service.AddLLMToCatalogue(llm.ID, catalogue.ID)
	assert.NoError(t, err)

	// Test GetCatalogueLLMs
	catalogueLLMs, err := service.GetCatalogueLLMs(catalogue.ID)
	assert.NoError(t, err)
	assert.Len(t, catalogueLLMs, 1)
	assert.Equal(t, llm.ID, catalogueLLMs[0].ID)

	// Test RemoveLLMFromCatalogue
	err = service.RemoveLLMFromCatalogue(llm.ID, catalogue.ID)
	assert.NoError(t, err)

	catalogueLLMs, err = service.GetCatalogueLLMs(catalogue.ID)
	assert.NoError(t, err)
	assert.Len(t, catalogueLLMs, 0)

	// Test DeleteCatalogue
	err = service.DeleteCatalogue(catalogue.ID)
	assert.NoError(t, err)

	// Verify catalogue is deleted
	_, err = service.GetCatalogueByID(catalogue.ID)
	assert.Error(t, err)
}

func TestCatalogueService_MultipleCatalogues(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create multiple catalogues
	catalogue1, _ := service.CreateCatalogue("AI Models")
	catalogue2, _ := service.CreateCatalogue("Machine Learning")
	catalogue3, _ := service.CreateCatalogue("Natural Language Processing")

	// Test GetAllCatalogues
	allCatalogues, _, _, err := service.GetAllCatalogues(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, allCatalogues, 3)

	// Test SearchCataloguesByNameStub
	aiCatalogues, err := service.SearchCataloguesByNameStub("AI")
	assert.NoError(t, err)
	assert.Len(t, aiCatalogues, 1)
	assert.Equal(t, catalogue1.ID, aiCatalogues[0].ID)

	mlCatalogues, err := service.SearchCataloguesByNameStub("Machine")
	assert.NoError(t, err)
	assert.Len(t, mlCatalogues, 1)
	assert.Equal(t, catalogue2.ID, mlCatalogues[0].ID)

	// Test adding multiple LLMs to a catalogue
	llm1, _ := service.CreateLLM("GPT-3", "key1", "https://api1.com", 80, "GPT-3 short", "GPT-3 long", "https://gpt3-logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	llm2, _ := service.CreateLLM("BERT", "key2", "https://api2.com", 70, "BERT short", "BERT long", "https://bert-logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)

	err = service.AddLLMToCatalogue(llm1.ID, catalogue3.ID)
	assert.NoError(t, err)
	err = service.AddLLMToCatalogue(llm2.ID, catalogue3.ID)
	assert.NoError(t, err)

	catalogueLLMs, err := service.GetCatalogueLLMs(catalogue3.ID)
	assert.NoError(t, err)
	assert.Len(t, catalogueLLMs, 2)
	assert.ElementsMatch(t, []uint{llm1.ID, llm2.ID}, []uint{catalogueLLMs[0].ID, catalogueLLMs[1].ID})

	// Test removing one LLM from the catalogue
	err = service.RemoveLLMFromCatalogue(llm1.ID, catalogue3.ID)
	assert.NoError(t, err)

	catalogueLLMs, err = service.GetCatalogueLLMs(catalogue3.ID)
	assert.NoError(t, err)
	assert.Len(t, catalogueLLMs, 1)
	assert.Equal(t, llm2.ID, catalogueLLMs[0].ID)
}

func TestUserAccessibleCatalogues(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create a user
	user, err := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
	assert.NoError(t, err)

	// Create groups
	group1, err := service.CreateGroup("Group 1", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)
	group2, err := service.CreateGroup("Group 2", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)

	// Add user to groups
	err = service.AddUserToGroup(user.ID, group1.ID)
	assert.NoError(t, err)
	err = service.AddUserToGroup(user.ID, group2.ID)
	assert.NoError(t, err)

	// Create catalogues
	catalogue1, err := service.CreateCatalogue("Catalogue 1")
	assert.NoError(t, err)
	catalogue2, err := service.CreateCatalogue("Catalogue 2")
	assert.NoError(t, err)
	_, err = service.CreateCatalogue("Catalogue 3")
	assert.NoError(t, err)

	// Add catalogues to groups
	err = service.AddCatalogueToGroup(catalogue1.ID, group1.ID)
	assert.NoError(t, err)
	err = service.AddCatalogueToGroup(catalogue2.ID, group2.ID)
	assert.NoError(t, err)

	// Test GetUserAccessibleCatalogues
	accessibleCatalogues, err := service.GetUserAccessibleCatalogues(user.ID)
	assert.NoError(t, err)
	assert.Len(t, accessibleCatalogues, 2)
	assert.ElementsMatch(t, []string{"Catalogue 1", "Catalogue 2"}, []string{accessibleCatalogues[0].Name, accessibleCatalogues[1].Name})

	// Remove user from a group
	err = service.RemoveUserFromGroup(user.ID, group2.ID)
	assert.NoError(t, err)

	// Test GetUserAccessibleCatalogues after removal
	accessibleCatalogues, err = service.GetUserAccessibleCatalogues(user.ID)
	assert.NoError(t, err)
	assert.Len(t, accessibleCatalogues, 1)
	assert.Equal(t, "Catalogue 1", accessibleCatalogues[0].Name)
}

func TestGroupCatalogueAssociation(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create a group
	group, err := service.CreateGroup("Test Group", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)

	// Create catalogues
	catalogue1, err := service.CreateCatalogue("Catalogue 1")
	assert.NoError(t, err)
	catalogue2, err := service.CreateCatalogue("Catalogue 2")
	assert.NoError(t, err)

	// Test AddCatalogueToGroup
	err = service.AddCatalogueToGroup(catalogue1.ID, group.ID)
	assert.NoError(t, err)
	err = service.AddCatalogueToGroup(catalogue2.ID, group.ID)
	assert.NoError(t, err)

	// Test GetGroupCatalogues
	groupCatalogues, err := service.GetGroupCatalogues(group.ID)
	assert.NoError(t, err)
	assert.Len(t, groupCatalogues, 2)
	assert.ElementsMatch(t, []string{"Catalogue 1", "Catalogue 2"}, []string{groupCatalogues[0].Name, groupCatalogues[1].Name})

	// Test RemoveCatalogueFromGroup
	err = service.RemoveCatalogueFromGroup(catalogue1.ID, group.ID)
	assert.NoError(t, err)

	// Test GetGroupCatalogues after removal
	groupCatalogues, err = service.GetGroupCatalogues(group.ID)
	assert.NoError(t, err)
	assert.Len(t, groupCatalogues, 1)
	assert.Equal(t, "Catalogue 2", groupCatalogues[0].Name)
}

func TestUpdateGroupCatalogues(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	t.Run("Success case - full update", func(t *testing.T) {
		catalogue1, err := service.CreateCatalogue("Catalog 1")
		assert.NoError(t, err)
		catalogue2, err := service.CreateCatalogue("Catalog 2")
		assert.NoError(t, err)

		dataCatalogue1, err := service.CreateDataCatalogue("Data Catalog 1", "desc1", "long desc1", "icon1")
		assert.NoError(t, err)
		dataCatalogue2, err := service.CreateDataCatalogue("Data Catalog 2", "desc2", "long desc2", "icon2")
		assert.NoError(t, err)

		toolCatalogue1, err := service.CreateToolCatalogue("Tool Catalog 1", "desc1", "long desc1", "icon1")
		assert.NoError(t, err)
		toolCatalogue2, err := service.CreateToolCatalogue("Tool Catalog 2", "desc2", "long desc2", "icon2")
		assert.NoError(t, err)

		group, err := service.CreateGroup(
			"Test Group",
			[]uint{}, // No users
			[]uint{catalogue1.ID},
			[]uint{dataCatalogue1.ID},
			[]uint{toolCatalogue1.ID},
		)
		assert.NoError(t, err)

		fetchedGroup, err := service.GetGroupByID(group.ID, "Catalogues", "DataCatalogues", "ToolCatalogues")
		assert.NoError(t, err)
		assert.Len(t, fetchedGroup.Catalogues, 1)
		assert.Equal(t, catalogue1.ID, fetchedGroup.Catalogues[0].ID)
		assert.Len(t, fetchedGroup.DataCatalogues, 1)
		assert.Equal(t, dataCatalogue1.ID, fetchedGroup.DataCatalogues[0].ID)
		assert.Len(t, fetchedGroup.ToolCatalogues, 1)
		assert.Equal(t, toolCatalogue1.ID, fetchedGroup.ToolCatalogues[0].ID)

		catalogue3, err := service.CreateCatalogue("Catalog 3")
		assert.NoError(t, err)
		dataCatalogue3, err := service.CreateDataCatalogue("Data Catalog 3", "desc3", "long desc3", "icon3")
		assert.NoError(t, err)
		toolCatalogue3, err := service.CreateToolCatalogue("Tool Catalog 3", "desc3", "long desc3", "icon3")
		assert.NoError(t, err)

		err = service.UpdateGroupCatalogues(
			group.ID,
			[]uint{catalogue2.ID, catalogue3.ID}, // Replace catalogue1 with catalogue2 and catalogue3
			[]uint{dataCatalogue2.ID, dataCatalogue3.ID}, // Replace dataCatalogue1 with dataCatalogue2 and dataCatalogue3
			[]uint{toolCatalogue2.ID, toolCatalogue3.ID}, // Replace toolCatalogue1 with toolCatalogue2 and toolCatalogue3
		)
		assert.NoError(t, err)

		// Verify the updates
		updatedGroup, err := service.GetGroupByID(group.ID, "Catalogues", "DataCatalogues", "ToolCatalogues")
		assert.NoError(t, err)

		// Verify Catalogues are updated
		assert.Len(t, updatedGroup.Catalogues, 2)
		catalogueIDs := []uint{updatedGroup.Catalogues[0].ID, updatedGroup.Catalogues[1].ID}
		assert.Contains(t, catalogueIDs, catalogue2.ID)
		assert.Contains(t, catalogueIDs, catalogue3.ID)
		assert.NotContains(t, catalogueIDs, catalogue1.ID)

		// Verify DataCatalogues are updated
		assert.Len(t, updatedGroup.DataCatalogues, 2)
		dataCatalogueIDs := []uint{updatedGroup.DataCatalogues[0].ID, updatedGroup.DataCatalogues[1].ID}
		assert.Contains(t, dataCatalogueIDs, dataCatalogue2.ID)
		assert.Contains(t, dataCatalogueIDs, dataCatalogue3.ID)
		assert.NotContains(t, dataCatalogueIDs, dataCatalogue1.ID)

		// Verify ToolCatalogues are updated
		assert.Len(t, updatedGroup.ToolCatalogues, 2)
		toolCatalogueIDs := []uint{updatedGroup.ToolCatalogues[0].ID, updatedGroup.ToolCatalogues[1].ID}
		assert.Contains(t, toolCatalogueIDs, toolCatalogue2.ID)
		assert.Contains(t, toolCatalogueIDs, toolCatalogue3.ID)
		assert.NotContains(t, toolCatalogueIDs, toolCatalogue1.ID)
	})

	t.Run("Partial updates", func(t *testing.T) {
		catalogue1, err := service.CreateCatalogue("Partial Catalog 1")
		assert.NoError(t, err)
		catalogue2, err := service.CreateCatalogue("Partial Catalog 2")
		assert.NoError(t, err)

		dataCatalogue1, err := service.CreateDataCatalogue("Partial Data Catalog 1", "desc1", "long desc1", "icon1")
		assert.NoError(t, err)

		toolCatalogue1, err := service.CreateToolCatalogue("Partial Tool Catalog 1", "desc1", "long desc1", "icon1")
		assert.NoError(t, err)
		toolCatalogue2, err := service.CreateToolCatalogue("Partial Tool Catalog 2", "desc2", "long desc2", "icon2")
		assert.NoError(t, err)

		group, err := service.CreateGroup(
			"Partial Test Group",
			[]uint{}, // No users
			[]uint{catalogue1.ID},
			[]uint{dataCatalogue1.ID},
			[]uint{toolCatalogue1.ID},
		)
		assert.NoError(t, err)

		// Update only the Catalogues and ToolCatalogues, leave DataCatalogues unchanged
		err = service.UpdateGroupCatalogues(
			group.ID,
			[]uint{catalogue2.ID},     // Replace catalogue1 with catalogue2
			[]uint{dataCatalogue1.ID}, // Keep the same dataCatalogue1
			[]uint{toolCatalogue2.ID}, // Replace toolCatalogue1 with toolCatalogue2
		)
		assert.NoError(t, err)

		// Verify the updates
		updatedGroup, err := service.GetGroupByID(group.ID, "Catalogues", "DataCatalogues", "ToolCatalogues")
		assert.NoError(t, err)

		// Verify Catalogues are updated
		assert.Len(t, updatedGroup.Catalogues, 1)
		assert.Equal(t, catalogue2.ID, updatedGroup.Catalogues[0].ID)

		// Verify DataCatalogues are unchanged
		assert.Len(t, updatedGroup.DataCatalogues, 1)
		assert.Equal(t, dataCatalogue1.ID, updatedGroup.DataCatalogues[0].ID)

		// Verify ToolCatalogues are updated
		assert.Len(t, updatedGroup.ToolCatalogues, 1)
		assert.Equal(t, toolCatalogue2.ID, updatedGroup.ToolCatalogues[0].ID)
	})

	t.Run("No updates needed", func(t *testing.T) {
		catalogue1, err := service.CreateCatalogue("No Update Catalog 1")
		assert.NoError(t, err)

		dataCatalogue1, err := service.CreateDataCatalogue("No Update Data Catalog 1", "desc1", "long desc1", "icon1")
		assert.NoError(t, err)

		toolCatalogue1, err := service.CreateToolCatalogue("No Update Tool Catalog 1", "desc1", "long desc1", "icon1")
		assert.NoError(t, err)

		group, err := service.CreateGroup(
			"No Update Test Group",
			[]uint{}, // No users
			[]uint{catalogue1.ID},
			[]uint{dataCatalogue1.ID},
			[]uint{toolCatalogue1.ID},
		)
		assert.NoError(t, err)

		// Call UpdateGroupCatalogues with the same IDs
		err = service.UpdateGroupCatalogues(
			group.ID,
			[]uint{catalogue1.ID},     // Same catalogue1
			[]uint{dataCatalogue1.ID}, // Same dataCatalogue1
			[]uint{toolCatalogue1.ID}, // Same toolCatalogue1
		)
		assert.NoError(t, err)

		// Verify nothing changed
		updatedGroup, err := service.GetGroupByID(group.ID, "Catalogues", "DataCatalogues", "ToolCatalogues")
		assert.NoError(t, err)

		// Verify Catalogues are unchanged
		assert.Len(t, updatedGroup.Catalogues, 1)
		assert.Equal(t, catalogue1.ID, updatedGroup.Catalogues[0].ID)

		// Verify DataCatalogues are unchanged
		assert.Len(t, updatedGroup.DataCatalogues, 1)
		assert.Equal(t, dataCatalogue1.ID, updatedGroup.DataCatalogues[0].ID)

		// Verify ToolCatalogues are unchanged
		assert.Len(t, updatedGroup.ToolCatalogues, 1)
		assert.Equal(t, toolCatalogue1.ID, updatedGroup.ToolCatalogues[0].ID)
	})

	t.Run("Non-existent group", func(t *testing.T) {
		// Call UpdateGroupCatalogues with a non-existent group ID
		err := service.UpdateGroupCatalogues(
			9999, // Non-existent ID
			[]uint{1, 2},
			[]uint{3, 4},
			[]uint{5, 6},
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")
	})
}

func TestSmartAPIKeyUpdateLogic(t *testing.T) {
	// Set required environment variable
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")

	db := setupTestDB(t)
	service := NewService(db)

	t.Run("LLM_APIKey_SmartUpdate", func(t *testing.T) {
		// Create an LLM with an initial API key
		llm, err := service.CreateLLM("Test LLM", "initial-api-key", "https://api.test.com", 75,
			"Short desc", "Long desc", "logo.png", models.OPENAI, true, nil,
			"gpt-4", []string{}, nil, nil)
		assert.NoError(t, err)
		assert.Equal(t, "initial-api-key", llm.APIKey)

		// Test 1: Update with [redacted] should preserve existing key
		updatedLLM1, err := service.UpdateLLM(llm.ID, "Test LLM", "[redacted]", "https://api.test.com", 75,
			"Short desc", "Long desc", "logo.png", models.OPENAI, true, nil,
			"gpt-4", []string{}, nil, nil, "")
		assert.NoError(t, err)
		assert.Equal(t, "initial-api-key", updatedLLM1.APIKey, "API key should be preserved when [redacted] is sent")

		// Test 2: Update with empty string should clear the key
		updatedLLM2, err := service.UpdateLLM(llm.ID, "Test LLM", "", "https://api.test.com", 75,
			"Short desc", "Long desc", "logo.png", models.OPENAI, true, nil,
			"gpt-4", []string{}, nil, nil, "")
		assert.NoError(t, err)
		assert.Equal(t, "", updatedLLM2.APIKey, "API key should be cleared when empty string is sent")

		// Test 3: Update with new key should update the key
		updatedLLM3, err := service.UpdateLLM(llm.ID, "Test LLM", "new-api-key", "https://api.test.com", 75,
			"Short desc", "Long desc", "logo.png", models.OPENAI, true, nil,
			"gpt-4", []string{}, nil, nil, "")
		assert.NoError(t, err)
		assert.Equal(t, "new-api-key", updatedLLM3.APIKey, "API key should be updated when new value is sent")
	})

	t.Run("Datasource_APIKeys_SmartUpdate", func(t *testing.T) {
		// Create a user first
		user, err := service.CreateUser(UserDTO{
			Email:                "test@example.com",
			Name:                 "Test User",
			Password:             "password123",
			IsAdmin:              true,
			ShowChat:             true,
			ShowPortal:           true,
			EmailVerified:        true,
			NotificationsEnabled: true,
			AccessToSSOConfig:    true,
			Groups:               []uint{},
		})
		assert.NoError(t, err)

		// Create a datasource with initial API keys
		datasource, err := service.CreateDatasource("Test Datasource", "Short desc", "Long desc", "icon.png",
			"https://example.com", 75, user.ID, []string{}, "conn_string", "source_type",
			"initial-db-key", "db1", "embed_vendor", "embed_url", "initial-embed-key", "embed_model", true)
		assert.NoError(t, err)
		assert.Equal(t, "initial-db-key", datasource.DBConnAPIKey)
		assert.Equal(t, "initial-embed-key", datasource.EmbedAPIKey)

		// Test 1: Update with [redacted] should preserve existing keys
		updatedDS1, err := service.UpdateDatasource(datasource.ID, "Test Datasource", "Short desc", "Long desc", "icon.png",
			"https://example.com", 75, "conn_string", "source_type", "[redacted]", "db1",
			"embed_vendor", "embed_url", "[redacted]", "embed_model", true, []string{}, user.ID)
		assert.NoError(t, err)
		assert.Equal(t, "initial-db-key", updatedDS1.DBConnAPIKey, "DB API key should be preserved when [redacted] is sent")
		assert.Equal(t, "initial-embed-key", updatedDS1.EmbedAPIKey, "Embed API key should be preserved when [redacted] is sent")

		// Test 2: Update with empty strings should clear the keys
		updatedDS2, err := service.UpdateDatasource(datasource.ID, "Test Datasource", "Short desc", "Long desc", "icon.png",
			"https://example.com", 75, "conn_string", "source_type", "", "db1",
			"embed_vendor", "embed_url", "", "embed_model", true, []string{}, user.ID)
		assert.NoError(t, err)
		assert.Equal(t, "", updatedDS2.DBConnAPIKey, "DB API key should be cleared when empty string is sent")
		assert.Equal(t, "", updatedDS2.EmbedAPIKey, "Embed API key should be cleared when empty string is sent")

		// Test 3: Update with new keys should update the keys
		updatedDS3, err := service.UpdateDatasource(datasource.ID, "Test Datasource", "Short desc", "Long desc", "icon.png",
			"https://example.com", 75, "conn_string", "source_type", "new-db-key", "db1",
			"embed_vendor", "embed_url", "new-embed-key", "embed_model", true, []string{}, user.ID)
		assert.NoError(t, err)
		assert.Equal(t, "new-db-key", updatedDS3.DBConnAPIKey, "DB API key should be updated when new value is sent")
		assert.Equal(t, "new-embed-key", updatedDS3.EmbedAPIKey, "Embed API key should be updated when new value is sent")
	})
}
