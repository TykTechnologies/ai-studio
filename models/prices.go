package models

import (
	"fmt"

	"gorm.io/gorm"
)

type ModelPrice struct {
	gorm.Model

	ID           uint    `gorm:"primaryKey"`
	ModelName    string  `gorm:"uniqueIndex:idx_model_vendor" json:"model_name"`
	Vendor       string  `gorm:"uniqueIndex:idx_model_vendor" json:"vendor"`
	// All four prices are PER TOKEN, not per million tokens.
	//
	// The admin form collects per-million figures and divides by 1e6 before
	// posting, and the UI's own column header says "per million" -- so an API
	// client posting the number straight off a vendor's pricing page stores
	// every figure 1,000,000x too high, silently, and every cost, budget and
	// budget alert downstream is wrong. See MaxPlausiblePerTokenPrice.
	//
	// Note the directions, which the names do not make obvious: CPT is charged
	// against *output* (completion) tokens and CPIT against *input* (prompt)
	// tokens. See proxy/bedrock_translator.go and proxy/analyze_utils.go.
	CPT          float64 `json:"cpt"`            // Price per OUTPUT (completion) token
	CPIT         float64 `json:"cpit"`           // Price per INPUT (prompt) token
	CacheWritePT float64 `json:"cache_write_pt"` // Price per token for cache writes
	CacheReadPT  float64 `json:"cache_read_pt"`  // Price per token for cache reads
	Currency     string  `json:"currency"`
}

type ModelPrices []ModelPrice

// MaxPlausiblePerTokenPrice is the ceiling above which a submitted price is
// almost certainly a unit error rather than a real figure.
//
// The most expensive models on the market are around $100 per million output
// tokens, i.e. 1e-4 per token. A value above 0.01 per token would be $10,000
// per million -- two orders of magnitude beyond anything real, and exactly
// what you get by posting a per-million figure into a per-token field.
const MaxPlausiblePerTokenPrice = 0.01

// ImplausiblePerTokenPrice reports whether a price looks like a per-million
// figure submitted into a per-token field, and names the field if so.
func ImplausiblePerTokenPrice(field string, value float64) (bool, string) {
	if value <= MaxPlausiblePerTokenPrice {
		return false, ""
	}
	return true, fmt.Sprintf(
		"%s is %g per token, which is %.0f per million tokens. Prices are stored per token; "+
			"if you meant %g per million tokens, submit %g.",
		field, value, value*1_000_000, value, value/1_000_000,
	)
}

// Create a new ModelPrice
func (mp *ModelPrice) Create(db *gorm.DB) error {
	return db.Create(mp).Error
}

// Get a ModelPrice by ID
func (mp *ModelPrice) Get(db *gorm.DB, id uint) error {
	return db.First(mp, id).Error
}

// Update an existing ModelPrice
func (mp *ModelPrice) Update(db *gorm.DB) error {
	return db.Save(mp).Error
}

// Delete a ModelPrice
func (mp *ModelPrice) Delete(db *gorm.DB) error {
	return db.Delete(mp).Error
}

// GetAll retrieves all ModelPrices
func (mps *ModelPrices) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&ModelPrice{})

	if err := query.Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Find(mps).Error
	return totalCount, totalPages, err
}

// GetByVendor retrieves all ModelPrices for a specific vendor
func (mps *ModelPrices) GetByVendor(db *gorm.DB, vendor string) error {
	return db.Where("vendor = ?", vendor).Find(mps).Error
}

// GetByModelName retrieves a ModelPrice by its model name
func (mp *ModelPrice) GetByModelName(db *gorm.DB, modelName string) error {
	return db.Where("model_name = ?", modelName).First(mp).Error
}

// GetOrCreateByModelName retrieves a ModelPrice by its model name, or creates it if not found
func (mp *ModelPrice) GetOrCreateByModelName(db *gorm.DB, modelName string) error {
	err := mp.GetByModelName(db, modelName)
	if err == gorm.ErrRecordNotFound {
		// Initialize new model price with default values
		mp.ModelName = modelName
		mp.CPT = 0.0          // Default CPT
		mp.CPIT = 0.0         // Default CPIT
		mp.CacheWritePT = 0.0 // Default cache write price per token
		mp.CacheReadPT = 0.0  // Default cache read price per token
		mp.Currency = "USD"   // Default currency
		return mp.Create(db)
	}
	return err
}

// GetByModelNameAndVendor retrieves a ModelPrice by its model name and vendor
func (mp *ModelPrice) GetByModelNameAndVendor(db *gorm.DB, modelName string, vendor string) error {
	return db.Where("model_name = ? AND vendor = ?", modelName, vendor).First(mp).Error
}

// CreateMultiple creates multiple ModelPrices at once
func (mps *ModelPrices) CreateMultiple(db *gorm.DB) error {
	return db.Create(mps).Error
}

// UpdateMultiple updates multiple ModelPrices at once
func (mps *ModelPrices) UpdateMultiple(db *gorm.DB) error {
	for _, mp := range *mps {
		if err := db.Save(&mp).Error; err != nil {
			return err
		}
	}
	return nil
}

// DeleteMultiple deletes multiple ModelPrices at once
func (mps *ModelPrices) DeleteMultiple(db *gorm.DB) error {
	return db.Delete(mps).Error
}
