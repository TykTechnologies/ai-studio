package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/stretchr/testify/assert"
)

func TestSearchGroups(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create test groups
	groupNames := []string{
		"Admin Team",
		"Development Team",
		"DevOps Team",
		"Product Management",
		"Quality Assurance",
	}

	for _, name := range groupNames {
		_, err := service.CreateGroup(name, []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)
	}

	t.Run("SearchGroups with specific term", func(t *testing.T) {
		// Search for groups with "Dev" in their name
		groups, totalCount, totalPages, err := service.SearchGroups("Dev", 10, 1, true, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(2), totalCount) // "Development Team" and "DevOps Team"
		assert.Equal(t, 1, totalPages)
		assert.Len(t, groups, 2)

		// Verify correct groups were found
		groupNames := []string{groups[0].Name, groups[1].Name}
		assert.Contains(t, groupNames, "Development Team")
		assert.Contains(t, groupNames, "DevOps Team")
	})

	t.Run("SearchGroups with empty term", func(t *testing.T) {
		// Empty term should return all groups
		groups, totalCount, totalPages, err := service.SearchGroups("", 10, 1, true, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 1, totalPages)
		assert.Len(t, groups, 5)
	})

	t.Run("SearchGroups with pagination", func(t *testing.T) {
		// First page with limit of 2
		groups, totalCount, totalPages, err := service.SearchGroups("", 2, 1, false, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 3, totalPages) // 5 items / 2 per page = 3 pages
		assert.Len(t, groups, 2)

		// Second page
		groups, totalCount, totalPages, err = service.SearchGroups("", 2, 2, false, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 3, totalPages)
		assert.Len(t, groups, 2)

		// Third page should have only 1 item
		groups, totalCount, totalPages, err = service.SearchGroups("", 2, 3, false, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 3, totalPages)
		assert.Len(t, groups, 1)
	})

	t.Run("SearchGroups with sorting", func(t *testing.T) {
		// Test ascending order
		groups, _, _, err := service.SearchGroups("", 10, 1, true, "name")
		assert.NoError(t, err)
		assert.Len(t, groups, 5)
		assert.Equal(t, "Admin Team", groups[0].Name) // Alphabetically first

		// Test descending order
		groups, _, _, err = service.SearchGroups("", 10, 1, true, "-name")
		assert.NoError(t, err)
		assert.Len(t, groups, 5)
		assert.Equal(t, "Quality Assurance", groups[0].Name) // Alphabetically last
	})

	t.Run("SearchGroups with non-existent term", func(t *testing.T) {
		groups, totalCount, totalPages, err := service.SearchGroups("NonExistentTerm", 10, 1, true, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(0), totalCount)
		assert.Equal(t, 0, totalPages)
		assert.Len(t, groups, 0)
	})
}

func TestGetGroupsWithMemberCounts(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create test groups
	group1, err := service.CreateGroup("Team Alpha", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)

	group2, err := service.CreateGroup("Team Beta", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)

	group3, err := service.CreateGroup("Team Gamma", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)

	// Create test users
	user1, err := service.CreateUser("user1@example.com", "User One", "password123", true, true, true, true, true, true)
	assert.NoError(t, err)

	user2, err := service.CreateUser("user2@example.com", "User Two", "password123", true, true, true, true, true, true)
	assert.NoError(t, err)

	user3, err := service.CreateUser("user3@example.com", "User Three", "password123", true, true, true, true, true, true)
	assert.NoError(t, err)

	user4, err := service.CreateUser("user4@example.com", "User Four", "password123", true, true, true, true, true, true)
	assert.NoError(t, err)

	// Add users to groups
	// Team Alpha: 3 users
	err = service.AddUserToGroup(user1.ID, group1.ID)
	assert.NoError(t, err)
	err = service.AddUserToGroup(user2.ID, group1.ID)
	assert.NoError(t, err)
	err = service.AddUserToGroup(user3.ID, group1.ID)
	assert.NoError(t, err)

	// Team Beta: 1 user
	err = service.AddUserToGroup(user4.ID, group2.ID)
	assert.NoError(t, err)

	// Team Gamma: 0 users - leave empty

	t.Run("GetGroupsWithMemberCounts with empty term", func(t *testing.T) {
		groups, memberCounts, totalCount, totalPages, err := service.GetGroupsWithMemberCounts("", 10, 1, true, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(3), totalCount)
		assert.Equal(t, 1, totalPages)
		assert.Len(t, groups, 3)

		// Only groups with members should have memberCounts entries
		assert.Len(t, memberCounts, 2)

		// Create a map for easy lookup by group ID
		countMap := make(map[uint]int64)
		for _, mc := range memberCounts {
			countMap[mc.GroupID] = mc.Count
		}

		// Verify counts
		assert.Equal(t, int64(3), countMap[group1.ID])
		assert.Equal(t, int64(1), countMap[group2.ID])
		assert.NotContains(t, countMap, group3.ID) // No members, so not in results
	})

	t.Run("GetGroupsWithMemberCounts with search term", func(t *testing.T) {
		groups, memberCounts, totalCount, totalPages, err := service.GetGroupsWithMemberCounts("Alpha", 10, 1, true, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(1), totalCount)
		assert.Equal(t, 1, totalPages)
		assert.Len(t, groups, 1)
		assert.Equal(t, "Team Alpha", groups[0].Name)

		// Should have member counts for the one matching group
		assert.Len(t, memberCounts, 1)
		assert.Equal(t, group1.ID, memberCounts[0].GroupID)
		assert.Equal(t, int64(3), memberCounts[0].Count)
	})

	t.Run("GetGroupsWithMemberCounts with pagination", func(t *testing.T) {
		// First page with limit of 2
		groups, memberCounts, totalCount, totalPages, err := service.GetGroupsWithMemberCounts("", 2, 1, false, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(3), totalCount)
		assert.Equal(t, 2, totalPages) // 3 items / 2 per page = 2 pages
		assert.Len(t, groups, 2)

		// Member counts for the two groups with members
		countMap := make(map[uint]int64)
		for _, mc := range memberCounts {
			countMap[mc.GroupID] = mc.Count
		}

		for _, group := range groups {
			if group.ID == group1.ID {
				assert.Equal(t, int64(3), countMap[group1.ID])
			} else if group.ID == group2.ID {
				assert.Equal(t, int64(1), countMap[group2.ID])
			}
		}

		// Second page
		groups, memberCounts, totalCount, totalPages, err = service.GetGroupsWithMemberCounts("", 2, 2, false, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(3), totalCount)
		assert.Equal(t, 2, totalPages)
		assert.Len(t, groups, 1)

		// Should have no member counts for the third group (Team Gamma) which has no members
		countMap = make(map[uint]int64)
		for _, mc := range memberCounts {
			countMap[mc.GroupID] = mc.Count
		}
		assert.NotContains(t, countMap, group3.ID)
	})

	t.Run("GetGroupsWithMemberCounts with non-existent term", func(t *testing.T) {
		groups, memberCounts, totalCount, totalPages, err := service.GetGroupsWithMemberCounts("NonExistentTerm", 10, 1, true, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(0), totalCount)
		assert.Equal(t, 0, totalPages)
		assert.Len(t, groups, 0)
		assert.Len(t, memberCounts, 0)
	})
}

// Test error handling for SearchGroups and GetGroupsWithMemberCounts
func TestGroupService_SearchFunctions_Errors(t *testing.T) {
	// Create a test DB that will be closed to simulate errors
	db := setupTestDB(t)
	service := NewService(db)

	// Create a test group first
	_, err := service.CreateGroup("Error Test Group", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)

	// Close the database connection to simulate errors
	sqlDB, err := db.DB()
	assert.NoError(t, err)
	err = sqlDB.Close()
	assert.NoError(t, err)

	t.Run("SearchGroups returns error on DB failure", func(t *testing.T) {
		groups, totalCount, totalPages, err := service.SearchGroups("Test", 10, 1, true, "name")
		assert.Error(t, err)
		assert.Nil(t, groups)
		assert.Equal(t, int64(0), totalCount)
		assert.Equal(t, 0, totalPages)
	})

	t.Run("GetGroupsWithMemberCounts returns error on DB failure", func(t *testing.T) {
		groups, memberCounts, totalCount, totalPages, err := service.GetGroupsWithMemberCounts("Test", 10, 1, true, "name")
		assert.Error(t, err)
		assert.Nil(t, groups)
		assert.Nil(t, memberCounts)
		assert.Equal(t, int64(0), totalCount)
		assert.Equal(t, 0, totalPages)
	})
}

func TestCreateGroupNameValidation(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	t.Run("CreateGroup with empty name should fail", func(t *testing.T) {
		_, err := service.CreateGroup("", []uint{}, []uint{}, []uint{}, []uint{})
		assert.Error(t, err)
		assert.IsType(t, helpers.ErrorResponse{}, err)
		errorResp := err.(helpers.ErrorResponse)
		assert.Equal(t, 400, errorResp.StatusCode)
		assert.Contains(t, errorResp.Message, "group name is required")
	})

	t.Run("CreateGroup with unique name should succeed", func(t *testing.T) {
		group, err := service.CreateGroup("Unique Group Name", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)
		assert.NotNil(t, group)
		assert.Equal(t, "Unique Group Name", group.Name)
	})

	t.Run("CreateGroup with duplicate name should fail", func(t *testing.T) {
		// First create a group
		_, err := service.CreateGroup("Duplicate Name", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)

		// Try to create another group with the same name
		_, err = service.CreateGroup("Duplicate Name", []uint{}, []uint{}, []uint{}, []uint{})
		assert.Error(t, err)
		assert.IsType(t, helpers.ErrorResponse{}, err)
		errorResp := err.(helpers.ErrorResponse)
		assert.Equal(t, 400, errorResp.StatusCode)
		assert.Contains(t, errorResp.Message, "group name already exists")
	})
}

func TestUpdateGroupNameValidation(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create initial groups
	group1, err := service.CreateGroup("Original Group 1", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)

	group2, err := service.CreateGroup("Original Group 2", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)

	t.Run("UpdateGroup with same name should succeed", func(t *testing.T) {
		// Update group with its current name should not fail
		updatedGroup, err := service.UpdateGroup(group1.ID, "Original Group 1", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)
		assert.Equal(t, "Original Group 1", updatedGroup.Name)
	})

	t.Run("UpdateGroup with unique new name should succeed", func(t *testing.T) {
		updatedGroup, err := service.UpdateGroup(group1.ID, "Updated Unique Name", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)
		assert.Equal(t, "Updated Unique Name", updatedGroup.Name)
	})

	t.Run("UpdateGroup with existing name should fail", func(t *testing.T) {
		// Try to update group1 to have the same name as group2
		_, err := service.UpdateGroup(group1.ID, "Original Group 2", []uint{}, []uint{}, []uint{}, []uint{})
		assert.Error(t, err)
		assert.IsType(t, helpers.ErrorResponse{}, err)
		errorResp := err.(helpers.ErrorResponse)
		assert.Equal(t, 400, errorResp.StatusCode)
		assert.Contains(t, errorResp.Message, "group name already exists")
	})

	t.Run("UpdateGroup with empty name should keep existing name", func(t *testing.T) {
		// When name is empty, it should not change the name
		originalGroup, err := service.GetGroupByID(group2.ID)
		assert.NoError(t, err)
		originalName := originalGroup.Name

		updatedGroup, err := service.UpdateGroup(group2.ID, "", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)
		assert.Equal(t, originalName, updatedGroup.Name)
	})
}
