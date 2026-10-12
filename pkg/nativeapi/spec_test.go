package nativeapi

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

func TestUsageIsPriceAdjustedTokenResponse(t *testing.T) {
	var r models.ITokenResponse = Usage{Model: "claude-x", InputTokens: 10, OutputTokens: 5,
		CacheCreationInputTokens: 3, CacheReadInputTokens: 2, Tier: "batch", Multiplier: 0.5, Billed: true}
	pa, ok := r.(models.IPriceAdjustedResponse)
	if !ok {
		t.Fatal("Usage does not implement IPriceAdjustedResponse")
	}
	if pa.GetModel() != "claude-x" || pa.GetPromptTokens() != 10 || pa.GetResponseTokens() != 5 ||
		pa.GetCacheWritePromptTokens() != 3 || pa.GetCacheReadPromptTokens() != 2 || pa.GetChoiceCount() != 1 {
		t.Fatalf("token getters wrong: %+v", pa)
	}
	if pa.ServiceTier() != "batch" || pa.PriceMultiplier() != 0.5 {
		t.Fatalf("tier/multiplier = %q %v", pa.ServiceTier(), pa.PriceMultiplier())
	}
	if (Usage{}).PriceMultiplier() != 1 {
		t.Fatal("zero multiplier must mean list price")
	}
}

func TestSpecResource(t *testing.T) {
	s := batchSpec()
	if rt, ok := s.Resource(testBatch.Name); !ok || rt.Name != testBatch.Name {
		t.Fatal("Resource lookup failed")
	}
	if _, ok := s.Resource("nope"); ok {
		t.Fatal("unknown resource found")
	}
}

func TestLedgerEffectString(t *testing.T) {
	if LedgerCreate.String() != "create" || LedgerEffect(99).String() != "LedgerEffect(99)" {
		t.Fatal("String")
	}
}
