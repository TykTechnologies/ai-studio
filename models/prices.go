package models

import (
	"fmt"
	"slices"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
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

// CachePricesFor returns the per-token prices to bill a vendor's cache write
// and cache read tokens at.
//
// OpenAI's prompt_tokens and Google's promptTokenCount already include the
// cache tokens, so the gateway carves them out of the prompt count and bills
// them separately. Before that split (OpenAI until 2.2.1) they were billed as
// ordinary input tokens. Cache prices default to 0, and most price rows never
// set them, so for these vendors a cache price of 0 means "not set" and the
// tokens fall back to the input price: an unset cache price must never make
// cached tokens free. Other vendors report cache tokens apart from the prompt
// count, so their cache prices are used as set.
//
// The analytics cost breakdown (analytics.costBreakdownColumns) applies the
// same rule in SQL, so change both together; the analytics test
// TestCostBreakdown_MatchesChatRecordCost fails when they drift.
func CachePricesFor(vendor string, cpit, cacheWritePT, cacheReadPT float64) (write, read float64) {
	if !slices.Contains(CacheFallbackVendors, vendor) {
		return cacheWritePT, cacheReadPT
	}
	if cacheWritePT == 0 {
		cacheWritePT = cpit
	}
	if cacheReadPT == 0 {
		cacheReadPT = cpit
	}
	return cacheWritePT, cacheReadPT
}

// CacheFallbackVendors are the vendors whose unset (zero) cache prices bill at
// the input price; see CachePricesFor.
var CacheFallbackVendors = []string{string(OPENAI), string(GOOGLEAI)}

// PromptTokensExcludingCache returns the prompt token count to record for a
// vendor's response: OpenAI's and Google's prompt counts include the cache
// tokens (see CachePricesFor), so they are subtracted to avoid counting them
// twice in TotalTokens and in ChatRecordCost. Other vendors report them apart.
func PromptTokensExcludingCache(vendor string, promptTokens, cacheWriteTokens, cacheReadTokens int) int {
	if !slices.Contains(CacheFallbackVendors, vendor) {
		return promptTokens
	}
	return promptTokens - cacheWriteTokens - cacheReadTokens
}

// ChatRecordCost is the cost the gateway stores on a chat record at list
// price, in currency units x 10000, before any price multiplier
// (IPriceAdjustedResponse) is applied.
func ChatRecordCost(vendor string, cpt, cpit, cacheWritePT, cacheReadPT float64, promptTokens, responseTokens, cacheWriteTokens, cacheReadTokens int) float64 {
	cacheWritePT, cacheReadPT = CachePricesFor(vendor, cpit, cacheWritePT, cacheReadPT)
	return ((cpt * float64(responseTokens)) +
		(cpit * float64(promptTokens)) +
		(cacheWritePT * float64(cacheWriteTokens)) +
		(cacheReadPT * float64(cacheReadTokens))) * 10000
}

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

// Delete a ModelPrice. The delete is permanent: a soft-deleted row would
// keep its (model_name, vendor) in idx_model_vendor, so the price could
// never be created again and the gateway's auto-create would fail on it.
func (mp *ModelPrice) Delete(db *gorm.DB) error {
	return db.Unscoped().Delete(mp).Error
}

// PurgeSoftDeletedModelPrices permanently removes the price rows earlier
// versions soft-deleted, which block their (model_name, vendor) in
// idx_model_vendor (see Delete).
func PurgeSoftDeletedModelPrices(db *gorm.DB) error {
	return db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&ModelPrice{}).Error
}

// GetAll retrieves all ModelPrices
func (mps *ModelPrices) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool, scopes ...func(*gorm.DB) *gorm.DB) (int64, int, error) {
	var totalCount int64
	query := db.Model(&ModelPrice{})

	// Optional search/sort scopes (services.ListOptions); applied before the
	// count so X-Total-Count reflects the filtered set.
	for _, scope := range scopes {
		query = scope(query)
	}
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

// DeleteMultiple deletes multiple ModelPrices at once, permanently (see Delete)
func (mps *ModelPrices) DeleteMultiple(db *gorm.DB) error {
	return db.Unscoped().Delete(mps).Error
}
