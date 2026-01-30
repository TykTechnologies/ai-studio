package grpc

import (
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBudgetSyncTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate both LLMChatRecord and App tables (App is needed for BudgetStartDate lookup)
	err = db.AutoMigrate(&models.LLMChatRecord{}, &models.App{})
	require.NoError(t, err)

	return db
}

func TestNewBudgetSyncService(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()

	// Test default interval
	service := NewBudgetSyncService(db, bus)
	assert.Equal(t, DefaultBudgetSyncInterval, service.GetSyncInterval())
}

func TestNewBudgetSyncService_CustomInterval(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()

	// Set custom interval via environment variable
	os.Setenv("BUDGET_SYNC_INTERVAL", "15s")
	defer os.Unsetenv("BUDGET_SYNC_INTERVAL")

	service := NewBudgetSyncService(db, bus)
	assert.Equal(t, 15*time.Second, service.GetSyncInterval())
}

func TestNewBudgetSyncService_InvalidInterval(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()

	// Set invalid interval - should fall back to default
	os.Setenv("BUDGET_SYNC_INTERVAL", "invalid")
	defer os.Unsetenv("BUDGET_SYNC_INTERVAL")

	service := NewBudgetSyncService(db, bus)
	assert.Equal(t, DefaultBudgetSyncInterval, service.GetSyncInterval())
}

func TestBudgetSyncService_PublishesEvents(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()

	// Insert test chat records
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Create apps first (required for budget sync to work)
	apps := []models.App{
		{Name: "App 1"},
		{Name: "App 2"},
	}
	for i := range apps {
		err := db.Create(&apps[i]).Error
		require.NoError(t, err)
	}

	records := []models.LLMChatRecord{
		{
			AppID:     apps[0].ID,
			LLMID:     1,
			Cost:      250000.0, // $25.00 (stored as dollars * 10000)
			TimeStamp: periodStart.Add(time.Hour),
		},
		{
			AppID:     apps[0].ID,
			LLMID:     1,
			Cost:      100000.0, // $10.00
			TimeStamp: periodStart.Add(2 * time.Hour),
		},
		{
			AppID:     apps[1].ID,
			LLMID:     1,
			Cost:      50000.0, // $5.00
			TimeStamp: periodStart.Add(3 * time.Hour),
		},
	}

	for _, record := range records {
		err := db.Create(&record).Error
		require.NoError(t, err)
	}

	// Subscribe to budget.sync events
	var receivedEvent eventbridge.Event
	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe(BudgetSyncTopic, func(event eventbridge.Event) {
		receivedEvent = event
		wg.Done()
	})

	// Create service and trigger aggregation
	service := NewBudgetSyncService(db, bus)
	service.aggregateAndPublish()

	// Wait for event with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for budget sync event")
	}

	// Verify event properties
	assert.Equal(t, BudgetSyncTopic, receivedEvent.Topic)
	assert.Equal(t, "control", receivedEvent.Origin)
	assert.Equal(t, eventbridge.DirDown, receivedEvent.Dir)

	// Parse and verify payload
	var payload BudgetSyncPayload
	err := json.Unmarshal(receivedEvent.Payload, &payload)
	require.NoError(t, err)

	// App 1 should have $35.00 total (25 + 10)
	assert.InDelta(t, 35.0, payload.AppUsages[uint32(apps[0].ID)], 0.01)

	// App 2 should have $5.00
	assert.InDelta(t, 5.0, payload.AppUsages[uint32(apps[1].ID)], 0.01)

	// Verify sequence number incremented
	assert.Equal(t, uint64(1), payload.SequenceNumber)

	// Verify period dates (compare year, month, day - timezone handling varies by environment)
	assert.Equal(t, periodStart.Year(), payload.PeriodStart.Year())
	assert.Equal(t, periodStart.Month(), payload.PeriodStart.Month())
	assert.Equal(t, periodStart.Day(), payload.PeriodStart.Day())
}

func TestBudgetSyncService_SequenceNumberIncrement(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()

	// Insert a test record
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Create app first (required for budget sync to work)
	app := models.App{Name: "Test App"}
	err := db.Create(&app).Error
	require.NoError(t, err)

	record := models.LLMChatRecord{
		AppID:     app.ID,
		LLMID:     1,
		Cost:      100000.0,
		TimeStamp: periodStart.Add(time.Hour),
	}
	err = db.Create(&record).Error
	require.NoError(t, err)

	// Collect events
	var events []BudgetSyncPayload
	var mu sync.Mutex

	bus.Subscribe(BudgetSyncTopic, func(event eventbridge.Event) {
		var payload BudgetSyncPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			mu.Lock()
			events = append(events, payload)
			mu.Unlock()
		}
	})

	// Create service and trigger multiple aggregations
	service := NewBudgetSyncService(db, bus)
	service.aggregateAndPublish()
	service.aggregateAndPublish()
	service.aggregateAndPublish()

	// Allow time for events to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify sequence numbers are incrementing
	mu.Lock()
	assert.Len(t, events, 3)
	assert.Equal(t, uint64(1), events[0].SequenceNumber)
	assert.Equal(t, uint64(2), events[1].SequenceNumber)
	assert.Equal(t, uint64(3), events[2].SequenceNumber)
	mu.Unlock()
}

func TestBudgetSyncService_NoDataNoPublish(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()

	// No records in database

	var eventReceived bool
	bus.Subscribe(BudgetSyncTopic, func(event eventbridge.Event) {
		eventReceived = true
	})

	// Create service and trigger aggregation
	service := NewBudgetSyncService(db, bus)
	service.aggregateAndPublish()

	// Allow time for potential event
	time.Sleep(100 * time.Millisecond)

	// No event should be published when there's no data
	assert.False(t, eventReceived, "No event should be published when there's no usage data")
}

func TestBudgetSyncService_StartStop(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()

	// Set a very short interval for testing
	os.Setenv("BUDGET_SYNC_INTERVAL", "50ms")
	defer os.Unsetenv("BUDGET_SYNC_INTERVAL")

	// Insert a test record
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Create app first (required for budget sync to work)
	app := models.App{Name: "Test App"}
	err := db.Create(&app).Error
	require.NoError(t, err)

	record := models.LLMChatRecord{
		AppID:     app.ID,
		LLMID:     1,
		Cost:      100000.0,
		TimeStamp: periodStart.Add(time.Hour),
	}
	err = db.Create(&record).Error
	require.NoError(t, err)

	// Count events
	var eventCount int
	var mu sync.Mutex

	bus.Subscribe(BudgetSyncTopic, func(event eventbridge.Event) {
		mu.Lock()
		eventCount++
		mu.Unlock()
	})

	// Start service
	service := NewBudgetSyncService(db, bus)
	service.Start()

	// Wait for a few cycles
	time.Sleep(200 * time.Millisecond)

	// Stop service
	service.Stop()

	// Verify we received some events
	mu.Lock()
	count := eventCount
	mu.Unlock()

	assert.GreaterOrEqual(t, count, 2, "Should have received at least 2 events during test")

	// Verify no more events after stop
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	finalCount := eventCount
	mu.Unlock()

	// Allow for at most 1 more event due to timing
	assert.LessOrEqual(t, finalCount-count, 1, "Should not receive many more events after stop")
}

func TestBudgetSyncService_IgnoresRecordsOutsidePeriod(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()

	// Calculate current period
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Create app first (required for budget sync to work)
	app := models.App{Name: "Test App"}
	err := db.Create(&app).Error
	require.NoError(t, err)

	// Insert records: one in current period, one in previous period
	currentPeriodRecord := models.LLMChatRecord{
		AppID:     app.ID,
		LLMID:     1,
		Cost:      100000.0, // $10.00
		TimeStamp: periodStart.Add(time.Hour),
	}
	previousPeriodRecord := models.LLMChatRecord{
		AppID:     app.ID,
		LLMID:     1,
		Cost:      200000.0, // $20.00 - should be ignored
		TimeStamp: periodStart.Add(-24 * time.Hour), // Previous period
	}

	err = db.Create(&currentPeriodRecord).Error
	require.NoError(t, err)
	err = db.Create(&previousPeriodRecord).Error
	require.NoError(t, err)

	// Subscribe to events
	var receivedPayload BudgetSyncPayload
	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe(BudgetSyncTopic, func(event eventbridge.Event) {
		json.Unmarshal(event.Payload, &receivedPayload)
		wg.Done()
	})

	// Create service and trigger aggregation
	service := NewBudgetSyncService(db, bus)
	service.aggregateAndPublish()

	// Wait for event
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for event")
	}

	// Only current period record should be counted
	assert.InDelta(t, 10.0, receivedPayload.AppUsages[uint32(app.ID)], 0.01, "Should only include current period records")
}
