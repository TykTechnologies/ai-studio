package proxy

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

func newCachedTool(id uint, updatedAt time.Time, spec, operations string) *models.Tool {
	return &models.Tool{
		Model:               gorm.Model{ID: id, UpdatedAt: updatedAt},
		ID:                  id,
		Name:                "Currency Exchange Rates",
		OASSpec:             spec,
		AvailableOperations: operations,
	}
}

func TestToolCacheVersion(t *testing.T) {
	p := &Proxy{}
	now := time.Now()
	base := newCachedTool(4, now, "c3BlYy12MQ==", "getExchangeRate")

	t.Run("stable for an unchanged tool", func(t *testing.T) {
		if p.toolCacheVersion(base) != p.toolCacheVersion(newCachedTool(4, now, "c3BlYy12MQ==", "getExchangeRate")) {
			t.Fatal("an unchanged tool must keep its version, or the cache never hits")
		}
	})

	t.Run("changes when the tool is updated", func(t *testing.T) {
		if p.toolCacheVersion(base) == p.toolCacheVersion(newCachedTool(4, now.Add(time.Second), "c3BlYy12MQ==", "getExchangeRate")) {
			t.Fatal("a newer UpdatedAt must produce a different version")
		}
	})

	t.Run("changes when the spec is edited", func(t *testing.T) {
		// The reported case: an edited OpenAPI document whose operation names
		// are unchanged. UpdatedAt alone covers this on the control plane, but
		// hashing the spec makes it hold even where timestamps are unreliable.
		if p.toolCacheVersion(base) == p.toolCacheVersion(newCachedTool(4, now, "c3BlYy12Mg==", "getExchangeRate")) {
			t.Fatal("an edited spec must produce a different version")
		}
	})

	t.Run("changes when operations are whitelisted or removed", func(t *testing.T) {
		if p.toolCacheVersion(base) == p.toolCacheVersion(newCachedTool(4, now, "c3BlYy12MQ==", "getExchangeRate,getExchangeRates")) {
			t.Fatal("a changed operation list must produce a different version")
		}
	})

	t.Run("distinguishes tools with zero timestamps", func(t *testing.T) {
		// An edge builds models.Tool by hand from local SQLite. If a converter
		// omits UpdatedAt, both tools carry the zero time — the version must
		// still tell a deleted tool apart from its same-named replacement.
		deleted := newCachedTool(4, time.Time{}, "c3BlYy12MQ==", "getExchangeRate")
		recreated := newCachedTool(5, time.Time{}, "c3BlYy12MQ==", "getExchangeRate")
		if p.toolCacheVersion(deleted) == p.toolCacheVersion(recreated) {
			t.Fatal("tools with different IDs must not share a cache version")
		}
	})
}

func TestGenerateOperationHash_EmptyWithoutOperations(t *testing.T) {
	p := &Proxy{}
	if got := p.generateOperationHash(newCachedTool(4, time.Now(), "c3BlYw==", "")); got != "" {
		t.Fatalf("expected empty hash for a tool with no operations, got %q", got)
	}
}
