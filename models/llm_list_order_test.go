package models

import (
	"strings"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lsj.27: the paged LLM list is in id order unless a sort scope orders it;
// Postgres otherwise returns heap order, which an UPDATE (the 2.3 credential
// backfill) changes, so page 1 showed a different set after an upgrade.
func TestLLMsGetAll_OrdersByIDWithoutASort(t *testing.T) {
	db := setupTestDB(t)
	var mu sync.Mutex
	var selects []string
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("lsj27:capture", func(tx *gorm.DB) {
		if tx.Statement.Table == "llms" {
			mu.Lock()
			selects = append(selects, tx.Statement.SQL.String())
			mu.Unlock()
		}
	}))
	for _, name := range []string{"c", "a", "b"} {
		require.NoError(t, db.Create(&LLM{Name: name, Vendor: OPENAI}).Error)
	}

	var page LLMs
	_, _, err := page.GetAll(db, 2, 1, false)
	require.NoError(t, err)
	require.NotEmpty(t, selects)
	assert.Contains(t, selects[len(selects)-1], "ORDER BY id ASC LIMIT")

	selects = nil
	var sorted LLMs
	_, _, err = sorted.GetAll(db, 2, 1, false, func(q *gorm.DB) *gorm.DB { return q.Order("name ASC").Order("id ASC") })
	require.NoError(t, err)
	last := selects[len(selects)-1]
	assert.Contains(t, last, "ORDER BY name ASC,id ASC LIMIT")
	assert.Equal(t, 1, strings.Count(last, "id ASC"), "the sort scope's order is kept as it is: %s", last)
	require.Len(t, sorted, 2)
	assert.Equal(t, []string{"a", "b"}, []string{sorted[0].Name, sorted[1].Name})
}
