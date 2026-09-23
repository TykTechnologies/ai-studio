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

	// Blocks maps app_id to the reason edges must refuse the App on budget
	// grounds (Enterprise): its budget is 0, or its hard-blocking team has
	// spent its budget. Edges read a synced App budget of 0 as "no limit",
	// so this is how a zero budget reaches them. When BlocksIncluded is set
	// it is the complete set: edges replace theirs, so a released App is
	// served again.
	Blocks         map[uint32]string `json:"blocks,omitempty"`
	BlocksIncluded bool              `json:"blocks_included,omitempty"`
}

// EdgeBudgetSource is the Enterprise budget service as the budget sync sees
// it: the Apps edges must refuse, and threshold analysis for spend that
// reached Studio from edges (edge traffic never passes Studio's own budget
// check, so nothing else would raise its alerts).
type EdgeBudgetSource interface {
	EdgeBlocks() (map[uint]string, error)
	AnalyzeApps(appIDs []uint)
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
	budgetSource   atomic.Value // EdgeBudgetSource
	// lastUsage is each App's usage at the previous sync; Apps whose usage
	// moved are analysed for alerts. Only the sync goroutine touches it.
	lastUsage map[uint32]float64
}

// SetEdgeBudgetSource makes every sync carry budget blocks and analyse
// Apps whose spend moved since the previous sync.
func (s *BudgetSyncService) SetEdgeBudgetSource(src EdgeBudgetSource) {
	if src != nil {
		s.budgetSource.Store(src)
	}
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

	// Blocks are sent every time (even empty) so edges release Apps that
	// are allowed again.
	var blocks map[uint32]string
	blocksIncluded := false
	src, _ := s.budgetSource.Load().(EdgeBudgetSource)
	if src != nil {
		found, err := src.EdgeBlocks()
		if err != nil {
			log.Error().Err(err).Msg("Failed to compute budget blocks; keeping edges' last set")
		} else {
			blocksIncluded = true
			blocks = make(map[uint32]string, len(found))
			for id, reason := range found {
				blocks[uint32(id)] = reason
			}
		}

		// Alerts for spend that came in from edges.
		var moved []uint
		for id, data := range appBudgets {
			if prev, ok := s.lastUsage[id]; !ok || prev != data.Usage {
				moved = append(moved, uint(id))
			}
		}
		next := make(map[uint32]float64, len(appBudgets))
		for id, data := range appBudgets {
			next[id] = data.Usage
		}
		s.lastUsage = next
		if len(moved) > 0 {
			src.AnalyzeApps(moved)
		}
	}

	// Skip publishing if there is nothing to sync
	if len(appBudgets) == 0 && !blocksIncluded {
		log.Debug().Msg("No budget usage data to sync")
		return
	}

	payload := BudgetSyncPayload{
		Blocks:           blocks,
		BlocksIncluded:   blocksIncluded,
		AppUsages:        appUsages,           // Legacy field
		AppBudgets:       appBudgets,          // New per-app periods
		PeriodStart:      calendarPeriodStart, // Legacy field
		PeriodEnd:        calendarPeriodEnd,   // Legacy field
		ControlTimestamp: now,
		SequenceNumber:   s.nextSequence(),
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

// nextSequence returns a sequence number above both the previous one and
// the current time in nanoseconds. Edges persist the highest sequence they
// have seen and drop anything lower, so a counter that restarted from zero
// with Studio made edges ignore every sync after a Studio restart (and a
// second Studio node's syncs). Seeding from the clock keeps it increasing
// across restarts and nodes.
func (s *BudgetSyncService) nextSequence() uint64 {
	for {
		prev := atomic.LoadUint64(&s.sequenceNumber)
		next := uint64(time.Now().UnixNano())
		if next <= prev {
			next = prev + 1
		}
		if atomic.CompareAndSwapUint64(&s.sequenceNumber, prev, next) {
			return next
		}
	}
}

// GetSyncInterval returns the configured sync interval (for testing)
func (s *BudgetSyncService) GetSyncInterval() time.Duration {
	return s.syncInterval
}
