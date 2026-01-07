package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBudgetSyncHandlerTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations
	err = database.Migrate(db)
	require.NoError(t, err)

	return db
}

func createBudgetSyncEvent(payload BudgetSyncPayload) eventbridge.Event {
	data, _ := json.Marshal(payload)
	return eventbridge.Event{
		ID:      "test-event-id",
		Topic:   BudgetSyncTopic,
		Origin:  "control",
		Dir:     eventbridge.DirDown,
		Payload: data,
	}
}

func TestBudgetSyncHandler_HandleBudgetSync(t *testing.T) {
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	payload := BudgetSyncPayload{
		AppUsages: map[uint32]float64{
			1: 25.50, // $25.50
			2: 10.00, // $10.00
		},
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   1,
	}

	event := createBudgetSyncEvent(payload)
	handler.HandleBudgetSync(event)

	// Verify budget_usage records were created
	var usage1 database.BudgetUsage
	err := db.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&usage1).Error
	require.NoError(t, err)
	assert.InDelta(t, 255000.0, usage1.TotalCost, 1.0) // $25.50 * 10000

	var usage2 database.BudgetUsage
	err = db.Where("app_id = ? AND period_start = ?", 2, periodStart).First(&usage2).Error
	require.NoError(t, err)
	assert.InDelta(t, 100000.0, usage2.TotalCost, 1.0) // $10.00 * 10000
}

func TestBudgetSyncHandler_SequenceNumberDeduplication(t *testing.T) {
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// First event with sequence 5
	payload1 := BudgetSyncPayload{
		AppUsages:        map[uint32]float64{1: 25.00},
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   5,
	}
	handler.HandleBudgetSync(createBudgetSyncEvent(payload1))

	// Stale event with sequence 3 (should be ignored)
	payload2 := BudgetSyncPayload{
		AppUsages:        map[uint32]float64{1: 50.00}, // Higher value but stale
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   3,
	}
	handler.HandleBudgetSync(createBudgetSyncEvent(payload2))

	// Verify the original value is preserved
	var usage database.BudgetUsage
	err := db.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&usage).Error
	require.NoError(t, err)
	assert.InDelta(t, 250000.0, usage.TotalCost, 1.0) // $25.00 * 10000 (not $50.00)

	// Verify sequence number tracking
	assert.Equal(t, uint64(5), handler.GetLastSequenceNumber())
}

func TestBudgetSyncHandler_MaxLocalValue(t *testing.T) {
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Pre-create a local budget_usage record with higher value
	localUsage := &database.BudgetUsage{
		AppID:       1,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalCost:   500000.0, // $50.00 (local value is higher)
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	err := db.Create(localUsage).Error
	require.NoError(t, err)

	// Sync event with lower value from control
	payload := BudgetSyncPayload{
		AppUsages:        map[uint32]float64{1: 30.00}, // $30.00 (lower than local)
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   1,
	}
	handler.HandleBudgetSync(createBudgetSyncEvent(payload))

	// Local value should be preserved (max of control and local)
	var usage database.BudgetUsage
	err = db.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&usage).Error
	require.NoError(t, err)
	assert.InDelta(t, 500000.0, usage.TotalCost, 1.0) // $50.00 preserved (not $30.00)
}

func TestBudgetSyncHandler_ControlValueHigher(t *testing.T) {
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Pre-create a local budget_usage record with lower value
	localUsage := &database.BudgetUsage{
		AppID:       1,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalCost:   200000.0, // $20.00 (local value is lower)
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	err := db.Create(localUsage).Error
	require.NoError(t, err)

	// Sync event with higher value from control
	payload := BudgetSyncPayload{
		AppUsages:        map[uint32]float64{1: 50.00}, // $50.00 (higher than local)
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   1,
	}
	handler.HandleBudgetSync(createBudgetSyncEvent(payload))

	// Control value should be used (max of control and local)
	var usage database.BudgetUsage
	err = db.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&usage).Error
	require.NoError(t, err)
	assert.InDelta(t, 500000.0, usage.TotalCost, 1.0) // $50.00 from control
}

func TestBudgetSyncHandler_ClockSkewWarning(t *testing.T) {
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)

	now := time.Now()
	localPeriodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Create a sync event with a period from 2 hours in the future (simulating clock skew)
	skewedPeriodStart := localPeriodStart.Add(2 * time.Hour)
	periodEnd := skewedPeriodStart.AddDate(0, 1, 0).Add(-time.Second)

	payload := BudgetSyncPayload{
		AppUsages:        map[uint32]float64{1: 25.00},
		PeriodStart:      skewedPeriodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   1,
	}

	// Should still process the event (control is authoritative)
	handler.HandleBudgetSync(createBudgetSyncEvent(payload))

	// Record should be created with control's period dates
	var usage database.BudgetUsage
	err := db.Where("app_id = ? AND period_start = ?", 1, skewedPeriodStart).First(&usage).Error
	require.NoError(t, err)
	assert.InDelta(t, 250000.0, usage.TotalCost, 1.0)
}

func TestBudgetSyncHandler_MultipleApps(t *testing.T) {
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Pre-create some local usage for apps
	localUsage1 := &database.BudgetUsage{
		AppID:       1,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalCost:   150000.0, // $15.00
	}
	localUsage2 := &database.BudgetUsage{
		AppID:       2,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalCost:   250000.0, // $25.00
	}
	db.Create(localUsage1)
	db.Create(localUsage2)

	// Sync with mixed values
	payload := BudgetSyncPayload{
		AppUsages: map[uint32]float64{
			1: 20.00, // Control: $20.00 > local $15.00 -> should update
			2: 10.00, // Control: $10.00 < local $25.00 -> should keep local
			3: 30.00, // New app, no local -> should create
		},
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   1,
	}

	handler.HandleBudgetSync(createBudgetSyncEvent(payload))

	// App 1: control value used (higher)
	var usage1 database.BudgetUsage
	err := db.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&usage1).Error
	require.NoError(t, err)
	assert.InDelta(t, 200000.0, usage1.TotalCost, 1.0) // $20.00

	// App 2: local value preserved (higher)
	var usage2 database.BudgetUsage
	err = db.Where("app_id = ? AND period_start = ?", 2, periodStart).First(&usage2).Error
	require.NoError(t, err)
	assert.InDelta(t, 250000.0, usage2.TotalCost, 1.0) // $25.00

	// App 3: new record created
	var usage3 database.BudgetUsage
	err = db.Where("app_id = ? AND period_start = ?", 3, periodStart).First(&usage3).Error
	require.NoError(t, err)
	assert.InDelta(t, 300000.0, usage3.TotalCost, 1.0) // $30.00
}

func TestBudgetSyncHandler_InvalidPayload(t *testing.T) {
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)

	// Create event with invalid JSON payload
	event := eventbridge.Event{
		ID:      "test-event-id",
		Topic:   BudgetSyncTopic,
		Origin:  "control",
		Dir:     eventbridge.DirDown,
		Payload: []byte("invalid json"),
	}

	// Should not panic, just log error
	handler.HandleBudgetSync(event)

	// Sequence number should not be updated
	assert.Equal(t, uint64(0), handler.GetLastSequenceNumber())
}

func TestBudgetSyncHandler_SequenceNumberPersistence(t *testing.T) {
	// This test verifies that sequence numbers survive edge restarts
	db := setupBudgetSyncHandlerTestDB(t)

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Create first handler and process an event
	handler1 := NewBudgetSyncHandler(db)
	payload := BudgetSyncPayload{
		AppUsages:        map[uint32]float64{1: 25.00},
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   10,
	}
	handler1.HandleBudgetSync(createBudgetSyncEvent(payload))
	assert.Equal(t, uint64(10), handler1.GetLastSequenceNumber())

	// Verify sync_state is persisted in database
	var state database.SyncState
	err := db.Where("topic = ?", BudgetSyncTopic).First(&state).Error
	require.NoError(t, err)
	assert.Equal(t, uint64(10), state.SequenceNumber)

	// Simulate edge restart by creating a new handler with same DB
	handler2 := NewBudgetSyncHandler(db)

	// New handler should have loaded the sequence number from DB
	assert.Equal(t, uint64(10), handler2.GetLastSequenceNumber())

	// Stale event should be rejected (sequence 5 < 10)
	stalePayload := BudgetSyncPayload{
		AppUsages:        map[uint32]float64{1: 100.00}, // Would change budget if processed
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   5, // Stale sequence number
	}
	handler2.HandleBudgetSync(createBudgetSyncEvent(stalePayload))

	// Budget should still be $25.00 (stale event was rejected)
	var usage database.BudgetUsage
	err = db.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&usage).Error
	require.NoError(t, err)
	assert.InDelta(t, 250000.0, usage.TotalCost, 1.0) // $25.00 * 10000

	// New event with higher sequence should be processed
	newPayload := BudgetSyncPayload{
		AppUsages:        map[uint32]float64{1: 50.00},
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   15, // Higher than 10
	}
	handler2.HandleBudgetSync(createBudgetSyncEvent(newPayload))

	// Budget should now be $50.00
	err = db.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&usage).Error
	require.NoError(t, err)
	assert.InDelta(t, 500000.0, usage.TotalCost, 1.0) // $50.00 * 10000

	// Sequence should be updated in database
	err = db.Where("topic = ?", BudgetSyncTopic).First(&state).Error
	require.NoError(t, err)
	assert.Equal(t, uint64(15), state.SequenceNumber)
}

func TestBudgetSyncHandler_ConcurrentEvents(t *testing.T) {
	// Test that concurrent events are handled correctly with the mutex
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Run concurrent sync events
	done := make(chan bool, 10)
	for i := 1; i <= 10; i++ {
		go func(seq int) {
			payload := BudgetSyncPayload{
				AppUsages:        map[uint32]float64{1: float64(seq * 10)},
				PeriodStart:      periodStart,
				PeriodEnd:        periodEnd,
				ControlTimestamp: now,
				SequenceNumber:   uint64(seq),
			}
			handler.HandleBudgetSync(createBudgetSyncEvent(payload))
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// The highest sequence number should be recorded
	assert.Equal(t, uint64(10), handler.GetLastSequenceNumber())

	// Budget should reflect the highest sequence event ($100.00)
	var usage database.BudgetUsage
	err := db.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&usage).Error
	require.NoError(t, err)
	assert.InDelta(t, 1000000.0, usage.TotalCost, 1.0) // $100.00 * 10000
}
