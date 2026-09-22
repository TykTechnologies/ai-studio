package models

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// filterIDs returns the ids of an LLM's filters in slice order.
func filterIDs(filters []*Filter) []uint {
	ids := make([]uint, len(filters))
	for i, f := range filters {
		ids[i] = f.ID
	}
	return ids
}

// orderIndexes reads llm_filters.order_index keyed by filter id for one LLM.
func orderIndexes(t *testing.T, db *gorm.DB, llmID uint) map[uint]int {
	t.Helper()
	var rows []LLMFilter
	require.NoError(t, db.Where("llm_id = ?", llmID).Find(&rows).Error)
	out := map[uint]int{}
	for _, r := range rows {
		out[r.FilterID] = r.OrderIndex
	}
	return out
}

// The admin arranges the filter chain in the LLM form; the arranged order is
// what the gateway must execute, so it has to survive a round trip through
// the many2many join. GORM's preload returns filters in id order, which is
// why this test attaches the higher-id filter first.
func TestLLM_FilterOrderIsPersisted(t *testing.T) {
	db := setupTestDB(t)

	a := &Filter{Name: "A", Script: []byte(`output := {block: false}`)}
	b := &Filter{Name: "B", Script: []byte(`output := {block: false}`)}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Create(b).Error)
	require.Greater(t, b.ID, a.ID, "B must have the higher id for the test to be meaningful")

	llm := &LLM{Name: "ordered", Vendor: OPENAI, Active: true, Filters: []*Filter{b, a}}
	require.NoError(t, llm.Create(db))
	require.Equal(t, map[uint]int{b.ID: 0, a.ID: 1}, orderIndexes(t, db, llm.ID))

	t.Run("Get", func(t *testing.T) {
		got := &LLM{}
		require.NoError(t, got.Get(db, llm.ID))
		require.Equal(t, []uint{b.ID, a.ID}, filterIDs(got.Filters))
	})

	t.Run("GetByName", func(t *testing.T) {
		got := &LLM{}
		require.NoError(t, got.GetByName(db, "ordered"))
		require.Equal(t, []uint{b.ID, a.ID}, filterIDs(got.Filters))
	})

	t.Run("GetAll", func(t *testing.T) {
		var all LLMs
		_, _, err := all.GetAll(db, 0, 0, true)
		require.NoError(t, err)
		require.Len(t, all, 1)
		require.Equal(t, []uint{b.ID, a.ID}, filterIDs(all[0].Filters))
	})

	t.Run("GetActiveLLMs", func(t *testing.T) {
		var active LLMs
		require.NoError(t, active.GetActiveLLMs(db))
		require.Len(t, active, 1)
		require.Equal(t, []uint{b.ID, a.ID}, filterIDs(active[0].Filters))
	})

	t.Run("Update rewrites the order", func(t *testing.T) {
		llm.Filters = []*Filter{a, b}
		require.NoError(t, llm.Update(db))
		require.Equal(t, map[uint]int{a.ID: 0, b.ID: 1}, orderIndexes(t, db, llm.ID))

		got := &LLM{}
		require.NoError(t, got.Get(db, llm.ID))
		require.Equal(t, []uint{a.ID, b.ID}, filterIDs(got.Filters))
	})

	t.Run("Update with id-only stubs (the API path)", func(t *testing.T) {
		llm.Filters = []*Filter{{ID: b.ID}, {ID: a.ID}}
		require.NoError(t, llm.Update(db))

		got := &LLM{}
		require.NoError(t, got.Get(db, llm.ID))
		require.Equal(t, []uint{b.ID, a.ID}, filterIDs(got.Filters))
		require.Equal(t, "B", got.Filters[0].Name, "stubs must not blank the filter rows")
	})
}

// Rows from installs that predate order_index all carry 0; ties fall back to
// id order, which is what those installs saw before.
func TestOrderLLMFilters_TiesFallBackToID(t *testing.T) {
	db := setupTestDB(t)

	a := &Filter{Name: "A"}
	b := &Filter{Name: "B"}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Create(b).Error)

	llm := &LLM{Name: "legacy", Vendor: OPENAI, Active: true}
	require.NoError(t, db.Create(llm).Error)
	for _, f := range []*Filter{b, a} {
		require.NoError(t, db.Create(&LLMFilter{LLMID: llm.ID, FilterID: f.ID}).Error)
	}

	got := &LLM{}
	require.NoError(t, got.Get(db, llm.ID))
	require.Equal(t, []uint{a.ID, b.ID}, filterIDs(got.Filters))

	// A no-op on LLMs with nothing to sort.
	require.NoError(t, OrderLLMFilters(db))
	require.NoError(t, OrderLLMFilters(db, &LLM{}))
}

// Existing installs have an llm_filters table without order_index (GORM's
// implicit join table). Registering the join model makes InitModels add the
// column, and the legacy rows read back in id order.
func TestInitModels_AddsOrderIndexToLegacyLLMFilters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE llm_filters (llm_id integer, filter_id integer, PRIMARY KEY (llm_id, filter_id))`).Error)
	require.NoError(t, db.Exec(`INSERT INTO llm_filters (llm_id, filter_id) VALUES (1, 2), (1, 1)`).Error)
	require.False(t, db.Migrator().HasColumn(&LLMFilter{}, "order_index"))

	require.NoError(t, InitModels(db))
	require.True(t, db.Migrator().HasColumn(&LLMFilter{}, "order_index"))

	var rows []LLMFilter
	require.NoError(t, db.Order("filter_id").Find(&rows).Error)
	require.Equal(t, []LLMFilter{{LLMID: 1, FilterID: 1}, {LLMID: 1, FilterID: 2}}, rows)
}
