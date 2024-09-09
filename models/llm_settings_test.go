package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLLMSettings_NewLLMSettings(t *testing.T) {
	settings := NewLLMSettings()
	assert.NotNil(t, settings)
}

func TestLLMSettings_CRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create
	settings := &LLMSettings{
		MaxLength:         100,
		MaxTokens:         50,
		Metadata:          map[string]interface{}{"key": "value"},
		MinLength:         10,
		ModelName:         "TestModel",
		RepetitionPenalty: 1.2,
		Seed:              42,
		StopWords:         []string{"stop1", "stop2"},
		Temperature:       0.7,
		TopK:              40,
		TopP:              0.9,
	}
	err := settings.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, settings.ID)

	// Get
	fetchedSettings := NewLLMSettings()
	err = fetchedSettings.Get(db, settings.ID)
	assert.NoError(t, err)
	assert.Equal(t, settings.ModelName, fetchedSettings.ModelName)
	assert.Equal(t, settings.Temperature, fetchedSettings.Temperature)
	assert.ElementsMatch(t, settings.StopWords, fetchedSettings.StopWords)

	// Update
	settings.ModelName = "UpdatedTestModel"
	settings.Temperature = 0.8
	err = settings.Update(db)
	assert.NoError(t, err)

	err = fetchedSettings.Get(db, settings.ID)
	assert.NoError(t, err)
	assert.Equal(t, "UpdatedTestModel", fetchedSettings.ModelName)
	assert.Equal(t, 0.8, fetchedSettings.Temperature)

	// Delete
	err = settings.Delete(db)
	assert.NoError(t, err)

	err = fetchedSettings.Get(db, settings.ID)
	assert.Error(t, err) // Should return an error as the settings are deleted
}

func TestLLMSettings_GetAll(t *testing.T) {
	db := setupTestDB(t)

	// Create some test LLMSettings
	settings := []LLMSettings{
		{ModelName: "Model1", Temperature: 0.7},
		{ModelName: "Model2", Temperature: 0.8},
		{ModelName: "Model3", Temperature: 0.9},
	}
	for _, s := range settings {
		err := db.Create(&s).Error
		assert.NoError(t, err)
	}

	// Test GetAll
	var fetchedSettings LLMSettingsSlice
	err := fetchedSettings.GetAll(db)
	assert.NoError(t, err)
	assert.Len(t, fetchedSettings, 3)
	assert.Equal(t, "Model1", fetchedSettings[0].ModelName)
	assert.Equal(t, "Model2", fetchedSettings[1].ModelName)
	assert.Equal(t, "Model3", fetchedSettings[2].ModelName)
}

func TestLLMSettings_GetByModel(t *testing.T) {
	db := setupTestDB(t)

	// Create a test LLMSettings
	settings := &LLMSettings{
		ModelName:   "UniqueModel",
		Temperature: 0.75,
	}
	err := db.Create(settings).Error
	assert.NoError(t, err)

	// Test GetByModel
	fetchedSettings := NewLLMSettings()
	err = fetchedSettings.GetByModel(db, "UniqueModel")
	assert.NoError(t, err)
	assert.Equal(t, settings.ID, fetchedSettings.ID)
	assert.Equal(t, settings.ModelName, fetchedSettings.ModelName)
	assert.Equal(t, settings.Temperature, fetchedSettings.Temperature)

	// Test with non-existent model
	err = fetchedSettings.GetByModel(db, "NonExistentModel")
	assert.Error(t, err)
}

func TestLLMSettings_SearchByModelStub(t *testing.T) {
	db := setupTestDB(t)

	// Create some test LLMSettings
	settings := []LLMSettings{
		{ModelName: "GPT-3", Temperature: 0.7},
		{ModelName: "GPT-4", Temperature: 0.8},
		{ModelName: "BERT", Temperature: 0.9},
	}
	for _, s := range settings {
		err := db.Create(&s).Error
		assert.NoError(t, err)
	}

	// Test SearchByModelStub
	var fetchedSettings LLMSettingsSlice
	err := fetchedSettings.SearchByModelStub(db, "GPT")
	assert.NoError(t, err)
	assert.Len(t, fetchedSettings, 2)
	assert.Equal(t, "GPT-3", fetchedSettings[0].ModelName)
	assert.Equal(t, "GPT-4", fetchedSettings[1].ModelName)

	// Test with a different stub
	fetchedSettings = LLMSettingsSlice{}
	err = fetchedSettings.SearchByModelStub(db, "BERT")
	assert.NoError(t, err)
	assert.Len(t, fetchedSettings, 1)
	assert.Equal(t, "BERT", fetchedSettings[0].ModelName)

	// Test with a stub that doesn't match any settings
	fetchedSettings = LLMSettingsSlice{}
	err = fetchedSettings.SearchByModelStub(db, "XYZ")
	assert.NoError(t, err)
	assert.Len(t, fetchedSettings, 0)
}
