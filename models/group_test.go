package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroup_NewGroup(t *testing.T) {
	group := NewGroup()
	assert.NotNil(t, group)
}

func TestGroup_CRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create
	group := &Group{Name: "Test Group"}
	err := group.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, group.ID)

	// Get
	fetchedGroup := NewGroup()
	err = fetchedGroup.Get(db, group.ID)
	assert.NoError(t, err)
	assert.Equal(t, group.Name, fetchedGroup.Name)

	// Update
	group.Name = "Updated Test Group"
	err = group.Update(db)
	assert.NoError(t, err)

	err = fetchedGroup.Get(db, group.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Test Group", fetchedGroup.Name)

	// Delete
	err = group.Delete(db)
	assert.NoError(t, err)

	err = fetchedGroup.Get(db, group.ID)
	assert.Error(t, err) // Should return an error as the group is deleted
}

func TestGroup_UserAssociation(t *testing.T) {
	db := setupTestDB(t)

	group := &Group{Name: "Test Group"}
	err := group.Create(db)
	assert.NoError(t, err)

	user := &User{Name: "Test User", Email: "test@example.com"}
	err = user.Create(db)
	assert.NoError(t, err)

	// Add User
	err = group.AddUser(db, user)
	assert.NoError(t, err)

	// Get Users
	err = group.GetGroupUsers(db)
	assert.NoError(t, err)
	assert.Len(t, group.Users, 1)
	assert.Equal(t, user.ID, group.Users[0].ID)

	// Remove User
	err = group.RemoveUser(db, user)
	assert.NoError(t, err)

	err = group.GetGroupUsers(db)
	assert.NoError(t, err)
	assert.Len(t, group.Users, 0)
}

func TestGroup_GroupsGetAll(t *testing.T) {
	db := setupTestDB(t)

	// Create some test groups
	groups := []Group{
		{Name: "Group 1"},
		{Name: "Group 2"},
		{Name: "Group 3"},
	}
	for _, g := range groups {
		err := db.Create(&g).Error
		assert.NoError(t, err)
	}

	// Test GetAll
	var fetchedGroups Groups
	err := fetchedGroups.GetAll(db)
	assert.NoError(t, err)
	assert.Len(t, fetchedGroups, 3)
	assert.Equal(t, "Group 1", fetchedGroups[0].Name)
	assert.Equal(t, "Group 2", fetchedGroups[1].Name)
	assert.Equal(t, "Group 3", fetchedGroups[2].Name)
}

func TestGroup_GetByNameStub(t *testing.T) {
	db := setupTestDB(t)

	// Create some test groups
	groups := []Group{
		{Name: "Apple Group"},
		{Name: "Banana Group"},
		{Name: "Apple Pie Group"},
		{Name: "Cherry Group"},
	}
	for _, g := range groups {
		err := db.Create(&g).Error
		assert.NoError(t, err)
	}

	// Test GetByNameStub
	var fetchedGroups Groups
	err := fetchedGroups.GetByNameStub(db, "Apple")
	assert.NoError(t, err)
	assert.Len(t, fetchedGroups, 2)
	assert.Equal(t, "Apple Group", fetchedGroups[0].Name)
	assert.Equal(t, "Apple Pie Group", fetchedGroups[1].Name)

	// Test with a different stub
	fetchedGroups = Groups{}
	err = fetchedGroups.GetByNameStub(db, "Cherry")
	assert.NoError(t, err)
	assert.Len(t, fetchedGroups, 1)
	assert.Equal(t, "Cherry Group", fetchedGroups[0].Name)

	// Test with a stub that doesn't match any groups
	fetchedGroups = Groups{}
	err = fetchedGroups.GetByNameStub(db, "Orange")
	assert.NoError(t, err)
	assert.Len(t, fetchedGroups, 0)
}

func TestGroup_CatalogueAssociation(t *testing.T) {
	db := setupTestDB(t)

	group := &Group{Name: "Test Group"}
	err := group.Create(db)
	assert.NoError(t, err)

	catalogue := &Catalogue{Name: "Test Catalogue"}
	err = catalogue.Create(db)
	assert.NoError(t, err)

	// Add Catalogue
	err = group.AddCatalogue(db, catalogue)
	assert.NoError(t, err)

	// Get Catalogues
	err = group.GetCatalogues(db)
	assert.NoError(t, err)
	assert.Len(t, group.Catalogues, 1)
	assert.Equal(t, catalogue.ID, group.Catalogues[0].ID)

	// Remove Catalogue
	err = group.RemoveCatalogue(db, catalogue)
	assert.NoError(t, err)

	err = group.GetCatalogues(db)
	assert.NoError(t, err)
	assert.Len(t, group.Catalogues, 0)
}
