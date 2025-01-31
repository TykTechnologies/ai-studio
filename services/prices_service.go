package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// CreateModelPrice creates a new model price
func (s *Service) CreateModelPrice(modelName, vendor string, cpt, cpit float64, currency string) (*models.ModelPrice, error) {
	modelPrice := &models.ModelPrice{
		ModelName: modelName,
		Vendor:    vendor,
		CPT:       cpt,
		CPIT:      cpit,
		Currency:  currency,
	}

	if err := modelPrice.Create(s.DB); err != nil {
		return nil, err
	}

	return modelPrice, nil
}

// GetModelPriceByID retrieves a model price by its ID
func (s *Service) GetModelPriceByID(id uint) (*models.ModelPrice, error) {
	modelPrice := &models.ModelPrice{}
	if err := modelPrice.Get(s.DB, id); err != nil {
		return nil, err
	}
	return modelPrice, nil
}

// UpdateModelPrice updates an existing model price
func (s *Service) UpdateModelPrice(id uint, modelName, vendor string, cpt, cpit float64, currency string) (*models.ModelPrice, error) {
	modelPrice, err := s.GetModelPriceByID(id)
	if err != nil {
		return nil, err
	}

	modelPrice.ModelName = modelName
	modelPrice.Vendor = vendor
	modelPrice.CPT = cpt
	modelPrice.CPIT = cpit
	modelPrice.Currency = currency

	if err := modelPrice.Update(s.DB); err != nil {
		return nil, err
	}

	return modelPrice, nil
}

// DeleteModelPrice deletes a model price
func (s *Service) DeleteModelPrice(id uint) error {
	modelPrice, err := s.GetModelPriceByID(id)
	if err != nil {
		return err
	}

	return modelPrice.Delete(s.DB)
}

// GetAllModelPrices retrieves all model prices
func (s *Service) GetAllModelPrices(pageSize int, pageNumber int, all bool) (models.ModelPrices, int64, int, error) {
	var modelPrices models.ModelPrices
	totalCount, totalPages, err := modelPrices.GetAll(s.DB, pageSize, pageNumber, all)
	if err != nil {
		return nil, 0, 0, err
	}
	return modelPrices, totalCount, totalPages, nil
}

// GetModelPricesByVendor retrieves all model prices for a specific vendor
func (s *Service) GetModelPricesByVendor(vendor string) (models.ModelPrices, error) {
	var modelPrices models.ModelPrices
	if err := modelPrices.GetByVendor(s.DB, vendor); err != nil {
		return nil, err
	}
	return modelPrices, nil
}

// GetModelPriceByModelName retrieves a model price by its model name
func (s *Service) GetModelPriceByModelName(modelName string) (*models.ModelPrice, error) {
	modelPrice := &models.ModelPrice{}
	if err := modelPrice.GetByModelName(s.DB, modelName); err != nil {
		return nil, err
	}
	return modelPrice, nil
}

// GetModelPriceByModelNameAndVendor retrieves a model price by its model name and vendor
func (s *Service) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	modelPrice := &models.ModelPrice{}
	if err := modelPrice.GetByModelNameAndVendor(s.DB, modelName, vendor); err != nil {
		if err.Error() == gorm.ErrRecordNotFound.Error() {
			// not found, create it
			modelPrice.ModelName = modelName
			modelPrice.Vendor = vendor
			modelPrice.CPT = 0.0
			modelPrice.CPIT = 0.0
			modelPrice.Currency = "USD"
			if err := modelPrice.Create(s.DB); err != nil {
				return nil, err
			}

			return modelPrice, nil
		}

		return nil, err
	}
	return modelPrice, nil
}

// CreateMultipleModelPrices creates multiple model prices at once
func (s *Service) CreateMultipleModelPrices(modelPrices models.ModelPrices) error {
	return modelPrices.CreateMultiple(s.DB)
}

// UpdateMultipleModelPrices updates multiple model prices at once
func (s *Service) UpdateMultipleModelPrices(modelPrices models.ModelPrices) error {
	return modelPrices.UpdateMultiple(s.DB)
}

// DeleteMultipleModelPrices deletes multiple model prices at once
func (s *Service) DeleteMultipleModelPrices(modelPrices models.ModelPrices) error {
	return modelPrices.DeleteMultiple(s.DB)
}
