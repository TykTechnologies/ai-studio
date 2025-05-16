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
	_, _, err := fetchedGroups.GetAll(db, 10, 1, true)
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

func TestGroup_DataCatalogueAssociation(t *testing.T) {
	db := setupTestDB(t)

	group := &Group{Name: "Test Group"}
	err := group.Create(db)
	assert.NoError(t, err)

	dataCatalogue := &DataCatalogue{Name: "Test Data Catalogue"}
	err = dataCatalogue.Create(db)
	assert.NoError(t, err)

	// Add DataCatalogue
	err = group.AddDataCatalogue(db, dataCatalogue)
	assert.NoError(t, err)

	// Get DataCatalogues
	err = group.GetDataCatalogues(db)
	assert.NoError(t, err)
	assert.Len(t, group.DataCatalogues, 1)
	assert.Equal(t, dataCatalogue.ID, group.DataCatalogues[0].ID)

	// Remove DataCatalogue
	err = group.RemoveDataCatalogue(db, dataCatalogue)
	assert.NoError(t, err)

	err = group.GetDataCatalogues(db)
	assert.NoError(t, err)
	assert.Len(t, group.DataCatalogues, 0)
}

func TestGroup_ToolCatalogueAssociation(t *testing.T) {
	db := setupTestDB(t)

	group := &Group{Name: "Test Group"}
	err := group.Create(db)
	assert.NoError(t, err)

	toolCatalogue := &ToolCatalogue{Name: "Test Tool Catalogue"}
	err = toolCatalogue.Create(db)
	assert.NoError(t, err)

	// Add ToolCatalogue
	err = group.AddToolCatalogue(db, toolCatalogue)
	assert.NoError(t, err)

	// Get ToolCatalogues
	_, _, err = group.GetToolCatalogues(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, group.ToolCatalogues, 1)
	assert.Equal(t, toolCatalogue.ID, group.ToolCatalogues[0].ID)

	// Remove ToolCatalogue
	err = group.RemoveToolCatalogue(db, toolCatalogue)
	assert.NoError(t, err)

	_, _, err = group.GetToolCatalogues(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, group.ToolCatalogues, 0)
}

func TestGroup_GetGroupsByUserID(t *testing.T) {
	db := setupTestDB(t)

	// Create test users and groups
	user1 := &User{Name: "User 1", Email: "user1@example.com"}
	user2 := &User{Name: "User 2", Email: "user2@example.com"}
	err := user1.Create(db)
	assert.NoError(t, err)
	err = user2.Create(db)
	assert.NoError(t, err)

	group1 := &Group{Name: "Group 1"}
	group2 := &Group{Name: "Group 2"}
	group3 := &Group{Name: "Group 3"}
	err = group1.Create(db)
	assert.NoError(t, err)
	err = group2.Create(db)
	assert.NoError(t, err)
	err = group3.Create(db)
	assert.NoError(t, err)

	// Associate users with groups
	err = group1.AddUser(db, user1)
	assert.NoError(t, err)
	err = group2.AddUser(db, user1)
	assert.NoError(t, err)
	err = group3.AddUser(db, user2)
	assert.NoError(t, err)

	// Test GetGroupsByUserID
	var fetchedGroups Groups
	err = fetchedGroups.GetGroupsByUserID(db, user1.ID)
	assert.NoError(t, err)
	assert.Len(t, fetchedGroups, 2)
	assert.Contains(t, []string{fetchedGroups[0].Name, fetchedGroups[1].Name}, "Group 1")
	assert.Contains(t, []string{fetchedGroups[0].Name, fetchedGroups[1].Name}, "Group 2")

	fetchedGroups = Groups{}
	err = fetchedGroups.GetGroupsByUserID(db, user2.ID)
	assert.NoError(t, err)
	assert.Len(t, fetchedGroups, 1)
	assert.Equal(t, "Group 3", fetchedGroups[0].Name)
}

func TestReplaceAssociation(t *testing.T) {
	db := setupTestDB(t)

	// Create test data
	group := &Group{Name: "Test Group"}
	err := group.Create(db)
	assert.NoError(t, err)

	user1 := &User{Email: "user1@example.com"}
	user2 := &User{Email: "user2@example.com"}
	err = user1.Create(db)
	assert.NoError(t, err)
	err = user2.Create(db)
	assert.NoError(t, err)

	// Test replacing Users association
	users := []User{*user1, *user2}
	err = group.ReplaceAssociation(db, "Users", users)
	assert.NoError(t, err)

	// Verify association was replaced
	err = group.GetGroupUsers(db)
	assert.NoError(t, err)
	assert.Len(t, group.Users, 2)
	assert.Equal(t, user1.ID, group.Users[0].ID)
	assert.Equal(t, user2.ID, group.Users[1].ID)

	// Replace with a subset
	err = group.ReplaceAssociation(db, "Users", []User{*user1})
	assert.NoError(t, err)

	// Verify association was updated
	err = group.GetGroupUsers(db)
	assert.NoError(t, err)
	assert.Len(t, group.Users, 1)
	assert.Equal(t, user1.ID, group.Users[0].ID)

	// Replace with empty slice
	err = group.ReplaceAssociation(db, "Users", []User{})
	assert.NoError(t, err)

	// Verify association was cleared
	err = group.GetGroupUsers(db)
	assert.NoError(t, err)
	assert.Len(t, group.Users, 0)
}

func TestParseAssociations(t *testing.T) {
	group := &Group{}

	// Test with non-empty arrays
	userIDs := []uint{1, 2, 3}
	catalogueIDs := []uint{4, 5}
	dataCatalogueIDs := []uint{6, 7, 8}
	toolCatalogueIDs := []uint{9}

	group.ParseAssociations(userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs)

	// Verify Users
	assert.Len(t, group.Users, 3)
	assert.Equal(t, uint(1), group.Users[0].ID)
	assert.Equal(t, uint(2), group.Users[1].ID)
	assert.Equal(t, uint(3), group.Users[2].ID)

	// Verify Catalogues
	assert.Len(t, group.Catalogues, 2)
	assert.Equal(t, uint(4), group.Catalogues[0].ID)
	assert.Equal(t, uint(5), group.Catalogues[1].ID)

	// Verify DataCatalogues
	assert.Len(t, group.DataCatalogues, 3)
	assert.Equal(t, uint(6), group.DataCatalogues[0].ID)
	assert.Equal(t, uint(7), group.DataCatalogues[1].ID)
	assert.Equal(t, uint(8), group.DataCatalogues[2].ID)

	// Verify ToolCatalogues
	assert.Len(t, group.ToolCatalogues, 1)
	assert.Equal(t, uint(9), group.ToolCatalogues[0].ID)

	// Test with empty arrays
	group = &Group{}
	group.ParseAssociations([]uint{}, []uint{}, []uint{}, []uint{})
	assert.Len(t, group.Users, 0)
	assert.Len(t, group.Catalogues, 0)
	assert.Len(t, group.DataCatalogues, 0)
	assert.Len(t, group.ToolCatalogues, 0)
}

func TestExtractAssociationsIDs(t *testing.T) {
	// Prepare a group with associations
	group := &Group{
		Users: []User{
			{ID: 1}, {ID: 2}, {ID: 3},
		},
		Catalogues: []Catalogue{
			{ID: 4}, {ID: 5},
		},
		DataCatalogues: []DataCatalogue{
			{ID: 6}, {ID: 7}, {ID: 8},
		},
		ToolCatalogues: []ToolCatalogue{
			{ID: 9},
		},
	}

	// Extract IDs
	userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs := group.ExtractAssociationsIDs()

	// Verify extracted IDs
	assert.Equal(t, []uint{1, 2, 3}, userIDs)
	assert.Equal(t, []uint{4, 5}, catalogueIDs)
	assert.Equal(t, []uint{6, 7, 8}, dataCatalogueIDs)
	assert.Equal(t, []uint{9}, toolCatalogueIDs)

	// Test with empty associations
	group = &Group{}
	userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs = group.ExtractAssociationsIDs()
	assert.Empty(t, userIDs)
	assert.Empty(t, catalogueIDs)
	assert.Empty(t, dataCatalogueIDs)
	assert.Empty(t, toolCatalogueIDs)
}

func TestGetAssociationsToUpdate(t *testing.T) {
	t.Run("No changes needed", func(t *testing.T) {
		// Prepare a group with initial associations
		group := &Group{
			Users: []User{
				{ID: 1}, {ID: 2},
			},
			Catalogues: []Catalogue{
				{ID: 3}, {ID: 4},
			},
			DataCatalogues: []DataCatalogue{
				{ID: 5}, {ID: 6},
			},
			ToolCatalogues: []ToolCatalogue{
				{ID: 7}, {ID: 8},
			},
		}

		userIDs := []uint{1, 2}
		catalogueIDs := []uint{3, 4}
		dataCatalogueIDs := []uint{5, 6}
		toolCatalogueIDs := []uint{7, 8}

		associations := group.GetAssociationsToUpdate(userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs)

		// Verify no associations need update
		for _, assoc := range associations {
			assert.False(t, assoc.NeedsUpdate, "Association %s should not need update", assoc.Name)
		}

		// Verify values returned by GetValue match expected values
		usersValue := associations[0].GetValue()
		users, ok := usersValue.([]User)
		assert.True(t, ok, "GetValue for Users should return []User")
		assert.Len(t, users, 2)
		assert.Equal(t, uint(1), users[0].ID)
		assert.Equal(t, uint(2), users[1].ID)
	})

	t.Run("All associations need update", func(t *testing.T) {
		// Prepare a group with initial associations
		group := &Group{
			Users: []User{
				{ID: 1}, {ID: 2},
			},
			Catalogues: []Catalogue{
				{ID: 3}, {ID: 4},
			},
			DataCatalogues: []DataCatalogue{
				{ID: 5}, {ID: 6},
			},
			ToolCatalogues: []ToolCatalogue{
				{ID: 7}, {ID: 8},
			},
		}

		userIDs := []uint{1, 2, 9}
		catalogueIDs := []uint{10, 11}
		dataCatalogueIDs := []uint{12}
		toolCatalogueIDs := []uint{13, 14, 15}

		associations := group.GetAssociationsToUpdate(userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs)

		// Verify all associations need update
		for _, assoc := range associations {
			assert.True(t, assoc.NeedsUpdate, "Association %s should need update", assoc.Name)
		}

		// Verify the group's associations have been updated
		assert.Len(t, group.Users, 3)
		assert.Len(t, group.Catalogues, 2)
		assert.Len(t, group.DataCatalogues, 1)
		assert.Len(t, group.ToolCatalogues, 3)
	})

	t.Run("Partial updates needed", func(t *testing.T) {
		// Prepare a group with initial associations
		group := &Group{
			Users: []User{
				{ID: 1}, {ID: 2},
			},
			Catalogues: []Catalogue{
				{ID: 3}, {ID: 4},
			},
			DataCatalogues: []DataCatalogue{
				{ID: 5}, {ID: 6},
			},
			ToolCatalogues: []ToolCatalogue{
				{ID: 7}, {ID: 8},
			},
		}

		// Only change Users and DataCatalogues
		userIDs := []uint{1, 2, 3}       // Added 3
		catalogueIDs := []uint{3, 4}     // No change
		dataCatalogueIDs := []uint{5}    // Removed 6
		toolCatalogueIDs := []uint{7, 8} // No change

		associations := group.GetAssociationsToUpdate(userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs)

		// Users and DataCatalogues need update, others don't
		needsUpdateMap := map[string]bool{
			"Users":          true,
			"Catalogues":     false,
			"DataCatalogues": true,
			"ToolCatalogues": false,
		}

		for _, assoc := range associations {
			expectedNeedsUpdate, exists := needsUpdateMap[assoc.Name]
			assert.True(t, exists, "Unexpected association name: %s", assoc.Name)
			assert.Equal(t, expectedNeedsUpdate, assoc.NeedsUpdate,
				"Association %s has incorrect NeedsUpdate value", assoc.Name)
		}

		// Verify the updated associations
		assert.Len(t, group.Users, 3)
		assert.Len(t, group.Catalogues, 2)
		assert.Len(t, group.DataCatalogues, 1)
		assert.Len(t, group.ToolCatalogues, 2)
	})

	t.Run("Empty to non-empty transitions", func(t *testing.T) {
		// Prepare a group with empty associations
		group := &Group{
			Users:          []User{},
			Catalogues:     []Catalogue{},
			DataCatalogues: []DataCatalogue{},
			ToolCatalogues: []ToolCatalogue{},
		}

		// Add items to all associations
		userIDs := []uint{1, 2}
		catalogueIDs := []uint{3, 4}
		dataCatalogueIDs := []uint{5}
		toolCatalogueIDs := []uint{6, 7}

		associations := group.GetAssociationsToUpdate(userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs)

		// All associations should need update
		for _, assoc := range associations {
			assert.True(t, assoc.NeedsUpdate, "Association %s should need update", assoc.Name)
		}

		// Verify lengths
		assert.Len(t, group.Users, 2)
		assert.Len(t, group.Catalogues, 2)
		assert.Len(t, group.DataCatalogues, 1)
		assert.Len(t, group.ToolCatalogues, 2)
	})

	t.Run("Non-empty to empty transitions", func(t *testing.T) {
		// Prepare a group with non-empty associations
		group := &Group{
			Users: []User{
				{ID: 1}, {ID: 2},
			},
			Catalogues: []Catalogue{
				{ID: 3}, {ID: 4},
			},
			DataCatalogues: []DataCatalogue{
				{ID: 5},
			},
			ToolCatalogues: []ToolCatalogue{
				{ID: 6}, {ID: 7},
			},
		}

		// Empty all associations
		userIDs := []uint{}
		catalogueIDs := []uint{}
		dataCatalogueIDs := []uint{}
		toolCatalogueIDs := []uint{}

		associations := group.GetAssociationsToUpdate(userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs)

		// All associations should need update
		for _, assoc := range associations {
			assert.True(t, assoc.NeedsUpdate, "Association %s should need update", assoc.Name)
		}

		// Verify all associations are empty
		assert.Len(t, group.Users, 0)
		assert.Len(t, group.Catalogues, 0)
		assert.Len(t, group.DataCatalogues, 0)
		assert.Len(t, group.ToolCatalogues, 0)
	})

	t.Run("Same elements in different order", func(t *testing.T) {
		// Prepare a group with ordered associations
		group := &Group{
			Users: []User{
				{ID: 1}, {ID: 2}, {ID: 3},
			},
			Catalogues: []Catalogue{
				{ID: 4}, {ID: 5}, {ID: 6},
			},
		}

		// Same elements but different order
		userIDs := []uint{3, 1, 2}
		catalogueIDs := []uint{6, 5, 4}

		// Keep data and tool catalogues empty
		dataCatalogueIDs := []uint{}
		toolCatalogueIDs := []uint{}

		associations := group.GetAssociationsToUpdate(userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs)

		// Users and Catalogues should not need update despite different order
		// DataCatalogues and ToolCatalogues should remain empty
		needsUpdateMap := map[string]bool{
			"Users":          false,
			"Catalogues":     false,
			"DataCatalogues": false,
			"ToolCatalogues": false,
		}

		for _, assoc := range associations {
			expectedNeedsUpdate, exists := needsUpdateMap[assoc.Name]
			assert.True(t, exists, "Unexpected association name: %s", assoc.Name)
			assert.Equal(t, expectedNeedsUpdate, assoc.NeedsUpdate,
				"Association %s has incorrect NeedsUpdate value", assoc.Name)
		}
	})

	t.Run("With duplicate IDs in input", func(t *testing.T) {
		// Prepare a group with initial associations
		group := &Group{
			Users: []User{
				{ID: 1}, {ID: 2},
			},
			Catalogues: []Catalogue{
				{ID: 3}, {ID: 4},
			},
		}

		// Input with duplicate IDs
		userIDs := []uint{1, 2, 2}   // Duplicate 2
		catalogueIDs := []uint{3, 4} // No duplicates

		// Keep data and tool catalogues empty
		dataCatalogueIDs := []uint{}
		toolCatalogueIDs := []uint{}

		associations := group.GetAssociationsToUpdate(userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs)

		// Users should need update because of the duplicate
		assert.True(t, associations[0].NeedsUpdate, "Users association should need update due to duplicate ID")

		// After parsing, Users should have 3 entries (duplicate preserved)
		assert.Len(t, group.Users, 3)

		// Check IDs to confirm duplicates were preserved
		usersValue := associations[0].GetValue()
		users, ok := usersValue.([]User)
		assert.True(t, ok, "GetValue for Users should return []User")
		assert.Equal(t, uint(1), users[0].ID)
		assert.Equal(t, uint(2), users[1].ID)
		assert.Equal(t, uint(2), users[2].ID)
	})

	t.Run("GetValue returns correct interface types", func(t *testing.T) {
		group := &Group{
			Users:          []User{{ID: 1}},
			Catalogues:     []Catalogue{{ID: 2}},
			DataCatalogues: []DataCatalogue{{ID: 3}},
			ToolCatalogues: []ToolCatalogue{{ID: 4}},
		}

		associations := group.GetAssociationsToUpdate(
			[]uint{1},
			[]uint{2},
			[]uint{3},
			[]uint{4},
		)

		// Check that the GetValue functions return the expected types
		usersValue := associations[0].GetValue()
		_, ok := usersValue.([]User)
		assert.True(t, ok, "GetValue for Users should return []User")

		cataloguesValue := associations[1].GetValue()
		_, ok = cataloguesValue.([]Catalogue)
		assert.True(t, ok, "GetValue for Catalogues should return []Catalogue")

		dataCataloguesValue := associations[2].GetValue()
		_, ok = dataCataloguesValue.([]DataCatalogue)
		assert.True(t, ok, "GetValue for DataCatalogues should return []DataCatalogue")

		toolCataloguesValue := associations[3].GetValue()
		_, ok = toolCataloguesValue.([]ToolCatalogue)
		assert.True(t, ok, "GetValue for ToolCatalogues should return []ToolCatalogue")
	})
}

func TestClearAssociations(t *testing.T) {
	db := setupTestDB(t)

	// Create test data
	group := &Group{Name: "Test Group"}
	err := group.Create(db)
	assert.NoError(t, err)

	// Create and associate users
	user1 := &User{Email: "user1@example.com"}
	user2 := &User{Email: "user2@example.com"}
	err = user1.Create(db)
	assert.NoError(t, err)
	err = user2.Create(db)
	assert.NoError(t, err)
	err = group.AddUser(db, user1)
	assert.NoError(t, err)
	err = group.AddUser(db, user2)
	assert.NoError(t, err)

	// Create and associate catalogues
	catalogue := &Catalogue{Name: "Test Catalogue"}
	err = catalogue.Create(db)
	assert.NoError(t, err)
	err = group.AddCatalogue(db, catalogue)
	assert.NoError(t, err)

	// Create and associate data catalogues
	dataCatalogue := &DataCatalogue{Name: "Test Data Catalogue"}
	err = dataCatalogue.Create(db)
	assert.NoError(t, err)
	err = group.AddDataCatalogue(db, dataCatalogue)
	assert.NoError(t, err)

	// Create and associate tool catalogues
	toolCatalogue := &ToolCatalogue{Name: "Test Tool Catalogue"}
	err = toolCatalogue.Create(db)
	assert.NoError(t, err)
	err = group.AddToolCatalogue(db, toolCatalogue)
	assert.NoError(t, err)

	// Verify associations exist
	err = group.GetGroupUsers(db)
	assert.NoError(t, err)
	assert.Len(t, group.Users, 2)

	err = group.GetCatalogues(db)
	assert.NoError(t, err)
	assert.Len(t, group.Catalogues, 1)

	err = group.GetDataCatalogues(db)
	assert.NoError(t, err)
	assert.Len(t, group.DataCatalogues, 1)

	_, _, err = group.GetToolCatalogues(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, group.ToolCatalogues, 1)

	// Clear all associations
	err = group.ClearAssociations(db)
	assert.NoError(t, err)

	// Verify all associations were cleared
	err = group.GetGroupUsers(db)
	assert.NoError(t, err)
	assert.Len(t, group.Users, 0)

	err = group.GetCatalogues(db)
	assert.NoError(t, err)
	assert.Len(t, group.Catalogues, 0)

	err = group.GetDataCatalogues(db)
	assert.NoError(t, err)
	assert.Len(t, group.DataCatalogues, 0)

	_, _, err = group.GetToolCatalogues(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, group.ToolCatalogues, 0)
}
