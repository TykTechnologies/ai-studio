//go:build enterprise

package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForFilters(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestFilterService(t *testing.T) {
	db := setupTestDBForFilters(t)
	service := NewService(db)

	// Test CreateFilter
	filter, err := service.CreateFilter("Test Filter", "Test Description", []byte("test script"), false, "")
	assert.NoError(t, err)
	assert.NotNil(t, filter)
	assert.NotZero(t, filter.ID)
	assert.Equal(t, "Test Filter", filter.Name)
	assert.Equal(t, "Test Description", filter.Description)
	assert.Equal(t, "test script", string(filter.Script))

	// Test GetFilterByID
	fetchedFilter, err := service.GetFilterByID(filter.ID)
	assert.NoError(t, err)
	assert.Equal(t, filter.ID, fetchedFilter.ID)
	assert.Equal(t, filter.Name, fetchedFilter.Name)
	assert.Equal(t, filter.Description, fetchedFilter.Description)
	assert.Equal(t, filter.Script, fetchedFilter.Script)

	// Test UpdateFilter
	updatedFilter, err := service.UpdateFilter(filter.ID, "Updated Filter", "Updated Description", []byte("updated script"), false, "")
	assert.NoError(t, err)
	assert.Equal(t, filter.ID, updatedFilter.ID)
	assert.Equal(t, "Updated Filter", updatedFilter.Name)
	assert.Equal(t, "Updated Description", updatedFilter.Description)
	assert.Equal(t, "updated script", string(updatedFilter.Script))

	// Test GetAllFilters
	filters, _, _, err := service.GetAllFilters(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, filters, 1)
	assert.Equal(t, updatedFilter.ID, filters[0].ID)

	// Test GetFilterByName
	namedFilter, err := service.GetFilterByName("Updated Filter")
	assert.NoError(t, err)
	assert.Equal(t, updatedFilter.ID, namedFilter.ID)

	// Test DeleteFilter
	err = service.DeleteFilter(filter.ID)
	assert.NoError(t, err)

	// Verify filter is deleted
	_, err = service.GetFilterByID(filter.ID)
	assert.Error(t, err)
}

func TestFilterServiceErrorCases(t *testing.T) {
	db := setupTestDBForFilters(t)
	service := NewService(db)

	// Test GetFilterByID with non-existent ID
	_, err := service.GetFilterByID(9999)
	assert.Error(t, err)

	// Test UpdateFilter with non-existent ID
	_, err = service.UpdateFilter(9999, "Non-existent", "Description", []byte("script"), false, "")
	assert.Error(t, err)

	// Test DeleteFilter with non-existent ID
	err = service.DeleteFilter(9999)
	assert.Error(t, err)

	// Test GetFilterByName with non-existent name
	_, err = service.GetFilterByName("Non-existent Filter")
	assert.Error(t, err)
}

func TestFilterService_MultipleFilters(t *testing.T) {
	db := setupTestDBForFilters(t)
	service := NewService(db)

	// Create multiple filters
	filter1, _ := service.CreateFilter("Filter 1", "Description 1", []byte("script 1"), false, "")
	filter2, _ := service.CreateFilter("Filter 2", "Description 2", []byte("script 2"), false, "")
	filter3, _ := service.CreateFilter("Filter 3", "Description 3", []byte("script 3"), false, "")

	// Test GetAllFilters
	allFilters, _, _, err := service.GetAllFilters(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, allFilters, 3)
	assert.ElementsMatch(t, []uint{filter1.ID, filter2.ID, filter3.ID}, []uint{allFilters[0].ID, allFilters[1].ID, allFilters[2].ID})

	// Test updating multiple filters
	_, err = service.UpdateFilter(filter1.ID, "Updated Filter 1", "Updated Description 1", []byte("updated script 1"), false, "")
	assert.NoError(t, err)
	_, err = service.UpdateFilter(filter2.ID, "Updated Filter 2", "Updated Description 2", []byte("updated script 2"), false, "")
	assert.NoError(t, err)

	// Verify updates
	updatedFilters, _, _, err := service.GetAllFilters(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, updatedFilters, 3)
	assert.Contains(t, []string{"Updated Filter 1", "Updated Filter 2", "Filter 3"}, updatedFilters[0].Name)
	assert.Contains(t, []string{"Updated Filter 1", "Updated Filter 2", "Filter 3"}, updatedFilters[1].Name)
	assert.Contains(t, []string{"Updated Filter 1", "Updated Filter 2", "Filter 3"}, updatedFilters[2].Name)

	// Test deleting multiple filters
	err = service.DeleteFilter(filter1.ID)
	assert.NoError(t, err)
	err = service.DeleteFilter(filter2.ID)
	assert.NoError(t, err)

	// Verify deletions
	remainingFilters, _, _, err := service.GetAllFilters(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, remainingFilters, 1)
	assert.Equal(t, filter3.ID, remainingFilters[0].ID)
}
