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
