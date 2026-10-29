package services

import (
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// ModelPriceExistsError is returned when creating a price for a model and
// vendor that already have a priced row.
type ModelPriceExistsError struct {
	ID        uint
	ModelName string
	Vendor    string
}

func (e *ModelPriceExistsError) Error() string {
	return fmt.Sprintf("a price for model %q and vendor %q already exists (model price %d); update that price instead",
		e.ModelName, e.Vendor, e.ID)
}

// CreateModelPrice creates a new model price; see CreateOrFillModelPrice.
func (s *Service) CreateModelPrice(modelName, vendor string, cpt, cpit, cacheWritePT, cacheReadPT float64, currency string) (*models.ModelPrice, error) {
	modelPrice, _, err := s.CreateOrFillModelPrice(modelName, vendor, cpt, cpit, cacheWritePT, cacheReadPT, currency)
	return modelPrice, err
}

// CreateOrFillModelPrice creates a model price. Traffic auto-creates an
// unpriced (all zero) row for every model it sees, so when the model and
// vendor already have one, it is filled in instead (filled is true). When
// they have a priced row, it returns a *ModelPriceExistsError naming it.
func (s *Service) CreateOrFillModelPrice(modelName, vendor string, cpt, cpit, cacheWritePT, cacheReadPT float64, currency string) (modelPrice *models.ModelPrice, filled bool, err error) {
	prices := map[string]any{"cpt": cpt, "cpit": cpit, "cache_write_pt": cacheWritePT, "cache_read_pt": cacheReadPT, "currency": currency}
	// Twice: a create that loses a race to another (the gateway's
	// auto-create, say) finds the row on the second pass.
	for attempt := 0; attempt < 2; attempt++ {
		existing := &models.ModelPrice{}
		err = existing.GetByModelNameAndVendor(s.DB, modelName, vendor)
		if err == nil {
			// Fill only while still unpriced, so a price set concurrently is
			// not overwritten.
			res := s.DB.Model(existing).
				Where("cpt = 0 AND cpit = 0 AND cache_write_pt = 0 AND cache_read_pt = 0").
				Updates(prices)
			if res.Error != nil {
				return nil, false, res.Error
			}
			if res.RowsAffected == 0 {
				return nil, false, &ModelPriceExistsError{ID: existing.ID, ModelName: modelName, Vendor: vendor}
			}
			modelPrice, err = s.GetModelPriceByID(existing.ID)
			return modelPrice, err == nil, err
		}
		if err != gorm.ErrRecordNotFound {
			return nil, false, err
		}

		modelPrice = &models.ModelPrice{
			ModelName:    modelName,
			Vendor:       vendor,
			CPT:          cpt,
			CPIT:         cpit,
			CacheWritePT: cacheWritePT,
			CacheReadPT:  cacheReadPT,
			Currency:     currency,
		}
		err = modelPrice.Create(s.DB)
		if err == nil {
			return modelPrice, false, nil
		}
		if !models.IsUniqueViolation(err) {
			return nil, false, err
		}
	}
	return nil, false, err
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
// It reprices the rows the gateway prices with this row: those recorded
// under the model's name, and those the caller asked for by this name that
// the vendor reported under another (a versioned name) whose own price row
// is unpriced, since the gateway then falls back to the requested name's
// price (proxy priceForExchange). Rows recorded before the requested model
// was stored (and edge rows) match by name only.
//
// Each row is repriced at its stored price multiplier
// (models.IPriceAdjustedResponse, e.g. 0.5x for batch), so a batch row stays
// at that multiplier, and a row recorded at list price gets exactly the new
// list price. A row without one (older and edge rows) keeps the ratio of its
// recorded cost to its list cost at the old price, and the ratio is stored
// as its multiplier so that it survives a later recalculation through a zero
// price. A row with neither (no old list cost: no price before, or old is
// the zero value; or no recorded cost) gets the new list price at 1x.
func (s *Service) recalculateChatRecordCosts(tx *gorm.DB, modelName, vendor string, old, updated models.ModelPrice) error {
	oldList, oldArgs := chatRecordListCostSQL(vendor, old)
	newList, newArgs := chatRecordListCostSQL(vendor, updated)

	ratio := `NULLIF(cost, 0) / NULLIF(` + oldList + `, 0)`
	pricedBy, pricedByArgs := chatRecordsPricedBySQL(modelName, vendor)
	args := append(append(append(append(newArgs, oldArgs...), oldArgs...), updated.Currency), pricedByArgs...)
	result := tx.Exec(`
		UPDATE llm_chat_records
		SET cost = `+newList+` * COALESCE(price_multiplier, `+ratio+`, 1),
		price_multiplier = COALESCE(price_multiplier, `+ratio+`),
		currency = ?
		WHERE `+pricedBy,
		args...,
	)
	return result.Error
}

// chatRecordsPricedBySQL selects, in a WHERE clause on llm_chat_records, the
// rows the gateway prices with the price row of modelName and vendor: those
// recorded under that name, and those requested by that name that the vendor
// reported under another (a versioned name) whose own price row is unpriced
// (proxy priceForExchange).
func chatRecordsPricedBySQL(modelName, vendor string) (string, []any) {
	return `vendor = ? AND (name = ? OR (
			requested_model = ? AND name <> requested_model AND NOT EXISTS (
				SELECT 1 FROM model_prices mp
				WHERE mp.model_name = llm_chat_records.name AND mp.vendor = llm_chat_records.vendor
				AND mp.deleted_at IS NULL
				AND (mp.cpt <> 0 OR mp.cpit <> 0 OR mp.cache_write_pt <> 0 OR mp.cache_read_pt <> 0))))`,
		[]any{vendor, modelName, modelName}
}

// unpricedChatRecordSQL selects the rows recorded with no cost although they
// used tokens: traffic from before the model had a price.
const unpricedChatRecordSQL = `cost = 0 AND total_tokens > 0`

// BackfillUnpricedHistory prices the history a model price row never priced:
// the chat records it prices (see chatRecordsPricedBySQL) that were recorded
// at no cost, optionally only those with a time stamp in [from, to). Each is
// priced at the row's current price and multiplier, and takes its currency.
// Rows with a cost are left alone (PATCH /model-prices/:id/recalculate
// reprices them). It returns the number of rows priced.
func (s *Service) BackfillUnpricedHistory(id uint, from, to *time.Time) (int64, error) {
	mp, err := s.GetModelPriceByID(id)
	if err != nil {
		return 0, err
	}
	list, listArgs := chatRecordListCostSQL(mp.Vendor, *mp)
	pricedBy, pricedByArgs := chatRecordsPricedBySQL(mp.ModelName, mp.Vendor)
	query := `UPDATE llm_chat_records
		SET cost = ` + list + ` * COALESCE(price_multiplier, 1), currency = ?
		WHERE ` + unpricedChatRecordSQL + ` AND ` + pricedBy
	args := append(append(listArgs, mp.Currency), pricedByArgs...)
	if from != nil {
		query += ` AND time_stamp >= ?`
		args = append(args, *from)
	}
	if to != nil {
		query += ` AND time_stamp < ?`
		args = append(args, *to)
	}
	result := s.DB.Exec(query, args...)
	if result.Error != nil {
		return 0, result.Error
	}
	// Budget spend in memory was summed from the old (zero) costs.
	if result.RowsAffected > 0 {
		s.spendRecalculated()
	}
	return result.RowsAffected, nil
}

// CountUnpricedHistory returns, for each of the given model price rows, the
// number of chat records it prices that were recorded at no cost (what
// BackfillUnpricedHistory would price). Rows with none are left out. It
// applies chatRecordsPricedBySQL's rule to one grouped scan of the unpriced
// records, rather than a scan per price row.
func (s *Service) CountUnpricedHistory(prices models.ModelPrices) (map[uint]int64, error) {
	var groups []struct {
		Name, RequestedModel, Vendor string
		N                            int64
	}
	if err := s.DB.Model(&models.LLMChatRecord{}).
		Select("name, COALESCE(requested_model, '') AS requested_model, vendor, COUNT(*) AS n").
		Where(unpricedChatRecordSQL).
		Group("name, COALESCE(requested_model, ''), vendor").
		Scan(&groups).Error; err != nil {
		return nil, err
	}
	var priced []struct{ ModelName, Vendor string }
	if err := s.DB.Model(&models.ModelPrice{}).
		Where("cpt <> 0 OR cpit <> 0 OR cache_write_pt <> 0 OR cache_read_pt <> 0").
		Select("model_name, vendor").Scan(&priced).Error; err != nil {
		return nil, err
	}
	type key struct{ name, vendor string }
	isPriced := map[key]bool{}
	for _, p := range priced {
		isPriced[key{p.ModelName, p.Vendor}] = true
	}

	counts := map[uint]int64{}
	for _, mp := range prices {
		for _, g := range groups {
			if g.Vendor != mp.Vendor {
				continue
			}
			if g.Name == mp.ModelName ||
				(g.RequestedModel == mp.ModelName && g.Name != g.RequestedModel && !isPriced[key{g.Name, g.Vendor}]) {
				counts[mp.ID] += g.N
			}
		}
	}
	return counts, nil
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

// GetModelPriceByModelNameAndVendor implements ServiceInterface. A model
// without a price row gets an unpriced (all zero) one.
func (s *Service) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	if modelName == "" {
		modelName = fmt.Sprintf("%s_%s", vendor, "default")
	}
	modelPrice := &models.ModelPrice{}
	err := modelPrice.GetByModelNameAndVendor(s.DB, modelName, vendor)
	if err != gorm.ErrRecordNotFound {
		if err != nil {
			return nil, err
		}
		return modelPrice, nil
	}

	modelPrice = &models.ModelPrice{ModelName: modelName, Vendor: vendor, Currency: "USD"}
	if err := modelPrice.Create(s.DB); err != nil {
		if !models.IsUniqueViolation(err) {
			return nil, err
		}
		// Another request created it in the meantime. Read it outside any
		// transaction: on Postgres a failed statement aborts the one it is in.
		modelPrice = &models.ModelPrice{}
		if err := modelPrice.GetByModelNameAndVendor(s.DB, modelName, vendor); err != nil {
			return nil, err
		}
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
