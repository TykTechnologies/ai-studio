package models

import (
	"database/sql/driver"
	"fmt"
	"math"
	"regexp"
)

// LLMPriceMultipliers overrides, for one LLM, the share of the list price a
// vendor service tier is billed at: {"batch": 0.4} for negotiated batch
// pricing. Keys are the tier a priced response reports
// (IPriceAdjustedResponse.ServiceTier: Anthropic "batch", "standard",
// "priority"; OpenAI "flex", "priority"...). A tier without an entry keeps
// the vendor's multiplier. Responses that report no tier are never adjusted.
//
// Stored via Scanner/Valuer like LLMFailover; empty is NULL.
type LLMPriceMultipliers map[string]float64

// MaxPriceMultiplier bounds an override.
const MaxPriceMultiplier = 10

var priceTierPattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// listPriceTiers are tier names vendors use for list-price traffic, which
// the gateway records as no tier (OpenAI "default" and "auto"); an override
// keyed by one could never apply. Refused for every vendor.
var listPriceTiers = map[string]bool{"default": true, "auto": true}

// Validate checks the tier names (lowercase letters, digits, '_' or '-', up
// to 32) and the multipliers (0 to MaxPriceMultiplier).
func (m LLMPriceMultipliers) Validate() error {
	for tier, mult := range m {
		if !priceTierPattern.MatchString(tier) {
			return fmt.Errorf("price_multipliers: tier %q must be 1-32 lowercase letters, digits, '_' or '-'", tier)
		}
		if listPriceTiers[tier] {
			return fmt.Errorf("price_multipliers: tier %q is billed at list price and recorded as no tier, so it has no multiplier to override", tier)
		}
		if math.IsNaN(mult) || mult < 0 || mult > MaxPriceMultiplier {
			return fmt.Errorf("price_multipliers: %q must be between 0 and %d", tier, MaxPriceMultiplier)
		}
	}
	return nil
}

// Multiplier is the multiplier to bill tier at: the override when the LLM
// has one for tier, else vendorDefault.
func (m LLMPriceMultipliers) Multiplier(tier string, vendorDefault float64) float64 {
	if tier != "" {
		if v, ok := m[tier]; ok {
			return v
		}
	}
	return vendorDefault
}

// Scan implements sql.Scanner.
func (m *LLMPriceMultipliers) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}
	return JSONScan(value, m)
}

// Value implements driver.Valuer. No overrides is stored as NULL.
func (m LLMPriceMultipliers) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}
	return JSONValue(m)
}
