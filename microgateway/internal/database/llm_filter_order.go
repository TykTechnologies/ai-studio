package database

import (
	"sort"

	"gorm.io/gorm"
)

// OrderLLMFilters sorts each LLM's Filters by llm_filters.order_index in one
// query. The hub sends FilterIds in the arranged chain order and the sync
// stores the position as order_index; GORM's preload returns the filters in
// id order, so every read has to re-sort. Ties fall back to filter id.
func OrderLLMFilters(db *gorm.DB, llms ...*LLM) error {
	ids := make([]uint, 0, len(llms))
	for _, l := range llms {
		if l != nil && len(l.Filters) > 1 {
			ids = append(ids, l.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	var rows []LLMFilter
	if err := db.Session(&gorm.Session{NewDB: true}).
		Where("llm_id IN ?", ids).Find(&rows).Error; err != nil {
		return err
	}
	position := make(map[uint]map[uint]int, len(ids))
	for _, r := range rows {
		if position[r.LLMID] == nil {
			position[r.LLMID] = map[uint]int{}
		}
		position[r.LLMID][r.FilterID] = r.OrderIndex
	}

	for _, l := range llms {
		if l == nil || len(l.Filters) < 2 {
			continue
		}
		pos := position[l.ID]
		sort.SliceStable(l.Filters, func(i, j int) bool {
			pi, pj := pos[l.Filters[i].ID], pos[l.Filters[j].ID]
			if pi != pj {
				return pi < pj
			}
			return l.Filters[i].ID < l.Filters[j].ID
		})
	}
	return nil
}

// OrderLLMFilterList is OrderLLMFilters for a value slice, as Find returns.
func OrderLLMFilterList(db *gorm.DB, llms []LLM) error {
	ptrs := make([]*LLM, len(llms))
	for i := range llms {
		ptrs[i] = &llms[i]
	}
	return OrderLLMFilters(db, ptrs...)
}
