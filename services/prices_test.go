package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPricesTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = db.AutoMigrate(&models.ModelPrice{}, &models.LLMChatRecord{})
	assert.NoError(t, err)

	return db
}

func TestUpdateModelPriceRecalculatesCosts(t *testing.T) {
	db := setupPricesTestDB(t)
	svc := &Service{DB: db}

	// Create initial model price
	mp := &models.ModelPrice{
		ModelName: "GPT-4",
		Vendor:    "OpenAI",
		CPT:       0.002, // Initial cost per token
		CPIT:      0.003, // Initial cost per input token
		Currency:  "USD",
	}
	err := mp.Create(db)
	assert.NoError(t, err)

	// Create some chat records
	records := []models.LLMChatRecord{
		{
			Name:           "GPT-4",
			Vendor:         "OpenAI",
			PromptTokens:   100,
			ResponseTokens: 50,
			TotalTokens:    150,
			Cost:           0.35, // (100 * 0.003) + (50 * 0.002)
			Currency:       "USD",
		},
		{
			Name:           "GPT-4",
			Vendor:         "OpenAI",
			PromptTokens:   200,
			ResponseTokens: 100,
			TotalTokens:    300,
			Cost:           0.80, // (200 * 0.003) + (100 * 0.002)
			Currency:       "USD",
		},
	}
	for _, record := range records {
		err := db.Create(&record).Error
		assert.NoError(t, err)
	}

	// Update model price with new rates using service
	_, err = svc.UpdateModelPriceAndRecalculate(mp.ID, mp.ModelName, mp.Vendor, 0.004, 0.006, mp.Currency)
	assert.NoError(t, err)

	// Verify costs were updated
	var updatedRecords []models.LLMChatRecord
	err = db.Where("name = ? AND vendor = ?", "GPT-4", "OpenAI").Find(&updatedRecords).Error
	assert.NoError(t, err)
	assert.Len(t, updatedRecords, 2)

	// Check first record: (100 * 0.006) + (50 * 0.004) = 0.8
	assert.Equal(t, 0.8, updatedRecords[0].Cost)

	// Check second record: (200 * 0.006) + (100 * 0.004) = 1.6
	assert.Equal(t, 1.6, updatedRecords[1].Cost)
}
