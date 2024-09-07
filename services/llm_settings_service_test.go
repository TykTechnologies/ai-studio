package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForLLMSettings(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestLLMSettingsService(t *testing.T) {
	db := setupTestDBForLLMSettings(t)
	service := NewService(db)

	// Test CreateLLMSettings
	settings := &models.LLMSettings{
		ModelName:        "TestModel",
		Temperature:      0.7,
		MaxTokens:        100,
		FrequencyPenalty: 0.5,
		PresencePenalty:  0.5,
		TopP:             0.9,
		StopWords:        []string{"stop1", "stop2"},
	}
	createdSettings, err := service.CreateLLMSettings(settings)
	assert.NoError(t, err)
	assert.NotNil(t, createdSettings)
	assert.NotZero(t, createdSettings.ID)
	assert.Equal(t, "TestModel", createdSettings.ModelName)

	// Test GetLLMSettingsByID
	fetchedSettings, err := service.GetLLMSettingsByID(createdSettings.ID)
	assert.NoError(t, err)
	assert.Equal(t, createdSettings.ID, fetchedSettings.ID)
	assert.Equal(t, createdSettings.ModelName, fetchedSettings.ModelName)

	// Test UpdateLLMSettings
	fetchedSettings.Temperature = 0.8
	updatedSettings, err := service.UpdateLLMSettings(fetchedSettings)
	assert.NoError(t, err)
	assert.Equal(t, 0.8, updatedSettings.Temperature)

	// Test GetAllLLMSettings
	allSettings, err := service.GetAllLLMSettings()
	assert.NoError(t, err)
	assert.Len(t, *allSettings, 1)

	// Test GetLLMSettingsByModel
	modelSettings, err := service.GetLLMSettingsByModel("TestModel")
	assert.NoError(t, err)
	assert.Equal(t, createdSettings.ID, modelSettings.ID)

	// Test SearchLLMSettingsByModelStub
	searchedSettings, err := service.SearchLLMSettingsByModelStub("Test")
	assert.NoError(t, err)
	assert.Len(t, *searchedSettings, 1)
	assert.Equal(t, createdSettings.ID, (*searchedSettings)[0].ID)

	// Test DeleteLLMSettings
	err = service.DeleteLLMSettings(createdSettings.ID)
	assert.NoError(t, err)

	// Verify deletion
	_, err = service.GetLLMSettingsByID(createdSettings.ID)
	assert.Error(t, err)
}

func TestLLMSettingsService_MultipleSettings(t *testing.T) {
	db := setupTestDBForLLMSettings(t)
	service := NewService(db)

	// Create multiple LLMSettings
	settings1 := &models.LLMSettings{ModelName: "Model1", Temperature: 0.7}
	settings2 := &models.LLMSettings{ModelName: "Model2", Temperature: 0.8}
	settings3 := &models.LLMSettings{ModelName: "TestModel3", Temperature: 0.9}

	service.CreateLLMSettings(settings1)
	service.CreateLLMSettings(settings2)
	service.CreateLLMSettings(settings3)

	// Test GetAllLLMSettings
	allSettings, err := service.GetAllLLMSettings()
	assert.NoError(t, err)
	assert.Len(t, *allSettings, 3)

	// Test SearchLLMSettingsByModelStub
	searchedSettings, err := service.SearchLLMSettingsByModelStub("Model")
	assert.NoError(t, err)
	assert.Len(t, *searchedSettings, 2)

	// Verify searched settings
	modelNames := []string{(*searchedSettings)[0].ModelName, (*searchedSettings)[1].ModelName}
	assert.Contains(t, modelNames, "Model1")
	assert.Contains(t, modelNames, "Model2")

	// Test updating multiple settings
	for _, s := range *allSettings {
		s.MaxTokens = 150
		_, err := service.UpdateLLMSettings(&s)
		assert.NoError(t, err)
	}

	// Verify updates
	updatedSettings, err := service.GetAllLLMSettings()
	assert.NoError(t, err)
	for _, s := range *updatedSettings {
		assert.Equal(t, 150, s.MaxTokens)
	}

	// Test deleting all settings
	for _, s := range *allSettings {
		err := service.DeleteLLMSettings(s.ID)
		assert.NoError(t, err)
	}

	// Verify all settings are deleted
	remainingSettings, err := service.GetAllLLMSettings()
	assert.NoError(t, err)
	assert.Len(t, *remainingSettings, 0)
}
