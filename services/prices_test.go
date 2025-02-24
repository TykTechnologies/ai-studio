package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestService(t *testing.T) *Service {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = db.AutoMigrate(&models.ModelPrice{}, &models.LLMChatRecord{})
	assert.NoError(t, err)

	return &Service{DB: db}
}

func TestUpdateModelPriceRecalculatesCosts(t *testing.T) {
	t.Run("basic cost recalculation", func(t *testing.T) {
		service := setupTestService(t)

		// Create initial model price
		mp := &models.ModelPrice{
			ModelName:    "GPT-4",
			Vendor:       "OpenAI",
			CPT:          0.002, // Initial cost per token
			CPIT:         0.003, // Initial cost per input token
			CacheWritePT: 0.0005,
			CacheReadPT:  0.0001,
			Currency:     "USD",
		}
		err := mp.Create(service.DB)
		assert.NoError(t, err)

		// Create some chat records
		records := []models.LLMChatRecord{
			{
				Name:                   "GPT-4",
				Vendor:                 "OpenAI",
				PromptTokens:           100,
				ResponseTokens:         50,
				CacheWritePromptTokens: 20,
				CacheReadPromptTokens:  10,
				TotalTokens:            180,    // 100 + 50 + 20 + 10
				Cost:                   0.3525, // (50 * 0.002) + (100 * 0.003) + (20 * 0.0005) + (10 * 0.0001)
				Currency:               "USD",
			},
			{
				Name:                   "GPT-4",
				Vendor:                 "OpenAI",
				PromptTokens:           200,
				ResponseTokens:         100,
				CacheWritePromptTokens: 40,
				CacheReadPromptTokens:  20,
				TotalTokens:            360,   // 200 + 100 + 40 + 20
				Cost:                   0.805, // (100 * 0.002) + (200 * 0.003) + (40 * 0.0005) + (20 * 0.0001)
				Currency:               "USD",
			},
		}
		for _, record := range records {
			err := service.DB.Create(&record).Error
			assert.NoError(t, err)
		}

		// Update model price with new rates and currency using service
		_, err = service.UpdateModelPriceAndRecalculate(mp.ID, mp.ModelName, mp.Vendor, 0.004, 0.006, 0.001, 0.0002, "EUR")
		assert.NoError(t, err)

		// Verify costs and currency were updated
		var updatedRecords []models.LLMChatRecord
		err = service.DB.Where("name = ? AND vendor = ?", "GPT-4", "OpenAI").Find(&updatedRecords).Error
		assert.NoError(t, err)
		assert.Len(t, updatedRecords, 2)

		// Check first record: (50 * 0.004) + (100 * 0.006) + (20 * 0.001) + (10 * 0.0002) = 0.822
		assert.InDelta(t, 0.822, updatedRecords[0].Cost, 0.0001)
		assert.Equal(t, "EUR", updatedRecords[0].Currency)

		// Check second record: (100 * 0.004) + (200 * 0.006) + (40 * 0.001) + (20 * 0.0002) = 1.644
		assert.InDelta(t, 1.644, updatedRecords[1].Cost, 0.0001)
		assert.Equal(t, "EUR", updatedRecords[1].Currency)
	})

	t.Run("cache-heavy cost recalculation", func(t *testing.T) {
		service := setupTestService(t)

		// Create initial model price with significant cache costs
		mp := &models.ModelPrice{
			ModelName:    "GPT-4-Cache",
			Vendor:       "OpenAI",
			CPT:          0.002,
			CPIT:         0.003,
			CacheWritePT: 0.001,  // Higher cache write cost
			CacheReadPT:  0.0005, // Higher cache read cost
			Currency:     "USD",
		}
		err := mp.Create(service.DB)
		assert.NoError(t, err)

		// Create chat records with significant cache usage
		records := []models.LLMChatRecord{
			{
				Name:                   "GPT-4-Cache",
				Vendor:                 "OpenAI",
				PromptTokens:           50,
				ResponseTokens:         25,
				CacheWritePromptTokens: 100,    // More cache writes than direct tokens
				CacheReadPromptTokens:  200,    // More cache reads than direct tokens
				TotalTokens:            375,    // 50 + 25 + 100 + 200
				Cost:                   0.3525, // (25 * 0.002) + (50 * 0.003) + (100 * 0.001) + (200 * 0.0005)
				Currency:               "USD",
			},
		}
		for _, record := range records {
			err := service.DB.Create(&record).Error
			assert.NoError(t, err)
		}

		// Update to new rates that heavily weight cache operations
		_, err = service.UpdateModelPriceAndRecalculate(mp.ID, mp.ModelName, mp.Vendor, 0.004, 0.006, 0.002, 0.001, "EUR")
		assert.NoError(t, err)

		// Verify costs and currency were updated
		var updatedRecords []models.LLMChatRecord
		err = service.DB.Where("name = ? AND vendor = ?", "GPT-4-Cache", "OpenAI").Find(&updatedRecords).Error
		assert.NoError(t, err)
		assert.Len(t, updatedRecords, 1)

		// Check record: (25 * 0.004) + (50 * 0.006) + (100 * 0.002) + (200 * 0.001) = 0.8
		assert.InDelta(t, 0.8, updatedRecords[0].Cost, 0.0001)
		assert.Equal(t, "EUR", updatedRecords[0].Currency)
	})
}

func TestCreateModelPrice(t *testing.T) {
	service := setupTestService(t)

	modelPrice, err := service.CreateModelPrice(
		"GPT-3",
		"OpenAI",
		0.002,
		0.001,
		0.0005,
		0.0001,
		"USD",
	)
	assert.NoError(t, err)
	assert.NotNil(t, modelPrice)
	assert.Equal(t, "GPT-3", modelPrice.ModelName)
	assert.Equal(t, "OpenAI", modelPrice.Vendor)
	assert.Equal(t, 0.002, modelPrice.CPT)
	assert.Equal(t, 0.001, modelPrice.CPIT)
	assert.Equal(t, 0.0005, modelPrice.CacheWritePT)
	assert.Equal(t, 0.0001, modelPrice.CacheReadPT)
	assert.Equal(t, "USD", modelPrice.Currency)
}

func TestGetModelPriceByID(t *testing.T) {
	service := setupTestService(t)

	modelPrice, err := service.CreateModelPrice(
		"GPT-3",
		"OpenAI",
		0.002,
		0.001,
		0.0005,
		0.0001,
		"USD",
	)
	assert.NoError(t, err)

	retrieved, err := service.GetModelPriceByID(modelPrice.ID)
	assert.NoError(t, err)
	assert.Equal(t, modelPrice.ID, retrieved.ID)
}

func TestUpdateModelPrice(t *testing.T) {
	service := setupTestService(t)

	modelPrice, err := service.CreateModelPrice(
		"GPT-3",
		"OpenAI",
		0.002,
		0.001,
		0.0005,
		0.0001,
		"USD",
	)
	assert.NoError(t, err)

	updated, err := service.UpdateModelPrice(
		modelPrice.ID,
		"GPT-3",
		"OpenAI",
		0.003,
		0.0015,
		0.0007,
		0.0002,
		"USD",
	)
	assert.NoError(t, err)
	assert.Equal(t, 0.003, updated.CPT)
	assert.Equal(t, 0.0015, updated.CPIT)
	assert.Equal(t, 0.0007, updated.CacheWritePT)
	assert.Equal(t, 0.0002, updated.CacheReadPT)
}

func TestDeleteModelPrice(t *testing.T) {
	service := setupTestService(t)

	modelPrice, err := service.CreateModelPrice(
		"GPT-3",
		"OpenAI",
		0.002,
		0.001,
		0.0005,
		0.0001,
		"USD",
	)
	assert.NoError(t, err)

	err = service.DeleteModelPrice(modelPrice.ID)
	assert.NoError(t, err)

	_, err = service.GetModelPriceByID(modelPrice.ID)
	assert.Equal(t, gorm.ErrRecordNotFound, err)
}

func TestGetAllModelPrices(t *testing.T) {
	service := setupTestService(t)

	modelPrice1, err := service.CreateModelPrice(
		"GPT-3",
		"OpenAI",
		0.002,
		0.001,
		0.0005,
		0.0001,
		"USD",
	)
	assert.NoError(t, err)

	modelPrice2, err := service.CreateModelPrice(
		"GPT-4",
		"OpenAI",
		0.003,
		0.0015,
		0.0007,
		0.0002,
		"USD",
	)
	assert.NoError(t, err)

	modelPrices, totalCount, totalPages, err := service.GetAllModelPrices(10, 1, true)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), totalCount)
	assert.Equal(t, 1, totalPages)
	assert.Len(t, modelPrices, 2)
	assert.Contains(t, []uint{modelPrice1.ID, modelPrice2.ID}, modelPrices[0].ID)
}

func TestGetModelPricesByVendor(t *testing.T) {
	service := setupTestService(t)

	modelPrice1, err := service.CreateModelPrice(
		"GPT-3",
		"OpenAI",
		0.002,
		0.001,
		0.0005,
		0.0001,
		"USD",
	)
	assert.NoError(t, err)

	modelPrice2, err := service.CreateModelPrice(
		"GPT-4",
		"OpenAI",
		0.003,
		0.0015,
		0.0007,
		0.0002,
		"USD",
	)
	assert.NoError(t, err)

	modelPrices, err := service.GetModelPricesByVendor("OpenAI")
	assert.NoError(t, err)
	assert.Len(t, modelPrices, 2)
	assert.Contains(t, []uint{modelPrice1.ID, modelPrice2.ID}, modelPrices[0].ID)
}

func TestGetModelPriceByModelName(t *testing.T) {
	service := setupTestService(t)

	modelPrice, err := service.CreateModelPrice(
		"GPT-3",
		"OpenAI",
		0.002,
		0.001,
		0.0005,
		0.0001,
		"USD",
	)
	assert.NoError(t, err)

	retrieved, err := service.GetModelPriceByModelName("GPT-3")
	assert.NoError(t, err)
	assert.Equal(t, modelPrice.ID, retrieved.ID)
}

func TestGetOrCreateModelPriceByName(t *testing.T) {
	service := setupTestService(t)

	// Test creating new model price
	modelPrice, err := service.GetOrCreateModelPriceByName("GPT-4")
	assert.NoError(t, err)
	assert.Equal(t, "GPT-4", modelPrice.ModelName)
	assert.Equal(t, 0.0, modelPrice.CPT)
	assert.Equal(t, 0.0, modelPrice.CPIT)
	assert.Equal(t, 0.0, modelPrice.CacheWritePT)
	assert.Equal(t, 0.0, modelPrice.CacheReadPT)

	// Test getting existing model price
	existing, err := service.GetOrCreateModelPriceByName("GPT-4")
	assert.NoError(t, err)
	assert.Equal(t, modelPrice.ID, existing.ID)
}

func TestGetModelPriceByModelNameAndVendor(t *testing.T) {
	service := setupTestService(t)

	modelPrice, err := service.CreateModelPrice(
		"GPT-3",
		"OpenAI",
		0.002,
		0.001,
		0.0005,
		0.0001,
		"USD",
	)
	assert.NoError(t, err)

	retrieved, err := service.GetModelPriceByModelNameAndVendor("GPT-3", "OpenAI")
	assert.NoError(t, err)
	assert.Equal(t, modelPrice.ID, retrieved.ID)
}

func TestCreateMultipleModelPrices(t *testing.T) {
	service := setupTestService(t)

	modelPrices := models.ModelPrices{
		{
			ModelName:    "GPT-3",
			Vendor:       "OpenAI",
			CPT:          0.002,
			CPIT:         0.001,
			CacheWritePT: 0.0005,
			CacheReadPT:  0.0001,
			Currency:     "USD",
		},
		{
			ModelName:    "GPT-4",
			Vendor:       "OpenAI",
			CPT:          0.003,
			CPIT:         0.0015,
			CacheWritePT: 0.0007,
			CacheReadPT:  0.0002,
			Currency:     "USD",
		},
	}

	err := service.CreateMultipleModelPrices(modelPrices)
	assert.NoError(t, err)

	retrieved, err := service.GetModelPriceByModelName("GPT-3")
	assert.NoError(t, err)
	assert.Equal(t, 0.002, retrieved.CPT)
	assert.Equal(t, 0.001, retrieved.CPIT)
	assert.Equal(t, 0.0005, retrieved.CacheWritePT)
	assert.Equal(t, 0.0001, retrieved.CacheReadPT)
}
