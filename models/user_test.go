package models

import (
	"fmt"
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

func TestGetRole(t *testing.T) {
	// Test Super Admin (ID=1)
	user := &User{
		ID: 1,
	}
	assert.Equal(t, "Super Admin", user.GetRole())

	// Test Admin
	user = &User{
		ID:      2,
		IsAdmin: true,
	}
	assert.Equal(t, "Admin", user.GetRole())

	// Test Developer
	user = &User{
		ID:         3,
		IsAdmin:    false,
		ShowPortal: true,
	}
	assert.Equal(t, "Developer", user.GetRole())

	// Test Chat user
	user = &User{
		ID:         4,
		IsAdmin:    false,
		ShowPortal: false,
	}
	assert.Equal(t, "Chat user", user.GetRole())
}

func TestSearchByTerm(t *testing.T) {
	db := setupTestDB(t)

	// Create test users with different names and emails
	users := []User{
		{Email: "johndoe@example.com", Name: "John Doe"},
		{Email: "janedoe@example.com", Name: "Jane Doe"},
		{Email: "bobsmith@example.com", Name: "Bob Smith"},
		{Email: "alicejones@example.com", Name: "Alice Jones"},
		{Email: "charliebrown@example.com", Name: "Charlie Brown"},
	}

	for _, user := range users {
		err := db.Create(&user).Error
		assert.NoError(t, err)
	}

	var results Users

	// Test search by email fragment
	totalCount, totalPages, err := results.SearchByTerm(db, "doe", 10, 1, false, "")
	assert.NoError(t, err)
	assert.Equal(t, int64(2), totalCount)
	assert.Equal(t, 1, totalPages)
	assert.Len(t, results, 2)

	emails := []string{results[0].Email, results[1].Email}
	assert.Contains(t, emails, "johndoe@example.com")
	assert.Contains(t, emails, "janedoe@example.com")

	// Test search by name fragment
	results = Users{}
	totalCount, totalPages, err = results.SearchByTerm(db, "Smith", 10, 1, false, "")
	assert.NoError(t, err)
	assert.Equal(t, int64(1), totalCount)
	assert.Equal(t, 1, totalPages)
	assert.Len(t, results, 1)
	assert.Equal(t, "bobsmith@example.com", results[0].Email)

	// Test pagination
	// Add more users to ensure pagination
	for i := 0; i < 10; i++ {
		user := User{
			Email: fmt.Sprintf("brown%d@example.com", i),
			Name:  fmt.Sprintf("Brown %d", i),
		}
		err := db.Create(&user).Error
		assert.NoError(t, err)
	}

	results = Users{}
	totalCount, totalPages, err = results.SearchByTerm(db, "brown", 5, 1, false, "")
	assert.NoError(t, err)
	assert.Equal(t, int64(11), totalCount) // 10 "brown" + 1 "Charlie Brown"
	assert.Equal(t, 3, totalPages)
	assert.Len(t, results, 5)

	// Test second page
	results = Users{}
	totalCount, totalPages, err = results.SearchByTerm(db, "brown", 5, 2, false, "")
	assert.NoError(t, err)
	assert.Equal(t, int64(11), totalCount)
	assert.Equal(t, 3, totalPages)
	assert.Len(t, results, 5)

	// Test with empty search term (should return all users)
	results = Users{}
	totalCount, totalPages, err = results.SearchByTerm(db, "", 20, 1, false, "")
	assert.NoError(t, err)
	assert.Equal(t, int64(15), totalCount) // 5 original + 10 additional
	assert.Equal(t, 1, totalPages)
	assert.Len(t, results, 15)

	// Test sorting
	results = Users{}
	totalCount, totalPages, err = results.SearchByTerm(db, "brown", 20, 1, false, "-email")
	assert.NoError(t, err)
	assert.Equal(t, int64(11), totalCount)
	assert.Equal(t, 1, totalPages)
	assert.Len(t, results, 11)
	// First should be charliebrown@example.com when sorting by email DESC
	assert.Equal(t, "charliebrown@example.com", results[0].Email)
	// Test with all=true parameter (should return all results regardless of pagination)
	results = Users{}
	totalCount, totalPages, err = results.SearchByTerm(db, "brown", 3, 1, true, "email")
	assert.NoError(t, err)
	assert.Equal(t, int64(11), totalCount) // Should still report the correct total count
	assert.Equal(t, 4, totalPages)         // Should still calculate the correct page count
	assert.Len(t, results, 11)             // But should return ALL matching results, not just the first page
}

func TestGetGroupUsersPaginated(t *testing.T) {
	db := setupTestDB(t)

	// Helper to create users and a group, and associate them
	createTestData := func(numUsers int, groupName string) (Group, []User) {
		group := Group{Name: groupName}
		err := db.Create(&group).Error
		assert.NoError(t, err)

		users := make([]User, numUsers)
		for i := 0; i < numUsers; i++ {
			users[i] = User{Email: fmt.Sprintf("user%d@%s.com", i+1, groupName), Name: fmt.Sprintf("User %d %s", i+1, groupName)}
			err = db.Create(&users[i]).Error
			assert.NoError(t, err)
			err = db.Model(&group).Association("Users").Append(&users[i])
			assert.NoError(t, err)
		}
		return group, users
	}

	// Test case 1: Basic pagination
	t.Run("Basic pagination", func(t *testing.T) {
		group, expectedUsers := createTestData(5, "group1") // Create 5 users in group1
		pageSize := 3
		pageNumber := 1

		var users Users
		count, totalPages, err := users.GetGroupUsersPaginated(db, group.ID, pageSize, pageNumber, false)

		assert.NoError(t, err)
		assert.Equal(t, int64(5), count)
		assert.Equal(t, 2, totalPages) // 5 users / 3 per page = 2 pages
		assert.Len(t, users, 3)        // Should get 3 users for the first page
		assert.Equal(t, expectedUsers[0].Email, users[0].Email)
		assert.Equal(t, expectedUsers[1].Email, users[1].Email)
		assert.Equal(t, expectedUsers[2].Email, users[2].Email)

		// Test second page
		pageNumber = 2
		var usersPage2 Users
		count, totalPages, err = usersPage2.GetGroupUsersPaginated(db, group.ID, pageSize, pageNumber, false)
		assert.NoError(t, err)
		assert.Equal(t, int64(5), count)
		assert.Equal(t, 2, totalPages)
		assert.Len(t, usersPage2, 2) // Should get the remaining 2 users
		assert.Equal(t, expectedUsers[3].Email, usersPage2[0].Email)
		assert.Equal(t, expectedUsers[4].Email, usersPage2[1].Email)
	})

	// Test case 2: Empty group
	t.Run("Empty group", func(t *testing.T) {
		group := Group{Name: "emptygroup"}
		err := db.Create(&group).Error
		assert.NoError(t, err)

		pageSize := 10
		pageNumber := 1

		var users Users
		count, totalPages, err := users.GetGroupUsersPaginated(db, group.ID, pageSize, pageNumber, false)

		assert.NoError(t, err)
		assert.Equal(t, int64(0), count)
		assert.Equal(t, 0, totalPages)
		assert.Len(t, users, 0)
	})

	// Test case 3: Pagination with all=true
	t.Run("Pagination with all=true", func(t *testing.T) {
		group, expectedUsers := createTestData(7, "groupAll") // Create 7 users

		var users Users
		pageSize := 3
		count, totalPages, err := users.GetGroupUsersPaginated(db, group.ID, pageSize, 1, true)

		assert.NoError(t, err)
		assert.Equal(t, int64(7), count)
		assert.Equal(t, 3, totalPages)
		assert.Len(t, users, len(expectedUsers))
		for i := range expectedUsers {
			assert.Equal(t, expectedUsers[i].Email, users[i].Email)
		}
	})

	// Test case 4: Group not found (non-existent group ID)
	t.Run("Group not found", func(t *testing.T) {
		nonExistentGroupID := uint(99999)
		pageSize := 10
		pageNumber := 1

		var users Users
		count, totalPages, err := users.GetGroupUsersPaginated(db, nonExistentGroupID, pageSize, pageNumber, false)

		assert.NoError(t, err) // The function itself might not error, but return 0 counts
		assert.Equal(t, int64(0), count)
		assert.Equal(t, 0, totalPages)
		assert.Len(t, users, 0)
	})

	// Test case 5: Pagination with more users than default page size limit when all=true
	// This tests if PaginateAndSort correctly returns all users even if their count exceeds a typical page limit.
	t.Run("Pagination with all=true and many users", func(t *testing.T) {
		// Create more users than a typical page size might handle (e.g., > 100 if default is 100 in PaginateAndSort)
		// For this test, let's simulate a scenario where 'all' is truly necessary.
		// We'll use a number like 150, assuming PaginateAndSort might have an internal cap before 'all' kicks in fully.
		// The actual PaginateAndSort logic needs to be considered for an exact value, but 150 is a good test.
		numUsers := 150
		group, expectedUsers := createTestData(numUsers, "groupMany")

		var users Users
		// pageSize and pageNumber are ignored for fetching data by PaginateAndSort when all=true,
		// but pageSize is still used for totalPages calculation.
		pageSize := 10
		count, totalPages, err := users.GetGroupUsersPaginated(db, group.ID, pageSize, 1, true)

		assert.NoError(t, err)
		assert.Equal(t, int64(numUsers), count)
		// totalPages = ceil(totalCount / pageSize) = ceil(150/10) = 15
		assert.Equal(t, 15, totalPages)
		assert.Len(t, users, len(expectedUsers))
		// Optionally, check a few users to ensure correctness if iterating all 150 is too slow for a typical test run
		assert.Equal(t, expectedUsers[0].Email, users[0].Email)
		assert.Equal(t, expectedUsers[numUsers-1].Email, users[numUsers-1].Email)
	})

	// Test case 6: User not in any group
	t.Run("User not in any group, try to get users for a group", func(t *testing.T) {
		_, _ = createTestData(3, "groupWithUsers") // Creates group1 and users
		lonelyUser := User{Email: "lonely@example.com", Name: "Lonely User"}
		err := db.Create(&lonelyUser).Error
		assert.NoError(t, err)

		// Create another group that lonelyUser is not part of
		otherGroup := Group{Name: "otherGroupForLonelyTest"}
		err = db.Create(&otherGroup).Error
		assert.NoError(t, err)

		var users Users
		count, totalPages, err := users.GetGroupUsersPaginated(db, otherGroup.ID, 10, 1, false)
		assert.NoError(t, err)
		assert.Equal(t, int64(0), count)
		assert.Equal(t, 0, totalPages)
		assert.Len(t, users, 0)
	})

	// Test case 7: Multiple groups, ensure correct users are fetched for a specific group
	t.Run("Multiple groups exist", func(t *testing.T) {
		groupA, usersA := createTestData(2, "groupA")
		_, _ = createTestData(3, "groupB") // Users in groupB are not expected

		var users Users
		count, totalPages, err := users.GetGroupUsersPaginated(db, groupA.ID, 5, 1, false)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), count)
		assert.Equal(t, 1, totalPages)
		assert.Len(t, users, 2)
		assert.Contains(t, users.toEmails(), usersA[0].Email)
		assert.Contains(t, users.toEmails(), usersA[1].Email)
	})
}

// Helper function for tests if needed
func (users Users) toEmails() []string {
	emails := make([]string, len(users))
	for i, u := range users {
		emails[i] = u.Email
	}
	return emails
}
