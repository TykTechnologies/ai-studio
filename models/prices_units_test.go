package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The admin form collects per-million figures and divides by 1e6 before
// posting, while the API stores per token -- and the UI's own column header
// says "per million". An API client posting the number straight off a vendor's
// pricing page therefore stored every figure 1,000,000x too high, silently,
// and every cost, budget and budget alert downstream was wrong.
func TestImplausiblePerTokenPrice(t *testing.T) {
	t.Run("accepts a realistic per-token price", func(t *testing.T) {
		// $10 per million output tokens.
		bad, _ := ImplausiblePerTokenPrice("cpt", 0.00001)
		assert.False(t, bad)
	})

	t.Run("accepts the most expensive plausible model", func(t *testing.T) {
		// $100 per million tokens is 1e-4 per token, still well under the cap.
		bad, _ := ImplausiblePerTokenPrice("cpt", 0.0001)
		assert.False(t, bad)
	})

	t.Run("accepts zero", func(t *testing.T) {
		bad, _ := ImplausiblePerTokenPrice("cpt", 0)
		assert.False(t, bad)
	})

	t.Run("rejects a per-million figure pasted into a per-token field", func(t *testing.T) {
		// The exact mistake: $10 per million submitted as 10.
		bad, msg := ImplausiblePerTokenPrice("cpt", 10)
		assert.True(t, bad)
		assert.Contains(t, msg, "per token")
		assert.Contains(t, msg, "10000000")   // what it would mean per million
		assert.Contains(t, msg, "1e-05")      // what they probably meant
	})

	t.Run("names the offending field", func(t *testing.T) {
		bad, msg := ImplausiblePerTokenPrice("cpit (price per input token)", 3)
		assert.True(t, bad)
		assert.Contains(t, msg, "cpit (price per input token)")
	})
}

// The names do not make the directions obvious, and nothing enforced them.
// proxy/analyze_utils.go and proxy/bedrock_translator.go charge CPT against
// completion tokens and CPIT against prompt tokens.
func TestModelPriceFieldDirectionsAreDocumented(t *testing.T) {
	price := ModelPrice{CPT: 0.00003, CPIT: 0.00001}

	promptTokens, completionTokens := 1000.0, 500.0
	cost := price.CPT*completionTokens + price.CPIT*promptTokens

	// 0.00003*500 + 0.00001*1000 = 0.015 + 0.01
	assert.InDelta(t, 0.025, cost, 1e-9,
		"CPT is charged against output tokens and CPIT against input tokens")
}
