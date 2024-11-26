package services

import (
	"fmt"
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
		ModelName:   "TestModel",
		Temperature: 0.7,
		MaxTokens:   100,
		TopP:        0.9,
		StopWords:   []string{"stop1", "stop2"},
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
	allSettings, _, _, err := service.GetAllLLMSettings(10, 1, true)
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
	allSettings, _, _, err := service.GetAllLLMSettings(10, 1, true)
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
	updatedSettings, _, _, err := service.GetAllLLMSettings(10, 1, true)
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
	remainingSettings, _, _, err := service.GetAllLLMSettings(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, *remainingSettings, 0)
}

func setupTestDBForLLMPagination(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestLLMServicePagination(t *testing.T) {
	db := setupTestDBForLLMPagination(t)
	service := NewService(db)

	// Create 25 test LLMs
	for i := 1; i <= 25; i++ {
		_, err := service.CreateLLM(
			fmt.Sprintf("LLM%d", i),
			fmt.Sprintf("key%d", i),
			fmt.Sprintf("https://api%d.com", i),
			75,
			"Short desc",
			"Long desc",
			"https://logo.test",
			models.OPENAI,
			true,
			nil,
			"",
		)
		assert.NoError(t, err)
	}

	testCases := []struct {
		name          string
		pageSize      int
		pageNumber    int
		all           bool
		expectedCount int
		expectedTotal int64
		expectedPages int
		expectedFirst string
		expectedLast  string
	}{
		{
			name:          "First page of 10",
			pageSize:      10,
			pageNumber:    1,
			all:           false,
			expectedCount: 10,
			expectedTotal: 25,
			expectedPages: 3,
			expectedFirst: "LLM1",
			expectedLast:  "LLM10",
		},
		{
			name:          "Second page of 10",
			pageSize:      10,
			pageNumber:    2,
			all:           false,
			expectedCount: 10,
			expectedTotal: 25,
			expectedPages: 3,
			expectedFirst: "LLM11",
			expectedLast:  "LLM20",
		},
		{
			name:          "Last page of 10",
			pageSize:      10,
			pageNumber:    3,
			all:           false,
			expectedCount: 5,
			expectedTotal: 25,
			expectedPages: 3,
			expectedFirst: "LLM21",
			expectedLast:  "LLM25",
		},
		{
			name:          "Page size larger than total",
			pageSize:      30,
			pageNumber:    1,
			all:           false,
			expectedCount: 25,
			expectedTotal: 25,
			expectedPages: 1,
			expectedFirst: "LLM1",
			expectedLast:  "LLM25",
		},
		{
			name:          "Get all LLMs",
			pageSize:      10,
			pageNumber:    1,
			all:           true,
			expectedCount: 25,
			expectedTotal: 25,
			expectedPages: 3,
			expectedFirst: "LLM1",
			expectedLast:  "LLM25",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			llms, totalCount, totalPages, err := service.GetAllLLMs(tc.pageSize, tc.pageNumber, tc.all)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedTotal, totalCount)
			assert.Equal(t, tc.expectedPages, totalPages)
			assert.Len(t, llms, tc.expectedCount)

			if len(llms) > 0 {
				assert.Equal(t, tc.expectedFirst, llms[0].Name)
				assert.Equal(t, tc.expectedLast, llms[len(llms)-1].Name)
			}
		})
	}

	// Test invalid page number
	t.Run("Invalid page number", func(t *testing.T) {
		llms, totalCount, totalPages, err := service.GetAllLLMs(10, 10, false)
		assert.NoError(t, err)
		assert.Equal(t, int64(25), totalCount)
		assert.Equal(t, 3, totalPages)
		assert.Len(t, llms, 0)
	})
}
