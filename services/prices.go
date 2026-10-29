package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// CreateModelPrice creates a new model price
func (s *Service) CreateModelPrice(modelName, vendor string, cpt, cpit, cacheWritePT, cacheReadPT float64, currency string) (*models.ModelPrice, error) {
	modelPrice := &models.ModelPrice{
		ModelName:    modelName,
		Vendor:       vendor,
		CPT:          cpt,
		CPIT:         cpit,
		CacheWritePT: cacheWritePT,
		CacheReadPT:  cacheReadPT,
		Currency:     currency,
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

// recalculateChatRecordCosts reprices all chat records of a model at the
// updated price and sets their currency.
//
// Each row keeps the ratio of its recorded cost to its list cost at the old
// price, so a batch row recorded at a price multiplier
// (models.IPriceAdjustedResponse, e.g. 0.5x) stays at that multiplier, and a
// row recorded at list price gets exactly the new list price. A row with no
// old list cost (no price before, or old is the zero value) or no recorded
// cost gets the new list price at 1x, as before.
func (s *Service) recalculateChatRecordCosts(tx *gorm.DB, modelName, vendor string, old, updated models.ModelPrice) error {
	oldList, oldArgs := chatRecordListCostSQL(vendor, old)
	newList, newArgs := chatRecordListCostSQL(vendor, updated)

	args := append(append(newArgs, oldArgs...), updated.Currency, modelName, vendor)
	result := tx.Exec(`
		UPDATE llm_chat_records
		SET cost = `+newList+` * COALESCE(NULLIF(cost, 0) / NULLIF(`+oldList+`, 0), 1),
		currency = ?
		WHERE name = ? AND vendor = ?`,
		args...,
	)
	return result.Error
}

// chatRecordListCostSQL is a chat record's cost at price p in SQL, in the
// stored unit, matching models.ChatRecordCost. The casts keep Postgres from
// typing the price parameters as integers and also work on SQLite.
func chatRecordListCostSQL(vendor string, p models.ModelPrice) (string, []any) {
	// Bill cache tokens the way the gateway does when it records them.
	cacheWritePT, cacheReadPT := models.CachePricesFor(vendor, p.CPIT, p.CacheWritePT, p.CacheReadPT)
	return `((COALESCE(response_tokens, 0) * CAST(? AS DOUBLE PRECISION) +
			COALESCE(prompt_tokens, 0) * CAST(? AS DOUBLE PRECISION) +
			COALESCE(cache_write_prompt_tokens, 0) * CAST(? AS DOUBLE PRECISION) +
			COALESCE(cache_read_prompt_tokens, 0) * CAST(? AS DOUBLE PRECISION)) * 10000)`,
		[]any{p.CPT, p.CPIT, cacheWritePT, cacheReadPT}
}

// UpdateModelPrice updates an existing model price
func (s *Service) UpdateModelPrice(id uint, modelName, vendor string, cpt, cpit, cacheWritePT, cacheReadPT float64, currency string) (*models.ModelPrice, error) {
	modelPrice, err := s.GetModelPriceByID(id)
	if err != nil {
		return nil, err
	}

	modelPrice.ModelName = modelName
	modelPrice.Vendor = vendor
	modelPrice.CPT = cpt
	modelPrice.CPIT = cpit
	modelPrice.CacheWritePT = cacheWritePT
	modelPrice.CacheReadPT = cacheReadPT
	modelPrice.Currency = currency

	if err := modelPrice.Update(s.DB); err != nil {
		return nil, err
	}

	return modelPrice, nil
}

// UpdateModelPriceAndRecalculate updates an existing model price and recalculates all associated chat record costs
func (s *Service) UpdateModelPriceAndRecalculate(id uint, modelName, vendor string, cpt, cpit, cacheWritePT, cacheReadPT float64, currency string) (*models.ModelPrice, error) {
	var modelPrice *models.ModelPrice

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		// Get existing model price
		mp := &models.ModelPrice{}
		if err = mp.Get(tx, id); err != nil {
			return err
		}
		// The records being repriced were priced at this row only if it keeps
		// its model and vendor; after a rename they were not priced by it.
		var old models.ModelPrice
		if mp.ModelName == modelName && mp.Vendor == vendor {
			old = *mp
		}

		// Update model price fields
		mp.ModelName = modelName
		mp.Vendor = vendor
		mp.CPT = cpt
		mp.CPIT = cpit
		mp.CacheWritePT = cacheWritePT
		mp.CacheReadPT = cacheReadPT
		mp.Currency = currency

		// Save the updated model price
		if err = mp.Update(tx); err != nil {
			return err
		}

		// Recalculate costs and currency for all associated chat records
		if err = s.recalculateChatRecordCosts(tx, modelName, vendor, old, *mp); err != nil {
			return err
		}

		modelPrice = mp
		return nil
	})

	if err != nil {
		return nil, err
	}
	s.spendRecalculated()

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
func (s *Service) GetAllModelPrices(pageSize int, pageNumber int, all bool, opts ...ListOptions) (models.ModelPrices, int64, int, error) {
	var modelPrices models.ModelPrices
	totalCount, totalPages, err := modelPrices.GetAll(s.DB, pageSize, pageNumber, all,
		firstListOptions(opts).Scopes("model_name", "vendor")...)
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

// GetOrCreateModelPriceByName retrieves a model price by its name, creating it with default values if it doesn't exist
func (s *Service) GetOrCreateModelPriceByName(modelName string) (*models.ModelPrice, error) {
	modelPrice := &models.ModelPrice{}
	err := modelPrice.GetOrCreateByModelName(s.DB, modelName)
	if err != nil {
		return nil, err
	}
	return modelPrice, nil
}

// GetModelPriceByModelNameAndVendor implements ServiceInterface
func (s *Service) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	modelPrice := &models.ModelPrice{}

	// Use a transaction to ensure atomicity
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		// Try to find existing record
		if err := modelPrice.GetByModelNameAndVendor(tx, modelName, vendor); err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}

			// Not found, create new record
			if modelName == "" {
				modelName = fmt.Sprintf("%s_%s", vendor, "default")
			}
			modelPrice.ModelName = modelName
			modelPrice.Vendor = vendor
			modelPrice.CPT = 0.0
			modelPrice.CPIT = 0.0
			modelPrice.CacheWritePT = 0.0
			modelPrice.CacheReadPT = 0.0
			modelPrice.Currency = "USD"

			// Try to create, if we get a unique constraint error, it means another
			// process created the record in the meantime, so try to get it again
			if err := modelPrice.Create(tx); err != nil {
				if err.Error() == "Error 1062: Duplicate entry" {
					return modelPrice.GetByModelNameAndVendor(tx, modelName, vendor)
				}
				return err
			}
		}
		return nil
	})

	if err != nil {
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
