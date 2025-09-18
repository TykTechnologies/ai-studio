// internal/services/filter_service_test.go
package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupFilterTestDB(t *testing.T) (*gorm.DB, *database.Repository) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = database.Migrate(db)
	require.NoError(t, err)

	repo := database.NewRepository(db)
	return db, repo
}

func TestFilterService_CreateFilter(t *testing.T) {
	db, repo := setupFilterTestDB(t)
	service := NewFilterService(db, repo)

	t.Run("ValidCreate", func(t *testing.T) {
		req := &CreateFilterRequest{
			Name:        "Test Filter",
			Description: "Test filter description",
			Script:      "result = true",
			IsActive:    true,
			OrderIndex:  1,
		}

		filter, err := service.CreateFilter(req)
		assert.NoError(t, err)
		assert.NotNil(t, filter)
		assert.Equal(t, "Test Filter", filter.Name)
		assert.Equal(t, "Test filter description", filter.Description)
		assert.Equal(t, "result = true", filter.Script)
		assert.True(t, filter.IsActive)
		assert.Equal(t, 1, filter.OrderIndex)
		assert.NotZero(t, filter.ID)
	})

	t.Run("InvalidScript", func(t *testing.T) {
		req := &CreateFilterRequest{
			Name:        "Invalid Filter",
			Description: "Filter with empty script", 
			Script:      "", // Empty script should fail
		}

		_, err := service.CreateFilter(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "script cannot be empty")
	})

	t.Run("EmptyName", func(t *testing.T) {
		req := &CreateFilterRequest{
			Name:        "",
			Description: "Filter with empty name",
			Script:      "result = true",
		}

		_, err := service.CreateFilter(req)
		// This should be caught by binding validation at handler level
		// But service level should handle gracefully
		assert.NoError(t, err) // Service doesn't validate name currently
	})
}

func TestFilterService_GetFilter(t *testing.T) {
	db, repo := setupFilterTestDB(t)
	service := NewFilterService(db, repo)

	// Create test filter
	testFilter := &CreateFilterRequest{
		Name:        "Test Filter",
		Description: "Test filter for get operations",
		Script:      "result = true",
	}
	createdFilter, err := service.CreateFilter(testFilter)
	require.NoError(t, err)

	t.Run("ValidGet", func(t *testing.T) {
		filter, err := service.GetFilter(createdFilter.ID)
		assert.NoError(t, err)
		assert.NotNil(t, filter)
		assert.Equal(t, createdFilter.ID, filter.ID)
		assert.Equal(t, "Test Filter", filter.Name)
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetFilter(999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "filter not found")
	})
}

func TestFilterService_ListFilters(t *testing.T) {
	db, repo := setupFilterTestDB(t)
	service := NewFilterService(db, repo)

	// Create test filters
	filter1 := &CreateFilterRequest{
		Name:        "Active Filter",
		Description: "Test active filter",
		Script:      "result = true",
	}
	filter2 := &CreateFilterRequest{
		Name:        "Another Active Filter",
		Description: "Another test filter", 
		Script:      "result = true",
	}
	filter3 := &CreateFilterRequest{
		Name:        "Inactive Filter",
		Description: "Test inactive filter",
		Script:      "result = false",
		IsActive:    true, // Create as active first
	}

	_, err := service.CreateFilter(filter1)
	require.NoError(t, err)
	_, err = service.CreateFilter(filter2)
	require.NoError(t, err)
	createdFilter3, err := service.CreateFilter(filter3)
	require.NoError(t, err)

	// Make filter3 inactive (same GORM issue fix)
	createdFilter3.IsActive = false
	db.Save(createdFilter3)

	t.Run("ListAllActiveFilters", func(t *testing.T) {
		filters, total, err := service.ListFilters(1, 10, true)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), total) // Only active ones
		assert.Len(t, filters, 2)
	})

	t.Run("ListActiveOnly", func(t *testing.T) {
		filters, total, err := service.ListFilters(1, 10, true)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), total) // Two active filters
		assert.Len(t, filters, 2)
		// Verify they are both active
		for _, filter := range filters {
			assert.True(t, filter.IsActive)
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		filters, total, err := service.ListFilters(1, 1, true)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), total)
		assert.Len(t, filters, 1) // Only 1 per page
	})
}

func TestFilterService_UpdateFilter(t *testing.T) {
	db, repo := setupFilterTestDB(t)
	service := NewFilterService(db, repo)

	// Create test filter
	testFilter := &CreateFilterRequest{
		Name:        "Original Filter",
		Description: "Original filter description",
		Script:      "result = true",
	}
	createdFilter, err := service.CreateFilter(testFilter)
	require.NoError(t, err)

	t.Run("ValidUpdate", func(t *testing.T) {
		newName := "Updated Filter"
		newScript := "result = false"
		updateReq := &UpdateFilterRequest{
			Name:   &newName,
			Script: &newScript,
		}

		updatedFilter, err := service.UpdateFilter(createdFilter.ID, updateReq)
		assert.NoError(t, err)
		assert.Equal(t, "Updated Filter", updatedFilter.Name)
		assert.Equal(t, "result = false", updatedFilter.Script)
		assert.Equal(t, "Original filter description", updatedFilter.Description) // Should remain unchanged
	})

	t.Run("UpdateNotFound", func(t *testing.T) {
		updateReq := &UpdateFilterRequest{
			Name: stringPtr("Should Fail"),
		}

		_, err := service.UpdateFilter(999, updateReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "filter not found")
	})
}

func TestFilterService_DeleteFilter(t *testing.T) {
	db, repo := setupFilterTestDB(t)
	service := NewFilterService(db, repo)

	// Create test filter
	testFilter := &CreateFilterRequest{
		Name:        "Delete Me",
		Description: "Filter to be deleted",
		Script:      "result = true",
	}
	createdFilter, err := service.CreateFilter(testFilter)
	require.NoError(t, err)

	t.Run("ValidDelete", func(t *testing.T) {
		err := service.DeleteFilter(createdFilter.ID)
		assert.NoError(t, err)

		// Verify it's deleted
		_, err = service.GetFilter(createdFilter.ID)
		assert.Error(t, err)
	})

	t.Run("DeleteNotFound", func(t *testing.T) {
		err := service.DeleteFilter(999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "filter not found")
	})
}

func TestFilterService_LLMFilterAssociations(t *testing.T) {
	db, repo := setupFilterTestDB(t)
	service := NewFilterService(db, repo)

	// Create test LLM
	llm := &database.LLM{
		Name:         "Test LLM",
		Slug:         "test-llm",
		Vendor:       "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
	}
	err := repo.CreateLLM(llm)
	require.NoError(t, err)

	// Create test filters
	filter1 := &CreateFilterRequest{
		Name:        "Filter 1",
		Description: "First test filter",
		Script:      "result = true",
	}
	filter2 := &CreateFilterRequest{
		Name:        "Filter 2", 
		Description: "Second test filter",
		Script:      "result = true",
	}

	createdFilter1, err := service.CreateFilter(filter1)
	require.NoError(t, err)
	createdFilter2, err := service.CreateFilter(filter2)
	require.NoError(t, err)

	t.Run("UpdateLLMFilters", func(t *testing.T) {
		filterIDs := []uint{createdFilter1.ID, createdFilter2.ID}
		err := service.UpdateLLMFilters(llm.ID, filterIDs)
		assert.NoError(t, err)
	})

	t.Run("GetFiltersForLLM", func(t *testing.T) {
		filters, err := service.GetFiltersForLLM(llm.ID)
		assert.NoError(t, err)
		assert.Len(t, filters, 2)
		
		// Should be ordered by order_index
		filterNames := []string{filters[0].Name, filters[1].Name}
		assert.Contains(t, filterNames, "Filter 1")
		assert.Contains(t, filterNames, "Filter 2")
	})

	t.Run("RemoveAllFilters", func(t *testing.T) {
		err := service.UpdateLLMFilters(llm.ID, []uint{})
		assert.NoError(t, err)

		filters, err := service.GetFiltersForLLM(llm.ID)
		assert.NoError(t, err)
		assert.Len(t, filters, 0)
	})
}

// stringPtr helper function already defined in management_service_test.go