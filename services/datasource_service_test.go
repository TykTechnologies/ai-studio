package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForDatasources(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestDatasourceService(t *testing.T) {
	db := setupTestDBForDatasources(t)
	service := NewService(db)

	// Create a user for testing
	user, err := service.CreateUser("test@example.com", "Test User", "password123")
	assert.NoError(t, err)

	// Test CreateDatasource
	datasource, err := service.CreateDatasource("Test Datasource", "Short Desc", "Long Desc", "icon.png", "https://example.com", 75, user.ID, []string{"AI", "ML"})
	assert.NoError(t, err)
	assert.NotNil(t, datasource)
	assert.NotZero(t, datasource.ID)
	assert.Equal(t, "Test Datasource", datasource.Name)
	assert.Len(t, datasource.Tags, 2)

	// Test GetDatasourceByID
	fetchedDatasource, err := service.GetDatasourceByID(datasource.ID)
	assert.NoError(t, err)
	assert.Equal(t, datasource.ID, fetchedDatasource.ID)
	assert.Equal(t, datasource.Name, fetchedDatasource.Name)

	// Test UpdateDatasource
	updatedDatasource, err := service.UpdateDatasource(datasource.ID, "Updated Datasource", "Updated Short", "Updated Long", "updated-icon.png", "https://updated-example.com", 80)
	assert.NoError(t, err)
	assert.Equal(t, datasource.ID, updatedDatasource.ID)
	assert.Equal(t, "Updated Datasource", updatedDatasource.Name)
	assert.Equal(t, 80, updatedDatasource.PrivacyScore)

	// Test GetAllDatasources
	allDatasources, err := service.GetAllDatasources()
	assert.NoError(t, err)
	assert.Len(t, allDatasources, 1)
	assert.Equal(t, updatedDatasource.ID, allDatasources[0].ID)

	// Test SearchDatasources
	searchedDatasources, err := service.SearchDatasources("Updated")
	assert.NoError(t, err)
	assert.Len(t, searchedDatasources, 1)
	assert.Equal(t, updatedDatasource.ID, searchedDatasources[0].ID)

	// Test GetDatasourcesByTag
	datasourcesByTag, err := service.GetDatasourcesByTag("AI")
	assert.NoError(t, err)
	assert.Len(t, datasourcesByTag, 1)
	assert.Equal(t, updatedDatasource.ID, datasourcesByTag[0].ID)

	// Test AddTagsToDatasource
	err = service.AddTagsToDatasource(datasource.ID, []string{"NLP"})
	assert.NoError(t, err)
	updatedDatasource, _ = service.GetDatasourceByID(datasource.ID)
	assert.Len(t, updatedDatasource.Tags, 3)

	// Test GetDatasourcesByPrivacyScoreRange
	datasourcesByScore, err := service.GetDatasourcesByPrivacyScoreRange(70, 90)
	assert.NoError(t, err)
	assert.Len(t, datasourcesByScore, 1)
	assert.Equal(t, updatedDatasource.ID, datasourcesByScore[0].ID)

	// Test GetDatasourcesByUserID
	datasourcesByUser, err := service.GetDatasourcesByUserID(user.ID)
	assert.NoError(t, err)
	assert.Len(t, datasourcesByUser, 1)
	assert.Equal(t, updatedDatasource.ID, datasourcesByUser[0].ID)

	// Test DeleteDatasource
	err = service.DeleteDatasource(datasource.ID)
	assert.NoError(t, err)

	// Verify datasource is deleted
	_, err = service.GetDatasourceByID(datasource.ID)
	assert.Error(t, err)
}

func TestDatasourceService_MultipleDatasourcesScenario(t *testing.T) {
	db := setupTestDBForDatasources(t)
	service := NewService(db)

	// Create a user for testing
	user, _ := service.CreateUser("test@example.com", "Test User", "password123")

	// Create multiple datasources
	ds1, _ := service.CreateDatasource("Datasource 1", "Short 1", "Long 1", "icon1.png", "https://ds1.com", 60, user.ID, []string{"AI", "ML"})
	ds2, _ := service.CreateDatasource("Datasource 2", "Short 2", "Long 2", "icon2.png", "https://ds2.com", 75, user.ID, []string{"NLP", "ML"})
	ds3, _ := service.CreateDatasource("Datasource 3", "Short 3", "Long 3", "icon3.png", "https://ds3.com", 90, user.ID, []string{"AI", "NLP"})

	// Test GetAllDatasources
	allDatasources, err := service.GetAllDatasources()
	assert.NoError(t, err)
	assert.Len(t, allDatasources, 3)

	// Test SearchDatasources
	searchedDatasources, err := service.SearchDatasources("Datasource")
	assert.NoError(t, err)
	assert.Len(t, searchedDatasources, 3)

	// Test GetDatasourcesByTag
	mlDatasources, err := service.GetDatasourcesByTag("ML")
	assert.NoError(t, err)
	assert.Len(t, mlDatasources, 2)

	// Test GetDatasourcesByMinPrivacyScore
	highPrivacyDatasources, err := service.GetDatasourcesByMinPrivacyScore(75)
	assert.NoError(t, err)
	assert.Len(t, highPrivacyDatasources, 2)
	assert.Contains(t, []uint{ds2.ID, ds3.ID}, highPrivacyDatasources[0].ID)
	assert.Contains(t, []uint{ds2.ID, ds3.ID}, highPrivacyDatasources[1].ID)

	// Test GetDatasourcesByMaxPrivacyScore
	lowPrivacyDatasources, err := service.GetDatasourcesByMaxPrivacyScore(75)
	assert.NoError(t, err)
	assert.Len(t, lowPrivacyDatasources, 2)
	assert.Contains(t, []uint{ds1.ID, ds2.ID}, lowPrivacyDatasources[0].ID)
	assert.Contains(t, []uint{ds1.ID, ds2.ID}, lowPrivacyDatasources[1].ID)

	// Test GetDatasourcesByPrivacyScoreRange
	midPrivacyDatasources, err := service.GetDatasourcesByPrivacyScoreRange(70, 80)
	assert.NoError(t, err)
	assert.Len(t, midPrivacyDatasources, 1)
	assert.Equal(t, ds2.ID, midPrivacyDatasources[0].ID)

	// Test GetDatasourcesByUserID
	userDatasources, err := service.GetDatasourcesByUserID(user.ID)
	assert.NoError(t, err)
	assert.Len(t, userDatasources, 3)
}
