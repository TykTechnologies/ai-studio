package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func edgeFilterIDs(filters []Filter) []uint {
	ids := make([]uint, len(filters))
	for i, f := range filters {
		ids[i] = f.ID
	}
	return ids
}

// The hub sends FilterIds in the arranged order and the sync writes the
// position into llm_filters.order_index; every read of an LLM's filters must
// honour it, because GORM's preload returns them in id order.
func TestRepository_LLMFiltersOrderedByOrderIndex(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	a := &Filter{Name: "A", Script: "output := {}", IsActive: true}
	b := &Filter{Name: "B", Script: "output := {}", IsActive: true}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Create(b).Error)
	require.Greater(t, b.ID, a.ID)

	llm := &LLM{Name: "Ordered", Slug: "ordered", Vendor: "openai", Endpoint: "https://x", IsActive: true}
	require.NoError(t, repo.CreateLLM(llm))
	require.NoError(t, db.Create(&LLMFilter{LLMID: llm.ID, FilterID: b.ID, IsActive: true, OrderIndex: 0}).Error)
	require.NoError(t, db.Create(&LLMFilter{LLMID: llm.ID, FilterID: a.ID, IsActive: true, OrderIndex: 1}).Error)

	want := []uint{b.ID, a.ID}

	got, err := repo.GetLLM(llm.ID)
	require.NoError(t, err)
	require.Equal(t, want, edgeFilterIDs(got.Filters), "GetLLM")

	got, err = repo.GetLLMBySlug("ordered")
	require.NoError(t, err)
	require.Equal(t, want, edgeFilterIDs(got.Filters), "GetLLMBySlug")

	list, _, err := repo.ListLLMs(1, 10, "", true)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, want, edgeFilterIDs(list[0].Filters), "ListLLMs")

	active, err := repo.GetActiveLLMs()
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, want, edgeFilterIDs(active[0].Filters), "GetActiveLLMs")
}
