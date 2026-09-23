package services

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBudgetSyncHandler_BudgetBlocks(t *testing.T) {
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)
	handler.blocks = &BudgetBlocks{blocks: map[uint]string{}}

	handler.HandleBudgetSync(createBudgetSyncEvent(BudgetSyncPayload{
		ControlTimestamp: time.Now(),
		SequenceNumber:   1,
		Blocks:           map[uint32]string{7: "team monthly budget exceeded"},
		BlocksIncluded:   true,
	}))
	reason, blocked := handler.blocks.Reason(7)
	assert.True(t, blocked)
	assert.Equal(t, "team monthly budget exceeded", reason)

	// A control without team budgets leaves the set alone.
	handler.HandleBudgetSync(createBudgetSyncEvent(BudgetSyncPayload{
		ControlTimestamp: time.Now(),
		SequenceNumber:   2,
		AppUsages:        map[uint32]float64{1: 1},
	}))
	_, blocked = handler.blocks.Reason(7)
	assert.True(t, blocked)

	// A complete, empty set releases the App.
	handler.HandleBudgetSync(createBudgetSyncEvent(BudgetSyncPayload{
		ControlTimestamp: time.Now(),
		SequenceNumber:   3,
		BlocksIncluded:   true,
	}))
	_, blocked = handler.blocks.Reason(7)
	assert.False(t, blocked)
}

func TestBudgetSyncHandler_BlocksSurviveRestart(t *testing.T) {
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)
	handler.blocks = &BudgetBlocks{blocks: map[uint]string{}}
	handler.HandleBudgetSync(createBudgetSyncEvent(BudgetSyncPayload{
		ControlTimestamp: time.Now(),
		SequenceNumber:   1,
		Blocks:           map[uint32]string{4: "app monthly budget is 0"},
		BlocksIncluded:   true,
	}))

	restarted := &BudgetBlocks{blocks: map[uint]string{}}
	loadBudgetBlocks(db, restarted)
	reason, blocked := restarted.Reason(4)
	assert.True(t, blocked)
	assert.Equal(t, "app monthly budget is 0", reason)
}

// A large set is written in batches and survives a restart; an unchanged
// set is not rewritten; a failed write is retried on the next sync.
func TestBudgetBlocks_BatchedAndOnlyOnChange(t *testing.T) {
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)
	handler.blocks = &BudgetBlocks{blocks: map[uint]string{}}

	big := map[uint32]string{}
	for i := uint32(1); i <= 1200; i++ { // > one batch, > SQLite's 999 variables
		big[i] = "app monthly budget is 0"
	}
	seq := uint64(0)
	sync := func(blocks map[uint32]string) {
		seq++
		handler.HandleBudgetSync(createBudgetSyncEvent(BudgetSyncPayload{
			ControlTimestamp: time.Now(), SequenceNumber: seq, Blocks: blocks, BlocksIncluded: true,
		}))
	}

	var inserts int64
	require.NoError(t, db.Callback().Create().After("gorm:create").Register("test:count_inserts", func(*gorm.DB) {
		atomic.AddInt64(&inserts, 1)
	}))

	sync(big)
	var n int64
	require.NoError(t, db.Model(&database.BudgetBlock{}).Count(&n).Error)
	assert.EqualValues(t, 1200, n)
	// 1200/budgetBlockBatch batches, plus the handler's sequence-number save.
	assert.LessOrEqual(t, atomic.LoadInt64(&inserts), int64(1200/budgetBlockBatch+1), "batched, not one INSERT per App")

	restarted := &BudgetBlocks{blocks: map[uint]string{}}
	loadBudgetBlocks(db, restarted)
	_, blocked := restarted.Reason(1200)
	assert.True(t, blocked)

	// Unchanged: no write at all (not even the DELETE; a marker row would go).
	require.NoError(t, db.Create(&database.BudgetBlock{AppID: 999999, Reason: "marker"}).Error)
	sync(big)
	require.NoError(t, db.Model(&database.BudgetBlock{}).Where("app_id = ?", 999999).Count(&n).Error)
	assert.EqualValues(t, 1, n, "an unchanged set is not rewritten")

	// A failed write is retried even though the set did not change again.
	handler.blocksUnsaved = true
	sync(big)
	require.NoError(t, db.Model(&database.BudgetBlock{}).Where("app_id = ?", 999999).Count(&n).Error)
	assert.Zero(t, n, "rewritten after an unsaved set")
	assert.False(t, handler.blocksUnsaved)

	// A change is written.
	sync(map[uint32]string{7: "team monthly budget exceeded"})
	require.NoError(t, db.Model(&database.BudgetBlock{}).Count(&n).Error)
	assert.EqualValues(t, 1, n)
}
