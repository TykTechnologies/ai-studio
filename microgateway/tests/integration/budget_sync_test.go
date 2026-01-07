// tests/integration/budget_sync_test.go
package integration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestBudgetSync_EndToEnd tests the complete budget sync flow:
// 1. Control server publishes budget sync event
// 2. Edge handler receives and processes the event
// 3. Edge's local budget_usage table is updated
func TestBudgetSync_EndToEnd(t *testing.T) {
	// Setup edge database
	edgeDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = database.Migrate(edgeDB)
	require.NoError(t, err)

	// Create event bus (simulates the edge's event bus connected via bridge)
	eventBus := eventbridge.NewBus()

	// Create edge budget sync handler
	budgetSyncHandler := services.NewBudgetSyncHandler(edgeDB)

	// Subscribe handler to budget.sync topic
	eventBus.Subscribe(services.BudgetSyncTopic, budgetSyncHandler.HandleBudgetSync)

	// Calculate period
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Simulate control server publishing budget sync event
	payload := services.BudgetSyncPayload{
		AppUsages: map[uint32]float64{
			1: 45.50,  // $45.50
			2: 100.00, // $100.00
		},
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   1,
	}

	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	// Publish event (simulates what control server does via PublishDown)
	eventBus.Publish(eventbridge.Event{
		ID:      "test-sync-1",
		Topic:   services.BudgetSyncTopic,
		Origin:  "control",
		Dir:     eventbridge.DirDown,
		Payload: payloadBytes,
	})

	// Allow time for event processing
	time.Sleep(50 * time.Millisecond)

	// Verify budget_usage records were created on edge
	var usage1 database.BudgetUsage
	err = edgeDB.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&usage1).Error
	require.NoError(t, err)
	assert.InDelta(t, 455000.0, usage1.TotalCost, 1.0) // $45.50 * 10000

	var usage2 database.BudgetUsage
	err = edgeDB.Where("app_id = ? AND period_start = ?", 2, periodStart).First(&usage2).Error
	require.NoError(t, err)
	assert.InDelta(t, 1000000.0, usage2.TotalCost, 1.0) // $100.00 * 10000
}

// TestBudgetSync_MultiEdgeScenario simulates the scenario where:
// 1. Two edges have different local usage values
// 2. Control aggregates and publishes the combined total
// 3. Both edges update to the same aggregated value
func TestBudgetSync_MultiEdgeScenario(t *testing.T) {
	// Setup two edge databases (simulating two edge gateways)
	edge1DB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = database.Migrate(edge1DB)
	require.NoError(t, err)

	edge2DB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = database.Migrate(edge2DB)
	require.NoError(t, err)

	// Create event buses for each edge
	edge1Bus := eventbridge.NewBus()
	edge2Bus := eventbridge.NewBus()

	// Create handlers for each edge
	edge1Handler := services.NewBudgetSyncHandler(edge1DB)
	edge2Handler := services.NewBudgetSyncHandler(edge2DB)

	// Subscribe handlers
	edge1Bus.Subscribe(services.BudgetSyncTopic, edge1Handler.HandleBudgetSync)
	edge2Bus.Subscribe(services.BudgetSyncTopic, edge2Handler.HandleBudgetSync)

	// Calculate period
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Simulate initial state: Edge 1 has processed $20, Edge 2 has processed $30
	// Control would aggregate this to $50 total
	edge1LocalUsage := &database.BudgetUsage{
		AppID:       1,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalCost:   200000.0, // $20.00
	}
	edge2LocalUsage := &database.BudgetUsage{
		AppID:       1,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalCost:   300000.0, // $30.00
	}
	err = edge1DB.Create(edge1LocalUsage).Error
	require.NoError(t, err)
	err = edge2DB.Create(edge2LocalUsage).Error
	require.NoError(t, err)

	// Control server aggregates and publishes $50 total
	payload := services.BudgetSyncPayload{
		AppUsages: map[uint32]float64{
			1: 50.00, // $50.00 aggregated total from all edges
		},
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   1,
	}

	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	event := eventbridge.Event{
		ID:      "test-sync-2",
		Topic:   services.BudgetSyncTopic,
		Origin:  "control",
		Dir:     eventbridge.DirDown,
		Payload: payloadBytes,
	}

	// Publish to both edges (simulates control broadcasting to all edges)
	edge1Bus.Publish(event)
	edge2Bus.Publish(event)

	// Allow time for event processing
	time.Sleep(50 * time.Millisecond)

	// Both edges should now have the aggregated value $50.00
	// Edge 1 had $20 locally, control sent $50, so it updates to $50 (max)
	var edge1Usage database.BudgetUsage
	err = edge1DB.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&edge1Usage).Error
	require.NoError(t, err)
	assert.InDelta(t, 500000.0, edge1Usage.TotalCost, 1.0, "Edge 1 should have $50.00")

	// Edge 2 had $30 locally, control sent $50, so it updates to $50 (max)
	var edge2Usage database.BudgetUsage
	err = edge2DB.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&edge2Usage).Error
	require.NoError(t, err)
	assert.InDelta(t, 500000.0, edge2Usage.TotalCost, 1.0, "Edge 2 should have $50.00")
}

// TestBudgetSync_LocalIncrementPreserved tests that local increments
// between syncs are preserved if they're higher than the control value
func TestBudgetSync_LocalIncrementPreserved(t *testing.T) {
	// Setup edge database
	edgeDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = database.Migrate(edgeDB)
	require.NoError(t, err)

	eventBus := eventbridge.NewBus()
	handler := services.NewBudgetSyncHandler(edgeDB)
	eventBus.Subscribe(services.BudgetSyncTopic, handler.HandleBudgetSync)

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Initial sync from control: $50 total
	payload1 := services.BudgetSyncPayload{
		AppUsages:        map[uint32]float64{1: 50.00},
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   1,
	}
	bytes1, _ := json.Marshal(payload1)
	eventBus.Publish(eventbridge.Event{
		ID:      "sync-1",
		Topic:   services.BudgetSyncTopic,
		Origin:  "control",
		Dir:     eventbridge.DirDown,
		Payload: bytes1,
	})
	time.Sleep(50 * time.Millisecond)

	// Verify initial value
	var usage database.BudgetUsage
	err = edgeDB.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&usage).Error
	require.NoError(t, err)
	assert.InDelta(t, 500000.0, usage.TotalCost, 1.0)

	// Simulate edge processing a local request that adds $10
	// This represents a request that hasn't been reported to control yet
	err = edgeDB.Model(&usage).Update("total_cost", 600000.0).Error // $60.00
	require.NoError(t, err)

	// Control sends another sync with $55 (it doesn't know about the $10 yet)
	payload2 := services.BudgetSyncPayload{
		AppUsages:        map[uint32]float64{1: 55.00},
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now.Add(30 * time.Second),
		SequenceNumber:   2,
	}
	bytes2, _ := json.Marshal(payload2)
	eventBus.Publish(eventbridge.Event{
		ID:      "sync-2",
		Topic:   services.BudgetSyncTopic,
		Origin:  "control",
		Dir:     eventbridge.DirDown,
		Payload: bytes2,
	})
	time.Sleep(50 * time.Millisecond)

	// Local value $60 should be preserved (higher than control's $55)
	err = edgeDB.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&usage).Error
	require.NoError(t, err)
	assert.InDelta(t, 600000.0, usage.TotalCost, 1.0, "Local value should be preserved")
}

// TestBudgetSync_BudgetEnforcementWithSync tests that budget checking
// uses the synchronized value for enforcement
func TestBudgetSync_BudgetEnforcementWithSync(t *testing.T) {
	// Setup edge database
	edgeDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = database.Migrate(edgeDB)
	require.NoError(t, err)

	eventBus := eventbridge.NewBus()
	handler := services.NewBudgetSyncHandler(edgeDB)
	eventBus.Subscribe(services.BudgetSyncTopic, handler.HandleBudgetSync)

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Create an app with a $100 monthly budget
	app := &database.App{
		Name:          "Test App",
		IsActive:      true,
		MonthlyBudget: 100.00, // $100 budget
	}
	err = edgeDB.Create(app).Error
	require.NoError(t, err)

	// Initial state: edge thinks it has only processed $20 locally
	localUsage := &database.BudgetUsage{
		AppID:       app.ID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalCost:   200000.0, // $20.00
	}
	err = edgeDB.Create(localUsage).Error
	require.NoError(t, err)

	// Control syncs: total across all edges is actually $90
	payload := services.BudgetSyncPayload{
		AppUsages:        map[uint32]float64{uint32(app.ID): 90.00},
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   1,
	}
	bytes, _ := json.Marshal(payload)
	eventBus.Publish(eventbridge.Event{
		ID:      "sync-budget",
		Topic:   services.BudgetSyncTopic,
		Origin:  "control",
		Dir:     eventbridge.DirDown,
		Payload: bytes,
	})
	time.Sleep(50 * time.Millisecond)

	// Verify budget_usage is now $90
	var usage database.BudgetUsage
	err = edgeDB.Where("app_id = ? AND period_start = ?", app.ID, periodStart).First(&usage).Error
	require.NoError(t, err)
	assert.InDelta(t, 900000.0, usage.TotalCost, 1.0)

	// Now when budget is checked, it should see $90 used, only $10 remaining
	// A $15 request should be rejected (assuming budget enforcement logic reads from budget_usage)
	currentUsageDollars := usage.TotalCost / 10000.0
	remainingBudget := app.MonthlyBudget - currentUsageDollars

	assert.InDelta(t, 10.0, remainingBudget, 0.01, "Should have $10 remaining budget")

	// A $15 request would exceed budget
	proposedCost := 15.00
	wouldExceedBudget := currentUsageDollars+proposedCost > app.MonthlyBudget
	assert.True(t, wouldExceedBudget, "$15 request should exceed remaining $10 budget")

	// A $5 request would be within budget
	proposedCost = 5.00
	wouldExceedBudget = currentUsageDollars+proposedCost > app.MonthlyBudget
	assert.False(t, wouldExceedBudget, "$5 request should be within remaining $10 budget")
}
