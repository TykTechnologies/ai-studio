package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUserTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestCreateUserWithAccessToSSOConfig(t *testing.T) {
	db := setupUserTestDB(t)
	service := NewService(db)

	// Test 1: Admin user with AccessToSSOConfig = true (should succeed)
	user, err := service.CreateUser(
		"admin@example.com",
		"Admin User",
		"password123",
		true, // isAdmin
		true, // showChat
		true, // showPortal
		true, // emailVerified
		true, // notificationsEnabled
		true, // accessToSSOConfig
	)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.True(t, user.AccessToSSOConfig)

	// Test 2: Non-admin user with AccessToSSOConfig = true (should fail)
	_, err = service.CreateUser(
		"nonadmin@example.com",
		"Non-Admin User",
		"password123",
		false, // isAdmin
		true,  // showChat
		true,  // showPortal
		true,  // emailVerified
		false, // notificationsEnabled
		true,  // accessToSSOConfig
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access to IdP configuration can only be enabled for admin users")

	// Test 3: Non-admin user with AccessToSSOConfig = false (should succeed)
	user, err = service.CreateUser(
		"regular@example.com",
		"Regular User",
		"password123",
		false, // isAdmin
		true,  // showChat
		true,  // showPortal
		true,  // emailVerified
		false, // notificationsEnabled
		false, // accessToSSOConfig
	)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.False(t, user.AccessToSSOConfig)
}

func TestUpdateUserWithAccessToSSOConfig(t *testing.T) {
	db := setupUserTestDB(t)

	service := NewService(db)

	// Create an admin user
	adminUser, err := service.CreateUser(
		"admin@example.com",
		"Admin User",
		"password123",
		true,  // isAdmin
		true,  // showChat
		true,  // showPortal
		true,  // emailVerified
		false, // notificationsEnabled
		false, // accessToSSOConfig
	)
	assert.NoError(t, err)

	// Create a non-admin user
	regularUser, err := service.CreateUser(
		"regular@example.com",
		"Regular User",
		"password123",
		false, // isAdmin
		true,  // showChat
		true,  // showPortal
		true,  // emailVerified
		false, // notificationsEnabled
		false, // accessToSSOConfig
	)
	assert.NoError(t, err)

	// Test 1: Enable AccessToSSOConfig for admin user (should succeed)
	updatedAdmin, err := service.UpdateUser(
		adminUser.ID,
		adminUser.Email,
		adminUser.Name,
		true,  // isAdmin
		true,  // showChat
		true,  // showPortal
		true,  // emailVerified
		false, // notificationsEnabled
		true,  // accessToSSOConfig
	)
	assert.NoError(t, err)
	assert.True(t, updatedAdmin.AccessToSSOConfig)

	// Test 2: Try to enable AccessToSSOConfig for non-admin user (should fail)
	_, err = service.UpdateUser(
		regularUser.ID,
		regularUser.Email,
		regularUser.Name,
		false, // isAdmin
		true,  // showChat
		true,  // showPortal
		true,  // emailVerified
		false, // notificationsEnabled
		true,  // accessToSSOConfig
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access to IdP configuration can only be enabled for admin users")

	// Test 3: Change admin user to non-admin with AccessToSSOConfig = true (should fail)
	_, err = service.UpdateUser(
		adminUser.ID,
		adminUser.Email,
		adminUser.Name,
		false, // isAdmin
		true,  // showChat
		true,  // showPortal
		true,  // emailVerified
		false, // notificationsEnabled
		true,  // accessToSSOConfig
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access to IdP configuration can only be enabled for admin users")

	// Test 4: Change admin user to non-admin with AccessToSSOConfig = false (should succeed)
	updatedUser, err := service.UpdateUser(
		adminUser.ID,
		adminUser.Email,
		adminUser.Name,
		false, // isAdmin
		true,  // showChat
		true,  // showPortal
		true,  // emailVerified
		false, // notificationsEnabled
		false, // accessToSSOConfig
	)
	assert.NoError(t, err)
	assert.False(t, updatedUser.IsAdmin)
	assert.False(t, updatedUser.AccessToSSOConfig)
}

func TestSkipQuickStartForUser(t *testing.T) {
	db := setupUserTestDB(t)
	service := NewService(db)

	// Create a test user
	user, err := service.CreateUser(
		"test@example.com",
		"Test User",
		"password123",
		false, // isAdmin
		true,  // showChat
		true,  // showPortal
		true,  // emailVerified
		false, // notificationsEnabled
		false, // accessToSSOConfig
	)
	assert.NoError(t, err)
	assert.NotNil(t, user)

	// Verify initial state - SkipQuickStart should be false by default
	assert.False(t, user.SkipQuickStart)

	// Call the SkipQuickStartForUser method
	err = service.SkipQuickStartForUser(user.ID)
	assert.NoError(t, err)

	// Fetch the user again to verify the flag was updated
	updatedUser, err := service.GetUserByID(user.ID)
	assert.NoError(t, err)
	assert.NotNil(t, updatedUser)

	// Verify SkipQuickStart is now true
	assert.True(t, updatedUser.SkipQuickStart)

	// Test with non-existent user ID
	err = service.SkipQuickStartForUser(9999)
	// This should not return an error since the update operation succeeds
	// even if no rows are affected (it's a valid SQL operation)
	assert.NoError(t, err)
}

func TestUpdateGroupUsers(t *testing.T) {
	db := setupUserTestDB(t)
	service := NewService(db)

	// Create test users
	user1, err := service.CreateUser("user1@example.com", "User 1", "password123", false, true, true, true, false, false)
	assert.NoError(t, err)

	user2, err := service.CreateUser("user2@example.com", "User 2", "password123", false, true, true, true, false, false)
	assert.NoError(t, err)

	user3, err := service.CreateUser("user3@example.com", "User 3", "password123", false, true, true, true, false, false)
	assert.NoError(t, err)

	// Create a group with user1 and user2
	group, err := service.CreateGroup("Test Group", []uint{user1.ID, user2.ID}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)

	// Verify initial users in the group
	fetchedGroup, err := service.GetGroupByID(group.ID, "Users")
	assert.NoError(t, err)
	assert.Len(t, fetchedGroup.Users, 2)

	userIDs := []uint{fetchedGroup.Users[0].ID, fetchedGroup.Users[1].ID}
	assert.Contains(t, userIDs, user1.ID)
	assert.Contains(t, userIDs, user2.ID)

	// Test 1: Update group users to user2 and user3 (removing user1 and adding user3)
	err = service.UpdateGroupUsers(group.ID, []uint{user2.ID, user3.ID})
	assert.NoError(t, err)

	// Verify updated users in the group
	fetchedGroup, err = service.GetGroupByID(group.ID, "Users")
	assert.NoError(t, err)
	assert.Len(t, fetchedGroup.Users, 2)

	userIDs = []uint{fetchedGroup.Users[0].ID, fetchedGroup.Users[1].ID}
	assert.Contains(t, userIDs, user2.ID)
	assert.Contains(t, userIDs, user3.ID)
	assert.NotContains(t, userIDs, user1.ID)

	// Test 2: Update group users to an empty list
	err = service.UpdateGroupUsers(group.ID, []uint{})
	assert.NoError(t, err)

	// Verify group has no users
	fetchedGroup, err = service.GetGroupByID(group.ID, "Users")
	assert.NoError(t, err)
	assert.Empty(t, fetchedGroup.Users)

	// Test 3: Try to update users for a non-existent group
	err = service.UpdateGroupUsers(9999, []uint{user1.ID})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "record not found")
}
