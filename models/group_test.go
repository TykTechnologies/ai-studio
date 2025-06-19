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
	_, _, err := fetchedGroups.GetAll(db, 10, 1, true, "id")
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
			assert.Equal(t, needsUpdateMap[assoc.Name], assoc.NeedsUpdate, "Association %s should have NeedsUpdate=%v", assoc.Name, needsUpdateMap[assoc.Name])
		}
	})
}

func TestClearAssociations(t *testing.T) {
	db := setupTestDB(t)

	// Create test data
	group := &Group{Name: "Test Group"}
	err := group.Create(db)
	assert.NoError(t, err)

	user := &User{Name: "Test User", Email: "test@example.com"}
	err = user.Create(db)
	assert.NoError(t, err)

	catalogue := &Catalogue{Name: "Test Catalogue"}
	err = catalogue.Create(db)
	assert.NoError(t, err)

	dataCatalogue := &DataCatalogue{Name: "Test Data Catalogue"}
	err = dataCatalogue.Create(db)
	assert.NoError(t, err)

	toolCatalogue := &ToolCatalogue{Name: "Test Tool Catalogue"}
	err = toolCatalogue.Create(db)
	assert.NoError(t, err)

	// Add associations
	err = group.AddUser(db, user)
	assert.NoError(t, err)

	err = group.AddCatalogue(db, catalogue)
	assert.NoError(t, err)

	err = group.AddDataCatalogue(db, dataCatalogue)
	assert.NoError(t, err)

	err = group.AddToolCatalogue(db, toolCatalogue)
	assert.NoError(t, err)

	// Verify associations exist
	err = group.GetGroupUsers(db)
	assert.NoError(t, err)
	assert.Len(t, group.Users, 1)

	err = group.GetCatalogues(db)
	assert.NoError(t, err)
	assert.Len(t, group.Catalogues, 1)

	err = group.GetDataCatalogues(db)
	assert.NoError(t, err)
	assert.Len(t, group.DataCatalogues, 1)

	_, _, err = group.GetToolCatalogues(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, group.ToolCatalogues, 1)

	// Clear associations
	err = group.ClearAssociations(db)
	assert.NoError(t, err)

	// Verify associations were cleared
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

func TestGroups_SearchByTerm(t *testing.T) {
	db := setupTestDB(t)

	// Create test groups
	groups := []Group{
		{Name: "DevOps Team"},
		{Name: "Frontend Developers"},
		{Name: "Backend Team"},
		{Name: "QA Engineers"},
		{Name: "Development Leadership"},
	}
	for _, g := range groups {
		err := db.Create(&g).Error
		assert.NoError(t, err)
	}

	t.Run("Search with specific term", func(t *testing.T) {
		var fetchedGroups Groups
		totalCount, totalPages, err := fetchedGroups.SearchByTerm(db, "Dev", 10, 1, true, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(3), totalCount)
		assert.Equal(t, 1, totalPages)
		assert.Len(t, fetchedGroups, 3)

		// Check that the correct groups were found
		foundNames := make([]string, len(fetchedGroups))
		for i, g := range fetchedGroups {
			foundNames[i] = g.Name
		}
		assert.Contains(t, foundNames, "DevOps Team")
		assert.Contains(t, foundNames, "Frontend Developers")
		assert.Contains(t, foundNames, "Development Leadership")
	})

	t.Run("Search with empty term", func(t *testing.T) {
		var fetchedGroups Groups
		totalCount, totalPages, err := fetchedGroups.SearchByTerm(db, "", 10, 1, true, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 1, totalPages)
		assert.Len(t, fetchedGroups, 5)
	})

	t.Run("Search with pagination", func(t *testing.T) {
		var fetchedGroups Groups
		totalCount, totalPages, err := fetchedGroups.SearchByTerm(db, "Team", 2, 1, false, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(2), totalCount)
		assert.Equal(t, 1, totalPages)
		assert.Len(t, fetchedGroups, 2)

		// Check that pagination works - second page
		fetchedGroups = Groups{}
		totalCount, totalPages, err = fetchedGroups.SearchByTerm(db, "", 2, 2, false, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 3, totalPages)
		assert.Len(t, fetchedGroups, 2)
	})

	t.Run("Search with sorting", func(t *testing.T) {
		// Test ascending order
		var fetchedGroups Groups
		_, _, err := fetchedGroups.SearchByTerm(db, "", 10, 1, true, "name")
		assert.NoError(t, err)
		assert.Len(t, fetchedGroups, 5)

		names := []string{}
		for _, g := range fetchedGroups {
			names = append(names, g.Name)
		}
		assert.Contains(t, names, "Backend Team")
		assert.Contains(t, names, "Development Leadership")
		assert.Contains(t, names, "DevOps Team")
		assert.Contains(t, names, "Frontend Developers")
		assert.Contains(t, names, "QA Engineers")

		// Test descending order - just check we get all groups
		fetchedGroups = Groups{}
		_, _, err = fetchedGroups.SearchByTerm(db, "", 10, 1, true, "-name")
		assert.NoError(t, err)
		assert.Len(t, fetchedGroups, 5)
	})

	t.Run("Search with case insensitivity", func(t *testing.T) {
		// Test lowercase search term
		var fetchedGroups Groups
		totalCount, _, err := fetchedGroups.SearchByTerm(db, "devops", 10, 1, true, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(1), totalCount)
		assert.Len(t, fetchedGroups, 1)
		assert.Equal(t, "DevOps Team", fetchedGroups[0].Name)

		// Test uppercase search term
		fetchedGroups = Groups{}
		totalCount, _, err = fetchedGroups.SearchByTerm(db, "FRONTEND", 10, 1, true, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(1), totalCount)
		assert.Len(t, fetchedGroups, 1)
		assert.Equal(t, "Frontend Developers", fetchedGroups[0].Name)

		// Test mixed case search term
		fetchedGroups = Groups{}
		totalCount, _, err = fetchedGroups.SearchByTerm(db, "LeAdErShIp", 10, 1, true, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(1), totalCount)
		assert.Len(t, fetchedGroups, 1)
		assert.Equal(t, "Development Leadership", fetchedGroups[0].Name)
	})

	t.Run("Search with preloads", func(t *testing.T) {
		// Create a user and add to a group
		user := &User{Name: "Test User", Email: "test@example.com"}
		err := user.Create(db)
		assert.NoError(t, err)

		group := &Group{}
		err = db.First(group, "name = ?", "DevOps Team").Error
		assert.NoError(t, err)

		err = group.AddUser(db, user)
		assert.NoError(t, err)

		// Search with preload
		var fetchedGroups Groups
		_, _, err = fetchedGroups.SearchByTerm(db, "DevOps", 10, 1, true, "name", "Users")
		assert.NoError(t, err)
		assert.Len(t, fetchedGroups, 1)
		assert.Equal(t, "DevOps Team", fetchedGroups[0].Name)
		assert.Len(t, fetchedGroups[0].Users, 1)
		assert.Equal(t, user.ID, fetchedGroups[0].Users[0].ID)
	})
}

func TestGroups_GetGroupsMemberCounts(t *testing.T) {
	db := setupTestDB(t)

	// Create test groups
	groups := []Group{
		{Name: "Group 1"},
		{Name: "Group 2"},
		{Name: "Group 3"},
	}
	for i := range groups {
		err := db.Create(&groups[i]).Error
		assert.NoError(t, err)
	}

	// Create test users
	users := []User{
		{Name: "User 1", Email: "user1@example.com"},
		{Name: "User 2", Email: "user2@example.com"},
		{Name: "User 3", Email: "user3@example.com"},
		{Name: "User 4", Email: "user4@example.com"},
	}
	for i := range users {
		err := db.Create(&users[i]).Error
		assert.NoError(t, err)
	}

	// Add users to groups with different counts
	// Group 1: 3 users
	// Group 2: 1 user
	// Group 3: 0 users
	err := db.Model(&groups[0]).Association("Users").Append(&users[0], &users[1], &users[2])
	assert.NoError(t, err)

	err = db.Model(&groups[1]).Association("Users").Append(&users[3])
	assert.NoError(t, err)

	// Fetch the groups with their IDs
	var fetchedGroups Groups
	err = db.Find(&fetchedGroups).Error
	assert.NoError(t, err)
	assert.Len(t, fetchedGroups, 3)

	// Test GetGroupsMemberCounts
	memberCounts, err := fetchedGroups.GetGroupsMemberCounts(db)
	assert.NoError(t, err)
	assert.Len(t, memberCounts, 2) // Only groups with members are returned

	// Create a map to easily check counts by group ID
	countMap := make(map[uint]int64)
	for _, mc := range memberCounts {
		countMap[mc.GroupID] = mc.Count
	}

	// Verify counts
	assert.Equal(t, int64(3), countMap[groups[0].ID])
	assert.Equal(t, int64(1), countMap[groups[1].ID])
	assert.NotContains(t, countMap, groups[2].ID) // Group 3 has no members

	err = db.Delete(&users[1]).Error
	assert.NoError(t, err)
	err = db.Delete(&users[2]).Error
	assert.NoError(t, err)

	var associationCount int64
	err = db.Table("user_groups").Where("group_id = ? AND user_id IN ?", groups[0].ID, []uint{users[1].ID, users[2].ID}).Count(&associationCount).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(2), associationCount)

	memberCounts, err = fetchedGroups.GetGroupsMemberCounts(db)
	assert.NoError(t, err)

	countMap = make(map[uint]int64)
	for _, mc := range memberCounts {
		countMap[mc.GroupID] = mc.Count
	}

	assert.Equal(t, int64(1), countMap[groups[0].ID])
	assert.Equal(t, int64(1), countMap[groups[1].ID])
	assert.NotContains(t, countMap, groups[2].ID)
}

func TestGroup_GetMembersCount(t *testing.T) {
	// Create a group with users
	group := &Group{
		ID: 1,
		Users: []User{
			{ID: 101}, {ID: 102}, {ID: 103},
		},
	}

	// Create member counts
	memberCounts := []GroupMemberCount{
		{GroupID: 1, Count: 5},  // This count should be used instead of len(Users)
		{GroupID: 2, Count: 10}, // Different group
	}

	t.Run("Get count from memberCounts", func(t *testing.T) {
		count := group.GetMembersCount(memberCounts)
		assert.Equal(t, 5, count) // Should use the count from memberCounts
	})

	t.Run("Fallback to Users length", func(t *testing.T) {
		// New group with no matching memberCount
		newGroup := &Group{
			ID: 3, // Not in memberCounts
			Users: []User{
				{ID: 101}, {ID: 102},
			},
		}
		count := newGroup.GetMembersCount(memberCounts)
		assert.Equal(t, 2, count) // Should use the length of Users
	})

	t.Run("Empty memberCounts", func(t *testing.T) {
		count := group.GetMembersCount([]GroupMemberCount{})
		assert.Equal(t, 3, count) // Should use the length of Users
	})
}

func TestGroup_GetCataloguesCount(t *testing.T) {
	t.Run("Group with catalogues", func(t *testing.T) {
		group := &Group{
			Catalogues: []Catalogue{
				{ID: 1}, {ID: 2}, {ID: 3},
			},
		}
		count := group.GetCataloguesCount()
		assert.Equal(t, 3, count)
	})

	t.Run("Group without catalogues", func(t *testing.T) {
		group := &Group{
			Catalogues: []Catalogue{},
		}
		count := group.GetCataloguesCount()
		assert.Equal(t, 0, count)
	})
}

func TestGroup_GetDataCataloguesCount(t *testing.T) {
	t.Run("Group with data catalogues", func(t *testing.T) {
		group := &Group{
			DataCatalogues: []DataCatalogue{
				{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4},
			},
		}
		count := group.GetDataCataloguesCount()
		assert.Equal(t, 4, count)
	})

	t.Run("Group without data catalogues", func(t *testing.T) {
		group := &Group{
			DataCatalogues: []DataCatalogue{},
		}
		count := group.GetDataCataloguesCount()
		assert.Equal(t, 0, count)
	})
}

func TestGroup_GetToolCataloguesCount(t *testing.T) {
	t.Run("Group with tool catalogues", func(t *testing.T) {
		group := &Group{
			ToolCatalogues: []ToolCatalogue{
				{ID: 1}, {ID: 2},
			},
		}
		count := group.GetToolCataloguesCount()
		assert.Equal(t, 2, count)
	})

	t.Run("Group without tool catalogues", func(t *testing.T) {
		group := &Group{
			ToolCatalogues: []ToolCatalogue{},
		}
		count := group.GetToolCataloguesCount()
		assert.Equal(t, 0, count)
	})
}
func TestIsGroupNameUnique(t *testing.T) {
	db := setupTestDB(t)

	group1 := &Group{Name: "Existing Group 1"}
	err := group1.Create(db)
	assert.NoError(t, err)

	group2 := &Group{Name: "Existing Group 2"}
	err = group2.Create(db)
	assert.NoError(t, err)

	isUnique, err := IsGroupNameUnique(db, "New Unique Group", 0)
	assert.NoError(t, err)
	assert.True(t, isUnique)

	isUnique, err = IsGroupNameUnique(db, "Existing Group 1", 0)
	assert.NoError(t, err)
	assert.False(t, isUnique)

	isUnique, err = IsGroupNameUnique(db, "Existing Group 1", group1.ID)
	assert.NoError(t, err)
	assert.True(t, isUnique)

	isUnique, err = IsGroupNameUnique(db, "Existing Group 1", group2.ID)
	assert.NoError(t, err)
	assert.False(t, isUnique)
}
