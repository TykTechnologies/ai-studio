package grpc

import (
	"context"
	"os"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
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

// BudgetSyncPayload is the JSON payload for budget.sync events
// sent from control server to edge gateways to synchronize budget usage.
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
	// Used by edges to detect clock skew
	ControlTimestamp time.Time `json:"control_timestamp"`

	// SequenceNumber for ordering and deduplication
	SequenceNumber uint64 `json:"sequence_number"`
}

// BudgetSyncService aggregates budget usage from llm_chat_records
// and publishes updates to edge gateways via the event bridge.
type BudgetSyncService struct {
	db             *gorm.DB
	eventBus       eventbridge.Bus
	syncInterval   time.Duration
	sequenceNumber uint64
	ctx            context.Context
	cancel         context.CancelFunc
	done           chan struct{}
}

// DefaultBudgetSyncInterval is the default interval for budget sync (30 seconds)
const DefaultBudgetSyncInterval = 30 * time.Second

// BudgetSyncTopic is re-exported from eventbridge for backwards compatibility
const BudgetSyncTopic = eventbridge.BudgetSyncTopic

// NewBudgetSyncService creates a new budget sync service.
// The interval can be configured via BUDGET_SYNC_INTERVAL environment variable.
func NewBudgetSyncService(db *gorm.DB, eventBus eventbridge.Bus) *BudgetSyncService {
	interval := DefaultBudgetSyncInterval

	if intervalStr := os.Getenv("BUDGET_SYNC_INTERVAL"); intervalStr != "" {
		if parsed, err := time.ParseDuration(intervalStr); err == nil {
			interval = parsed
			log.Info().Dur("interval", interval).Msg("Budget sync interval configured from environment")
		} else {
			log.Warn().Str("value", intervalStr).Err(err).Msg("Invalid BUDGET_SYNC_INTERVAL, using default")
		}
	}

	return &BudgetSyncService{
		db:           db,
		eventBus:     eventBus,
		syncInterval: interval,
		done:         make(chan struct{}),
	}
}

// Start begins the budget sync background task.
// It aggregates usage and publishes to edges at the configured interval.
func (s *BudgetSyncService) Start() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	go func() {
		log.Info().Dur("interval", s.syncInterval).Msg("Starting budget sync service")

		// Perform initial sync immediately
		s.aggregateAndPublish()

		ticker := time.NewTicker(s.syncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-s.ctx.Done():
				log.Info().Msg("Budget sync service stopped")
				close(s.done)
				return
			case <-ticker.C:
				s.aggregateAndPublish()
			}
		}
	}()
}

// Stop gracefully stops the budget sync service.
func (s *BudgetSyncService) Stop() {
	if s.cancel != nil {
		s.cancel()
		// Wait for goroutine to finish
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
			log.Warn().Msg("Budget sync service stop timed out")
		}
	}
}

// calculateBudgetPeriod determines the budget period for an app based on its budget_start_date.
// If no budget_start_date is set, uses calendar month (1st to last day).
// When a budget is reset on the same day, this preserves the exact reset time to ensure
// usage from before the reset is not counted.
// This is a package-level function so it can be shared by BudgetSyncService and ControlServer.
// Note: Timestamps are truncated to second precision to ensure consistency across all components.
func calculateBudgetPeriod(budgetStartDate *time.Time, now time.Time) (time.Time, time.Time) {
	if budgetStartDate == nil {
		// Default to calendar month
		periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)
		return periodStart, periodEnd
	}

	budgetDay := budgetStartDate.Day()
	currentYear := now.Year()
	currentMonth := now.Month()

	// If we haven't reached the budget day in current month,
	// the period started on the budget day of previous month
	if now.Day() < budgetDay {
		if currentMonth == time.January {
			currentMonth = time.December
			currentYear--
		} else {
			currentMonth--
		}
	}

	// Calculate the normalized period start (midnight of the budget day)
	normalizedPeriodStart := time.Date(currentYear, currentMonth, budgetDay, 0, 0, 0, 0, now.Location())
	periodEnd := normalizedPeriodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Check if the actual budget_start_date falls within this period.
	// If it does (e.g., budget was reset mid-period), use the exact timestamp
	// to ensure usage from before the reset is not counted.
	// Truncate to second precision to ensure consistency across control server and edges.
	if budgetStartDate.After(normalizedPeriodStart) && budgetStartDate.Before(periodEnd) {
		truncated := budgetStartDate.Truncate(time.Second)
		return truncated, periodEnd
	}

	return normalizedPeriodStart, periodEnd
}

// aggregateAndPublish queries the database for budget usage
// and publishes the aggregated values to all edges.
func (s *BudgetSyncService) aggregateAndPublish() {
	now := time.Now()

	// Default calendar period for legacy compatibility
	calendarPeriodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	calendarPeriodEnd := calendarPeriodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Get all apps with their budget_start_date
	var apps []models.App
	if err := s.db.Select("id", "budget_start_date").Find(&apps).Error; err != nil {
		log.Error().Err(err).Msg("Failed to fetch apps for budget sync")
		return
	}

	// Build per-app budget data with custom periods
	appBudgets := make(map[uint32]AppBudgetData)
	appUsages := make(map[uint32]float64) // Legacy field for backwards compatibility

	for _, app := range apps {
		periodStart, periodEnd := calculateBudgetPeriod(app.BudgetStartDate, now)

		// Query usage for this app's specific period
		var totalCost float64
		err := s.db.Raw(`
			SELECT COALESCE(SUM(cost), 0) as total_cost
			FROM llm_chat_records
			WHERE app_id = ? AND time_stamp >= ? AND time_stamp <= ?
		`, app.ID, periodStart, periodEnd).Scan(&totalCost).Error

		if err != nil {
			log.Error().Err(err).Uint("app_id", app.ID).Msg("Failed to query app budget usage")
			continue
		}

		// Convert from stored format (dollars * 10000) to dollars
		usageDollars := totalCost / 10000.0

		// Only include apps with usage > 0
		if usageDollars > 0 {
			appBudgets[uint32(app.ID)] = AppBudgetData{
				Usage:       usageDollars,
				PeriodStart: periodStart,
				PeriodEnd:   periodEnd,
			}
			// Also populate legacy field for backwards compatibility
			appUsages[uint32(app.ID)] = usageDollars
		}
	}

	// Skip publishing if no usage data
	if len(appBudgets) == 0 {
		log.Debug().Msg("No budget usage data to sync")
		return
	}

	payload := BudgetSyncPayload{
		AppUsages:        appUsages,           // Legacy field
		AppBudgets:       appBudgets,          // New per-app periods
		PeriodStart:      calendarPeriodStart, // Legacy field
		PeriodEnd:        calendarPeriodEnd,   // Legacy field
		ControlTimestamp: now,
		SequenceNumber:   atomic.AddUint64(&s.sequenceNumber, 1),
	}

	// Publish via event bridge (DirDown = control to edges)
	if err := eventbridge.PublishDown(s.eventBus, "control", BudgetSyncTopic, payload); err != nil {
		log.Error().Err(err).Msg("Failed to publish budget sync event")
		return
	}

	log.Debug().
		Int("app_count", len(appBudgets)).
		Uint64("sequence", payload.SequenceNumber).
		Msg("Published budget sync to edges")
}

// GetSyncInterval returns the configured sync interval (for testing)
func (s *BudgetSyncService) GetSyncInterval() time.Duration {
	return s.syncInterval
}
