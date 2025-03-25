package models

import (
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
