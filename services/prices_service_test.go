package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestModelPriceService(t *testing.T) {
	db := setupTestDB(t)
	service := &Service{DB: db}

	t.Run("CreateModelPrice", func(t *testing.T) {
		modelPrice, err := service.CreateModelPrice("GPT-4", "OpenAI", 0.0001, 0.00005, "USD")
		assert.NoError(t, err)
		assert.NotNil(t, modelPrice)
		assert.Equal(t, "GPT-4", modelPrice.ModelName)
		assert.Equal(t, "OpenAI", modelPrice.Vendor)
		assert.Equal(t, 0.0001, modelPrice.CPT)
	})

	t.Run("GetModelPriceByID", func(t *testing.T) {
		createdModelPrice, _ := service.CreateModelPrice("GPT-3", "OpenAI", 0.00005, 0.00005, "USD")
		modelPrice, err := service.GetModelPriceByID(createdModelPrice.ID)
		assert.NoError(t, err)
		assert.NotNil(t, modelPrice)
		assert.Equal(t, createdModelPrice.ID, modelPrice.ID)
	})

	t.Run("UpdateModelPrice", func(t *testing.T) {
		createdModelPrice, _ := service.CreateModelPrice("BERT", "Google", 0.00002, 0.00005, "USD")
		updatedModelPrice, err := service.UpdateModelPrice(createdModelPrice.ID, "BERT-Large", "Google", 0.00003, 0.00005, "USD")
		assert.NoError(t, err)
		assert.Equal(t, "BERT-Large", updatedModelPrice.ModelName)
		assert.Equal(t, 0.00003, updatedModelPrice.CPT)
	})

	t.Run("DeleteModelPrice", func(t *testing.T) {
		createdModelPrice, _ := service.CreateModelPrice("T5", "Google", 0.00001, 0.00005, "USD")
		err := service.DeleteModelPrice(createdModelPrice.ID)
		assert.NoError(t, err)
		_, err = service.GetModelPriceByID(createdModelPrice.ID)
		assert.Error(t, err)
	})

	t.Run("GetAllModelPrices", func(t *testing.T) {
		service.CreateModelPrice("Model1", "Vendor1", 0.0001, 0.00005, "USD")
		service.CreateModelPrice("Model2", "Vendor2", 0.0002, 0.00005, "USD")
		modelPrices, _, _, err := service.GetAllModelPrices(10, 1, true)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(modelPrices), 2)
	})

	t.Run("GetModelPricesByVendor", func(t *testing.T) {
		service.CreateModelPrice("Model3", "Vendor3", 0.0003, 0.00005, "USD")
		service.CreateModelPrice("Model4", "Vendor3", 0.0004, 0.00005, "USD")
		modelPrices, err := service.GetModelPricesByVendor("Vendor3")
		assert.NoError(t, err)
		assert.Equal(t, 2, len(modelPrices))
	})

	t.Run("GetModelPriceByModelName", func(t *testing.T) {
		service.CreateModelPrice("UniqueModel", "UniqueVendor", 0.0005, 0.00005, "USD")
		modelPrice, err := service.GetModelPriceByModelName("UniqueModel")
		assert.NoError(t, err)
		assert.Equal(t, "UniqueModel", modelPrice.ModelName)
		assert.Equal(t, "UniqueVendor", modelPrice.Vendor)
	})

	t.Run("GetModelPriceByModelNameAndVendor", func(t *testing.T) {
		service.CreateModelPrice("SpecificModel", "SpecificVendor", 0.0006, 0.00005, "USD")
		modelPrice, err := service.GetModelPriceByModelNameAndVendor("SpecificModel", "SpecificVendor")
		assert.NoError(t, err)
		assert.Equal(t, "SpecificModel", modelPrice.ModelName)
		assert.Equal(t, "SpecificVendor", modelPrice.Vendor)
	})

	t.Run("CreateMultipleModelPrices", func(t *testing.T) {
		modelPrices := models.ModelPrices{
			{ModelName: "Bulk1", Vendor: "BulkVendor", CPT: 0.0007},
			{ModelName: "Bulk2", Vendor: "BulkVendor", CPT: 0.0008},
		}
		err := service.CreateMultipleModelPrices(modelPrices)
		assert.NoError(t, err)
		retrievedPrices, err := service.GetModelPricesByVendor("BulkVendor")
		assert.NoError(t, err)
		assert.Equal(t, 2, len(retrievedPrices))
	})

	t.Run("UpdateMultipleModelPrices", func(t *testing.T) {
		modelPrices := models.ModelPrices{
			{ModelName: "UpdateBulk1", Vendor: "UpdateVendor", CPT: 0.0009},
			{ModelName: "UpdateBulk2", Vendor: "UpdateVendor", CPT: 0.0010},
		}
		service.CreateMultipleModelPrices(modelPrices)

		modelPrices[0].CPT = 0.0011
		modelPrices[1].CPT = 0.0012

		err := service.UpdateMultipleModelPrices(modelPrices)
		assert.NoError(t, err)

		updatedPrices, err := service.GetModelPricesByVendor("UpdateVendor")
		assert.NoError(t, err)
		assert.Equal(t, 0.0011, updatedPrices[0].CPT)
		assert.Equal(t, 0.0012, updatedPrices[1].CPT)
	})

	t.Run("DeleteMultipleModelPrices", func(t *testing.T) {
		modelPrices := models.ModelPrices{
			{ModelName: "DeleteBulk1", Vendor: "DeleteVendor", CPT: 0.0013},
			{ModelName: "DeleteBulk2", Vendor: "DeleteVendor", CPT: 0.0014},
		}
		service.CreateMultipleModelPrices(modelPrices)

		err := service.DeleteMultipleModelPrices(modelPrices)
		assert.NoError(t, err)

		remainingPrices, err := service.GetModelPricesByVendor("DeleteVendor")
		assert.NoError(t, err)
		assert.Equal(t, 0, len(remainingPrices))
	})
}
