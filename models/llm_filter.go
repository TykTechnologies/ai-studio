package models

import (
	"sort"

	"gorm.io/gorm"
)

// LLMFilter is the llm_filters join row. OrderIndex is the filter's position
// in the LLM's chain as the admin arranged it: filters run top to bottom and
// the first block wins, so the order has to be persisted rather than left to
// whatever order the preload happens to return (id order). The same column
// exists on the edge schema and the snapshot sends FilterIds in this order.
type LLMFilter struct {
	LLMID      uint `json:"llm_id" gorm:"primaryKey"`
	FilterID   uint `json:"filter_id" gorm:"primaryKey"`
	OrderIndex int  `json:"order_index" gorm:"not null;default:0"`
}

func (LLMFilter) TableName() string { return "llm_filters" }

// setupLLMFilterJoin registers the join model so AutoMigrate adds
// order_index to existing installs and the association writes go through it.
func setupLLMFilterJoin(db *gorm.DB) error {
	return db.SetupJoinTable(&LLM{}, "Filters", &LLMFilter{})
}

// writeLLMFilterOrder stores each filter's slice position as order_index.
// Called after Association("Filters").Replace, which has already written the
// join rows; one UPDATE per row is fine for chains this short.
func writeLLMFilterOrder(db *gorm.DB, l *LLM) error {
	for i, f := range l.Filters {
		if f == nil {
			continue
		}
		if err := db.Model(&LLMFilter{}).
			Where("llm_id = ? AND filter_id = ?", l.ID, f.ID).
			Update("order_index", i).Error; err != nil {
			return err
		}
	}
	return nil
}

// OrderLLMFilters sorts each LLM's Filters by the persisted chain order in
// one query. Ties (rows from installs that predate order_index all carry 0)
// fall back to filter id, which is the order those installs saw before.
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

// withFilterOrder runs the ordering step after a successful read.
func withFilterOrder(db *gorm.DB, err error, llms ...*LLM) error {
	if err != nil {
		return err
	}
	return OrderLLMFilters(db, llms...)
}

// withFilterListOrder is withFilterOrder for LLMs.
func withFilterListOrder(db *gorm.DB, err error, llms *LLMs) error {
	if err != nil {
		return err
	}
	return OrderLLMFilterList(db, *llms)
}
