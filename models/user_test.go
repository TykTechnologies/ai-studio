package models

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestUser_NewUser(t *testing.T) {
	user := NewUser()
	assert.NotNil(t, user)
}

func TestUser_UserCRUD(t *testing.T) {
	db := setupTestDB(t)

	user := &User{Email: "test@example.com", Password: "password"}

	// Test Create
	err := user.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, user.ID)

	// Test Get
	fetchedUser := NewUser()
	err = fetchedUser.Get(db, user.ID)
	assert.NoError(t, err)
	assert.Equal(t, user.Email, fetchedUser.Email)

	// Test Update
	user.Email = "updated@example.com"
	err = user.Update(db)
	assert.NoError(t, err)

	err = fetchedUser.Get(db, user.ID)
	assert.NoError(t, err)
	assert.Equal(t, "updated@example.com", fetchedUser.Email)

	// Test Delete
	err = user.Delete(db)
	assert.NoError(t, err)

	err = fetchedUser.Get(db, user.ID)
	assert.Error(t, err) // Should return an error as the user is deleted
}

func TestUser_GetByEmail(t *testing.T) {
	db := setupTestDB(t)

	user := &User{Email: "test@example.com", Password: "password"}
	err := user.Create(db)
	assert.NoError(t, err)

	fetchedUser := NewUser()
	err = fetchedUser.GetByEmail(db, "test@example.com")
	assert.NoError(t, err)
	assert.Equal(t, user.ID, fetchedUser.ID)
}

func TestUser_DoesPasswordMatch(t *testing.T) {
	user := &User{Password: "hashed_password"}

	// Mock HashPassword function
	oldHashPassword := HashPassword
	oldIsPasswordValid := IsPasswordValid
	defer func() {
		HashPassword = oldHashPassword
		IsPasswordValid = oldIsPasswordValid
	}()
	HashPassword = func(password string) (string, error) {
		if password == "correct_password" {
			return "hashed_password", nil
		}
		return "", nil
	}

	IsPasswordValid = func(password, hashedPassword string) bool {
		return password == "correct_password" && hashedPassword == "hashed_password"
	}

	assert.True(t, user.DoesPasswordMatch("correct_password"))
	assert.False(t, user.DoesPasswordMatch("wrong_password"))
}

func TestUser_SetPassword(t *testing.T) {
	user := NewUser()

	// Mock HashPassword function
	oldHashPassword := HashPassword
	defer func() { HashPassword = oldHashPassword }()
	HashPassword = func(password string) (string, error) {
		return "hashed_" + password, nil
	}

	err := user.SetPassword("new_password")
	assert.NoError(t, err)
	assert.Equal(t, "hashed_new_password", user.Password)
}

func TestGetAll(t *testing.T) {
	db := setupTestDB(t)

	users := []User{
		{Email: "user1@example.com"},
		{Email: "user2@example.com"},
	}
	for _, u := range users {
		err := db.Create(&u).Error
		assert.NoError(t, err)
	}

	var fetchedUsers Users
	_, _, err := fetchedUsers.GetAll(db, 10, 1, true, "id")
	assert.NoError(t, err)
	assert.Len(t, fetchedUsers, 2)
}

func TestUser_SearchByEmailStub(t *testing.T) {
	db := setupTestDB(t)

	users := []User{
		{Email: "alice@example.com"},
		{Email: "bob@example.com"},
		{Email: "charlie@example.com"},
	}
	for _, u := range users {
		err := db.Create(&u).Error
		assert.NoError(t, err)
	}

	var fetchedUsers Users
	err := fetchedUsers.SearchByEmailStub(db, "al")
	assert.NoError(t, err)
	assert.Len(t, fetchedUsers, 1)
	assert.Equal(t, "alice@example.com", fetchedUsers[0].Email)
}

func TestUser_GetByGroupID(t *testing.T) {
	db := setupTestDB(t)

	// Create a group
	group := &Group{Name: "Test Group"}
	err := group.Create(db)
	assert.NoError(t, err)

	// Create users
	users := []User{
		{Email: "user1@example.com", Name: "User 1"},
		{Email: "user2@example.com", Name: "User 2"},
		{Email: "user3@example.com", Name: "User 3"},
	}
	for _, u := range users {
		err := u.Create(db)
		assert.NoError(t, err)
	}

	// Add users to the group
	err = group.AddUser(db, &users[0])
	assert.NoError(t, err)
	err = group.AddUser(db, &users[1])
	assert.NoError(t, err)
	// Note: user3 is not added to the group

	// Test GetByGroupID
	var fetchedUsers Users
	err = fetchedUsers.GetByGroupID(db, group.ID)
	assert.NoError(t, err)

	// Check the results
	assert.Len(t, fetchedUsers, 2)
	assert.Equal(t, users[0].Email, fetchedUsers[0].Email)
	assert.Equal(t, users[1].Email, fetchedUsers[1].Email)

	// Test with a non-existent group ID
	fetchedUsers = Users{}
	err = fetchedUsers.GetByGroupID(db, 9999) // Assuming 9999 is not a valid group ID
	assert.NoError(t, err)
	assert.Len(t, fetchedUsers, 0)
}

func TestUser_GetAccessibleCatalogues(t *testing.T) {
	db := setupTestDB(t)

	user := &User{Email: "test@example.com", Password: "password"}
	err := user.Create(db)
	assert.NoError(t, err)

	group1 := &Group{Name: "Group 1"}
	err = group1.Create(db)
	assert.NoError(t, err)

	group2 := &Group{Name: "Group 2"}
	err = group2.Create(db)
	assert.NoError(t, err)

	catalogue1 := &Catalogue{Name: "Catalogue 1"}
	err = catalogue1.Create(db)
	assert.NoError(t, err)

	catalogue2 := &Catalogue{Name: "Catalogue 2"}
	err = catalogue2.Create(db)
	assert.NoError(t, err)

	// Add user to groups
	err = group1.AddUser(db, user)
	assert.NoError(t, err)
	err = group2.AddUser(db, user)
	assert.NoError(t, err)

	// Add catalogues to groups
	err = group1.AddCatalogue(db, catalogue1)
	assert.NoError(t, err)
	err = group2.AddCatalogue(db, catalogue2)
	assert.NoError(t, err)

	// Get accessible catalogues
	accessibleCatalogues, err := user.GetAccessibleCatalogues(db)
	assert.NoError(t, err)
	assert.Len(t, accessibleCatalogues, 2)
	assert.ElementsMatch(t, []string{"Catalogue 1", "Catalogue 2"}, []string{accessibleCatalogues[0].Name, accessibleCatalogues[1].Name})
}

func TestUser_UpdateGroupMemberships(t *testing.T) {
	db := setupTestDB(t)

	// Create a user
	user := &User{Email: "test@example.com", Password: "password"}
	err := user.Create(db)
	assert.NoError(t, err)

	// Create multiple groups
	group1 := &Group{Name: "Group 1"}
	err = group1.Create(db)
	assert.NoError(t, err)

	group2 := &Group{Name: "Group 2"}
	err = group2.Create(db)
	assert.NoError(t, err)

	group3 := &Group{Name: "Group 3"}
	err = group3.Create(db)
	assert.NoError(t, err)

	// Test 1: Add user to multiple groups
	err = user.UpdateGroupMemberships(db,
		strconv.FormatUint(uint64(group1.ID), 10),
		strconv.FormatUint(uint64(group2.ID), 10))
	assert.NoError(t, err)

	// Verify user is in both groups
	var fetchedUser User
	err = db.Preload("Groups").First(&fetchedUser, user.ID).Error
	assert.NoError(t, err)
	assert.Len(t, fetchedUser.Groups, 2)

	// Verify the correct groups were assigned
	groupIDs := []uint{fetchedUser.Groups[0].ID, fetchedUser.Groups[1].ID}
	assert.Contains(t, groupIDs, group1.ID)
	assert.Contains(t, groupIDs, group2.ID)

	// Test 2: Change user's groups (replace existing groups)
	err = user.UpdateGroupMemberships(db,
		strconv.FormatUint(uint64(group2.ID), 10),
		strconv.FormatUint(uint64(group3.ID), 10))
	assert.NoError(t, err)

	// Verify user is now in the new set of groups
	err = db.Preload("Groups").First(&fetchedUser, user.ID).Error
	assert.NoError(t, err)
	assert.Len(t, fetchedUser.Groups, 2)

	// Verify the correct groups were assigned
	groupIDs = []uint{fetchedUser.Groups[0].ID, fetchedUser.Groups[1].ID}
	assert.Contains(t, groupIDs, group2.ID)
	assert.Contains(t, groupIDs, group3.ID)
	assert.NotContains(t, groupIDs, group1.ID) // Should no longer be in group1

	// Test 3: Remove all groups
	err = user.UpdateGroupMemberships(db) // No group IDs provided
	assert.NoError(t, err)

	// Verify user has no groups
	err = db.Preload("Groups").First(&fetchedUser, user.ID).Error
	assert.NoError(t, err)
	assert.Len(t, fetchedUser.Groups, 0)

	// Test 4: Invalid group ID
	err = user.UpdateGroupMemberships(db, "invalid_id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid group ID")

	// Test 5: Non-existent group ID
	err = user.UpdateGroupMemberships(db, "9999")
	assert.NoError(t, err) // Should not error, just assign to an empty set of groups

	// Verify user has no groups (since group 9999 doesn't exist)
	err = db.Preload("Groups").First(&fetchedUser, user.ID).Error
	assert.NoError(t, err)
	assert.Len(t, fetchedUser.Groups, 0)
}

func TestUser_AccessToSSOConfig(t *testing.T) {
	db := setupTestDB(t)

	// Test creating a user with AccessToSSOConfig = true
	user := &User{
		Email:             "test@example.com",
		Name:              "Test User",
		IsAdmin:           true,
		AccessToSSOConfig: true,
	}
	err := user.Create(db)
	assert.NoError(t, err)
	assert.True(t, user.AccessToSSOConfig)

	// Test retrieving the user
	retrievedUser := NewUser()
	err = retrievedUser.Get(db, user.ID)
	assert.NoError(t, err)
	assert.True(t, retrievedUser.AccessToSSOConfig)

	// Test updating the user's AccessToSSOConfig field
	retrievedUser.AccessToSSOConfig = false
	err = retrievedUser.Update(db)
	assert.NoError(t, err)

	// Verify the update
	updatedUser := NewUser()
	err = updatedUser.Get(db, user.ID)
	assert.NoError(t, err)
	assert.False(t, updatedUser.AccessToSSOConfig)
}

func TestUser_AccessToSSOConfigValidation(t *testing.T) {
	db := setupTestDB(t)

	// Test that non-admin users cannot have AccessToSSOConfig = true
	// This is enforced at the service layer, but we can test the model behavior

	// Create a non-admin user with AccessToSSOConfig = true
	nonAdminUser := &User{
		Email:             "nonadmin@example.com",
		Name:              "Non Admin User",
		IsAdmin:           false,
		AccessToSSOConfig: true, // This would be rejected by the service layer
	}

	// The model itself doesn't enforce this constraint, so it should save successfully
	err := nonAdminUser.Create(db)
	assert.NoError(t, err)

	// Retrieve the user to verify the field was saved
	retrievedUser := NewUser()
	err = retrievedUser.Get(db, nonAdminUser.ID)
	assert.NoError(t, err)
	assert.True(t, retrievedUser.AccessToSSOConfig)
	assert.False(t, retrievedUser.IsAdmin)
}

func TestIsEmailUnique(t *testing.T) {
	db := setupTestDB(t)

	user1 := &User{Email: "test@example.com", Name: "Test User 1"}
	err := user1.Create(db)
	assert.NoError(t, err)

	user2 := &User{Email: "another@example.com", Name: "Test User 2"}
	err = user2.Create(db)
	assert.NoError(t, err)

	// Test case 1: Check if a new email is unique
	isUnique, err := IsEmailUnique(db, "new@example.com", 0)
	assert.NoError(t, err)
	assert.True(t, isUnique)

	// Test case 2: Check if an existing email is not unique
	isUnique, err = IsEmailUnique(db, "test@example.com", 0)
	assert.NoError(t, err)
	assert.False(t, isUnique)

	// Test case 3: Check if an existing email with different case is not unique
	isUnique, err = IsEmailUnique(db, "TEST@example.com", 0)
	assert.NoError(t, err)
	assert.False(t, isUnique)

	// Test case 4: Check if an existing email is unique when excluding the user
	isUnique, err = IsEmailUnique(db, "test@example.com", user1.ID)
	assert.NoError(t, err)
	assert.True(t, isUnique)

	// Test case 5: Check if another user's email is not unique
	isUnique, err = IsEmailUnique(db, "another@example.com", user1.ID)
	assert.NoError(t, err)
	assert.False(t, isUnique)
}

func TestSetSkipQuickStartForUser(t *testing.T) {
	db := setupTestDB(t)

	// Create a test user with SkipQuickStart = false
	user := &User{
		Email:          "test@example.com",
		Name:           "Test User",
		SkipQuickStart: false,
	}
	err := user.Create(db)
	assert.NoError(t, err)
	assert.False(t, user.SkipQuickStart)

	// Call the SetSkipQuickStartForUser function
	err = SetSkipQuickStartForUser(db, user.ID)
	assert.NoError(t, err)

	// Retrieve the user and verify SkipQuickStart is now true
	var updatedUser User
	err = db.First(&updatedUser, user.ID).Error
	assert.NoError(t, err)
	assert.True(t, updatedUser.SkipQuickStart)

	// Test with non-existent user ID
	err = SetSkipQuickStartForUser(db, 9999)
	assert.NoError(t, err) // Should not error, just not update any rows

	// Test idempotency - calling the function again should not cause errors
	err = SetSkipQuickStartForUser(db, user.ID)
	assert.NoError(t, err)

	// Verify SkipQuickStart is still true
	var reUpdatedUser User
	err = db.First(&reUpdatedUser, user.ID).Error
	assert.NoError(t, err)
	assert.True(t, reUpdatedUser.SkipQuickStart)
}
func TestGetUserGroupCount(t *testing.T) {
	db := setupTestDB(t)

	// Initially, there should be no groups
	count, err := GetUserGroupCount(db)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Create some groups
	groups := []Group{
		{Name: "Group 1"},
		{Name: "Group 2"},
		{Name: "Group 3"},
	}
	for _, g := range groups {
		err := db.Create(&g).Error
		assert.NoError(t, err)
	}

	// Now there should be 3 groups
	count, err = GetUserGroupCount(db)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestGetUserCounts(t *testing.T) {
	db := setupTestDB(t)

	// Create different types of users
	users := []User{
		{Email: "admin@example.com", IsAdmin: true, ShowPortal: true, ShowChat: true},        // Admin
		{Email: "admin2@example.com", IsAdmin: true, ShowPortal: false, ShowChat: false},     // Admin (different settings)
		{Email: "developer@example.com", IsAdmin: false, ShowPortal: true, ShowChat: true},   // Developer
		{Email: "developer2@example.com", IsAdmin: false, ShowPortal: true, ShowChat: false}, // Developer (chat disabled)
		{Email: "chatuser@example.com", IsAdmin: false, ShowPortal: false, ShowChat: true},   // Chat user
		{Email: "inactive@example.com", IsAdmin: false, ShowPortal: false, ShowChat: false},  // Inactive user
	}

	// Insert users into the database
	for _, u := range users {
		err := db.Create(&u).Error
		assert.NoError(t, err)
	}

	// Call the function being tested
	counts, err := GetUserCounts(db)
	assert.NoError(t, err)

	// Verify the counts
	assert.Equal(t, int64(6), counts.UserCount, "Total user count should be 6")
	assert.Equal(t, int64(2), counts.AdminCount, "Admin count should be 2")
	assert.Equal(t, int64(2), counts.DeveloperCount, "Developer count should be 2")
	assert.Equal(t, int64(1), counts.ChatUserCount, "Chat user count should be 1")

	// Test with an empty database
	db = setupTestDB(t) // Reset the database
	emptyCounts, err := GetUserCounts(db)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), emptyCounts.UserCount)
	assert.Equal(t, int64(0), emptyCounts.AdminCount)
	assert.Equal(t, int64(0), emptyCounts.DeveloperCount)
	assert.Equal(t, int64(0), emptyCounts.ChatUserCount)

	// Test with soft-deleted users
	db = setupTestDB(t) // Reset the database
	user := User{Email: "deleted@example.com", IsAdmin: true}
	err = db.Create(&user).Error
	assert.NoError(t, err)

	// Verify user is counted
	countsBefore, err := GetUserCounts(db)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), countsBefore.UserCount)

	// Soft delete the user
	err = db.Delete(&user).Error
	assert.NoError(t, err)

	// Verify soft-deleted user is not counted (GORM's default behavior)
	countsAfter, err := GetUserCounts(db)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), countsAfter.UserCount)
}
