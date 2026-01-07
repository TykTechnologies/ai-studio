package services

import (
	"encoding/json"
	"math"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// AppBudgetData contains budget usage and period info for a single app
type AppBudgetData struct {
	// Usage is the current period usage in dollars
	Usage float64 `json:"usage"`

	// PeriodStart is the start of this app's budget period
	PeriodStart time.Time `json:"period_start"`

	// PeriodEnd is the end of this app's budget period
	PeriodEnd time.Time `json:"period_end"`
}

// BudgetSyncPayload matches the control server's payload structure
type BudgetSyncPayload struct {
	// AppUsages maps app_id to current period usage in dollars
	// Deprecated: Use AppBudgets instead for per-app budget periods
	AppUsages map[uint32]float64 `json:"app_usages"`

	// AppBudgets maps app_id to budget data with per-app period info
	// This supports custom budget_start_date per app
	AppBudgets map[uint32]AppBudgetData `json:"app_budgets,omitempty"`

	// PeriodStart is the start of the budget period (1st of month)
	// Deprecated: Use per-app periods in AppBudgets instead
	PeriodStart time.Time `json:"period_start"`

	// PeriodEnd is the end of the budget period (last moment of month)
	// Deprecated: Use per-app periods in AppBudgets instead
	PeriodEnd time.Time `json:"period_end"`

	// ControlTimestamp is when the control server generated this payload
	ControlTimestamp time.Time `json:"control_timestamp"`

	// SequenceNumber for ordering and deduplication
	SequenceNumber uint64 `json:"sequence_number"`
}

// BudgetSyncTopic is re-exported from eventbridge for convenience
const BudgetSyncTopic = eventbridge.BudgetSyncTopic

// BudgetSyncHandler handles budget sync events from the control server.
// It updates local budget_usage table with aggregated values from control,
// using max(control_value, local_value) to ensure budgets never decrease.
type BudgetSyncHandler struct {
	db                 *gorm.DB
	lastSequenceNumber uint64
	mu                 sync.Mutex
}

// NewBudgetSyncHandler creates a new budget sync handler.
// It loads the last sequence number from the database to survive restarts.
func NewBudgetSyncHandler(db *gorm.DB) *BudgetSyncHandler {
	handler := &BudgetSyncHandler{
		db: db,
	}
	// Load persisted sequence number from database
	handler.loadSequenceNumber()
	return handler
}

// loadSequenceNumber loads the last processed sequence number from the database.
// This ensures deduplication works correctly across edge restarts.
func (h *BudgetSyncHandler) loadSequenceNumber() {
	var state database.SyncState
	err := h.db.Where("topic = ?", BudgetSyncTopic).First(&state).Error
	if err == nil {
		h.lastSequenceNumber = state.SequenceNumber
		log.Debug().
			Uint64("sequence", state.SequenceNumber).
			Msg("Loaded budget sync sequence number from database")
	} else if err != gorm.ErrRecordNotFound {
		log.Error().Err(err).Msg("Failed to load budget sync sequence number")
	}
	// If record not found, start from 0 (new installation)
}

// persistSequenceNumber saves the sequence number to the database.
func (h *BudgetSyncHandler) persistSequenceNumber(seqNum uint64) {
	now := time.Now()
	state := database.SyncState{
		Topic:          BudgetSyncTopic,
		SequenceNumber: seqNum,
		LastSyncAt:     now,
		UpdatedAt:      now,
	}

	// Upsert - update if exists, create if not
	err := h.db.Where("topic = ?", BudgetSyncTopic).Assign(map[string]interface{}{
		"sequence_number": seqNum,
		"last_sync_at":    now,
		"updated_at":      now,
	}).FirstOrCreate(&state).Error

	if err != nil {
		log.Error().Err(err).Uint64("sequence", seqNum).Msg("Failed to persist budget sync sequence number")
	}
}

// HandleBudgetSync processes a budget sync event from the control server.
// This method should be registered as a callback with the event bus.
func (h *BudgetSyncHandler) HandleBudgetSync(event eventbridge.Event) {
	// Parse payload
	var payload BudgetSyncPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal budget sync payload")
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Check sequence number to prevent processing stale updates
	if payload.SequenceNumber <= h.lastSequenceNumber {
		log.Debug().
			Uint64("received", payload.SequenceNumber).
			Uint64("last", h.lastSequenceNumber).
			Msg("Ignoring stale budget sync event")
		return
	}
	h.lastSequenceNumber = payload.SequenceNumber

	// Use new AppBudgets if available (supports per-app budget periods)
	if len(payload.AppBudgets) > 0 {
		for appID, budgetData := range payload.AppBudgets {
			h.updateLocalBudget(uint(appID), budgetData.Usage, budgetData.PeriodStart, budgetData.PeriodEnd)
		}

		log.Debug().
			Int("app_count", len(payload.AppBudgets)).
			Uint64("sequence", payload.SequenceNumber).
			Msg("Processed budget sync from control server (per-app periods)")
	} else {
		// Fall back to legacy AppUsages with global period (backwards compatibility)
		now := time.Now()
		localPeriodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

		// Allow up to 1 hour clock skew tolerance
		periodDiff := payload.PeriodStart.Sub(localPeriodStart).Hours()
		if math.Abs(periodDiff) > 1 {
			log.Warn().
				Time("control_period", payload.PeriodStart).
				Time("local_period", localPeriodStart).
				Float64("diff_hours", periodDiff).
				Msg("Budget sync period mismatch - possible clock skew")
		}

		for appID, usageDollars := range payload.AppUsages {
			h.updateLocalBudget(uint(appID), usageDollars, payload.PeriodStart, payload.PeriodEnd)
		}

		log.Debug().
			Int("app_count", len(payload.AppUsages)).
			Uint64("sequence", payload.SequenceNumber).
			Msg("Processed budget sync from control server (legacy)")
	}

	// Persist sequence number to survive restarts
	h.persistSequenceNumber(payload.SequenceNumber)
}

// updateLocalBudget updates the local budget_usage table for a single app.
// Uses max(control_value, local_value) to ensure budget never decreases.
func (h *BudgetSyncHandler) updateLocalBudget(appID uint, usageDollars float64, periodStart, periodEnd time.Time) {
	// Convert from dollars to stored format (dollars * 10000)
	storedCostFromControl := usageDollars * 10000

	// Get current local value
	var localUsage database.BudgetUsage
	err := h.db.Where("app_id = ? AND period_start = ?", appID, periodStart).First(&localUsage).Error

	// Determine final cost using max(control, local)
	finalCost := storedCostFromControl
	if err == nil && localUsage.TotalCost > storedCostFromControl {
		// Local value is higher than control - keep local value
		// This can happen if edge has processed requests since control's last aggregation
		finalCost = localUsage.TotalCost
		log.Debug().
			Uint("app_id", appID).
			Float64("control_cost", storedCostFromControl).
			Float64("local_cost", localUsage.TotalCost).
			Msg("Keeping higher local budget value")
	}

	// Upsert the budget_usage record
	now := time.Now()
	if err == gorm.ErrRecordNotFound {
		// Create new record
		newUsage := &database.BudgetUsage{
			AppID:       appID,
			PeriodStart: periodStart,
			PeriodEnd:   periodEnd,
			TotalCost:   finalCost,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if createErr := h.db.Create(newUsage).Error; createErr != nil {
			log.Error().Err(createErr).Uint("app_id", appID).Msg("Failed to create budget usage from sync")
		}
	} else if err == nil {
		// Update existing record
		if updateErr := h.db.Model(&localUsage).Updates(map[string]interface{}{
			"total_cost": finalCost,
			"updated_at": now,
		}).Error; updateErr != nil {
			log.Error().Err(updateErr).Uint("app_id", appID).Msg("Failed to update budget usage from sync")
		}
	} else {
		log.Error().Err(err).Uint("app_id", appID).Msg("Failed to query local budget usage")
	}
}

// GetLastSequenceNumber returns the last processed sequence number (for testing)
func (h *BudgetSyncHandler) GetLastSequenceNumber() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastSequenceNumber
}
