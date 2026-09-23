package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBudgetSyncHandler_TeamBlocks(t *testing.T) {
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)
	handler.blocks = &TeamBlocks{blocks: map[uint]string{}}

	handler.HandleBudgetSync(createBudgetSyncEvent(BudgetSyncPayload{
		ControlTimestamp:   time.Now(),
		SequenceNumber:     1,
		TeamBlocks:         map[uint32]string{7: "team monthly budget exceeded"},
		TeamBlocksIncluded: true,
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
		ControlTimestamp:   time.Now(),
		SequenceNumber:     3,
		TeamBlocksIncluded: true,
	}))
	_, blocked = handler.blocks.Reason(7)
	assert.False(t, blocked)
}
