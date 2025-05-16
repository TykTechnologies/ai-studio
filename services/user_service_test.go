package services

import (
	"fmt"
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

func TestSearchUsers(t *testing.T) {
	db := setupUserTestDB(t)
	service := NewService(db)

	// Create test users with different names and emails
	users := []struct {
		email    string
		name     string
		isAdmin  bool
		showChat bool
	}{
		{"johndoe@example.com", "John Doe", false, true},
		{"janedoe@example.com", "Jane Doe", false, true},
		{"bobsmith@example.com", "Bob Smith", false, true},
		{"alicejones@example.com", "Alice Jones", false, true},
		{"charliebrown@example.com", "Charlie Brown", false, true},
	}

	// Create users in the database
	for _, u := range users {
		_, err := service.CreateUser(
			u.email,
			u.name,
			"password123",
			u.isAdmin,
			u.showChat,
			true,  // showPortal
			true,  // emailVerified
			false, // notificationsEnabled
			false, // accessToSSOConfig
		)
		assert.NoError(t, err)
	}

	// Test search by email fragment
	results, totalCount, totalPages, err := service.SearchUsers("doe", 10, 1, false, "")
	assert.NoError(t, err)
	assert.Equal(t, int64(2), totalCount)
	assert.Equal(t, 1, totalPages)
	assert.Len(t, results, 2)

	emails := []string{results[0].Email, results[1].Email}
	assert.Contains(t, emails, "johndoe@example.com")
	assert.Contains(t, emails, "janedoe@example.com")

	// Test search by name fragment
	results, totalCount, totalPages, err = service.SearchUsers("Smith", 10, 1, false, "")
	assert.NoError(t, err)
	assert.Equal(t, int64(1), totalCount)
	assert.Equal(t, 1, totalPages)
	assert.Len(t, results, 1)
	assert.Equal(t, "bobsmith@example.com", results[0].Email)

	// Add more users to test pagination
	for i := 0; i < 10; i++ {
		_, err := service.CreateUser(
			fmt.Sprintf("brown%d@example.com", i),
			fmt.Sprintf("Brown %d", i),
			"password123",
			false, // isAdmin
			true,  // showChat
			true,  // showPortal
			true,  // emailVerified
			false, // notificationsEnabled
			false, // accessToSSOConfig
		)
		assert.NoError(t, err)
	}

	// Test pagination - first page
	results, totalCount, totalPages, err = service.SearchUsers("brown", 5, 1, false, "")
	assert.NoError(t, err)
	assert.Equal(t, int64(11), totalCount) // 10 "brown" + 1 "Charlie Brown"
	assert.Equal(t, 3, totalPages)
	assert.Len(t, results, 5)

	// Test pagination - second page
	results, totalCount, totalPages, err = service.SearchUsers("brown", 5, 2, false, "")
	assert.NoError(t, err)
	assert.Equal(t, int64(11), totalCount)
	assert.Equal(t, 3, totalPages)
	assert.Len(t, results, 5)

	// Test with empty search term (should return all users)
	results, totalCount, totalPages, err = service.SearchUsers("", 20, 1, false, "")
	assert.NoError(t, err)
	assert.Equal(t, int64(15), totalCount) // 5 original + 10 additional
	assert.Equal(t, 1, totalPages)
	assert.Len(t, results, 15)

	// Test sorting
	results, totalCount, totalPages, err = service.SearchUsers("brown", 20, 1, false, "-email")
	assert.NoError(t, err)
	assert.Equal(t, int64(11), totalCount)
	assert.Equal(t, 1, totalPages)
	assert.Len(t, results, 11)
	// First should be charliebrown@example.com when sorting by email DESC
	assert.Equal(t, "charliebrown@example.com", results[0].Email)

	// Test with all=true parameter (should return all results regardless of pagination)
	results, totalCount, totalPages, err = service.SearchUsers("brown", 3, 1, true, "email")
	assert.NoError(t, err)
	assert.Equal(t, int64(11), totalCount) // Should still report the correct total count
	assert.Equal(t, 4, totalPages)         // Should still calculate the correct page count
	assert.Len(t, results, 11)             // But should return ALL matching results
}
