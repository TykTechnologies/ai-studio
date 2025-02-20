package models_test

import (
	"testing"

	. "github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPricesTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = db.AutoMigrate(&ModelPrice{}, &LLMChatRecord{})
	assert.NoError(t, err)

	return db
}

func TestModelPriceBasicCRUD(t *testing.T) {
	db := setupPricesTestDB(t)

	// Test Create
	mp := &ModelPrice{
		ModelName: "GPT-4",
		Vendor:    "OpenAI",
		CPT:       0.002,
		CPIT:      0.003,
		Currency:  "USD",
	}
	err := mp.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, mp.ID)

	// Test Get
	retrieved := &ModelPrice{}
	err = retrieved.Get(db, mp.ID)
	assert.NoError(t, err)
	assert.Equal(t, mp.ModelName, retrieved.ModelName)
	assert.Equal(t, mp.Vendor, retrieved.Vendor)
	assert.Equal(t, mp.CPT, retrieved.CPT)
	assert.Equal(t, mp.CPIT, retrieved.CPIT)
	assert.Equal(t, mp.Currency, retrieved.Currency)

	// Test Update
	mp.CPT = 0.004
	mp.CPIT = 0.006
	err = mp.Update(db)
	assert.NoError(t, err)

	// Verify update
	updated := &ModelPrice{}
	err = updated.Get(db, mp.ID)
	assert.NoError(t, err)
	assert.Equal(t, 0.004, updated.CPT)
	assert.Equal(t, 0.006, updated.CPIT)

	// Test Delete
	err = mp.Delete(db)
	assert.NoError(t, err)

	// Verify deletion
	deleted := &ModelPrice{}
	err = deleted.Get(db, mp.ID)
	assert.Error(t, err)
}

func TestGetOrCreateByModelName(t *testing.T) {
	db := setupPricesTestDB(t)

	t.Run("creates new model price when not found", func(t *testing.T) {
		mp := &ModelPrice{}
		err := mp.GetOrCreateByModelName(db, "GPT-4")
		assert.NoError(t, err)

		// Verify the model was created with default values
		assert.Equal(t, "GPT-4", mp.ModelName)
		assert.Equal(t, 0.0, mp.CPT)
		assert.Equal(t, 0.0, mp.CPIT)
		assert.Equal(t, "USD", mp.Currency)
		assert.NotZero(t, mp.ID) // Ensure ID was set
	})

	t.Run("returns existing model price when found", func(t *testing.T) {
		// Create a model price first
		existingMP := &ModelPrice{
			ModelName: "GPT-3",
			Vendor:    "OpenAI",
			CPT:       0.002,
			CPIT:      0.003,
			Currency:  "EUR",
		}
		err := existingMP.Create(db)
		assert.NoError(t, err)

		// Try to get or create the same model
		mp := &ModelPrice{}
		err = mp.GetOrCreateByModelName(db, "GPT-3")
		assert.NoError(t, err)

		// Verify we got the existing model
		assert.Equal(t, existingMP.ID, mp.ID)
		assert.Equal(t, "GPT-3", mp.ModelName)
		assert.Equal(t, "OpenAI", mp.Vendor)
		assert.Equal(t, 0.002, mp.CPT)
		assert.Equal(t, 0.003, mp.CPIT)
		assert.Equal(t, "EUR", mp.Currency)
	})
}
