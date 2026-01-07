package grpc

import (
	"context"
	"os"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// BudgetSyncPayload is the JSON payload for budget.sync events
// sent from control server to edge gateways to synchronize budget usage.
type BudgetSyncPayload struct {
	// AppUsages maps app_id to current period usage in dollars
	AppUsages map[uint32]float64 `json:"app_usages"`

	// PeriodStart is the start of the budget period (1st of month)
	PeriodStart time.Time `json:"period_start"`

	// PeriodEnd is the end of the budget period (last moment of month)
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

// aggregateAndPublish queries the database for budget usage
// and publishes the aggregated values to all edges.
func (s *BudgetSyncService) aggregateAndPublish() {
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Query total cost from llm_chat_records grouped by app_id for current period
	var results []struct {
		AppID     uint32  `gorm:"column:app_id"`
		TotalCost float64 `gorm:"column:total_cost"`
	}

	// Cost is stored as dollars * 10000 in the database
	err := s.db.Raw(`
		SELECT
			app_id,
			COALESCE(SUM(cost), 0) as total_cost
		FROM llm_chat_records
		WHERE time_stamp >= ? AND time_stamp < ?
		AND app_id IS NOT NULL AND app_id > 0
		GROUP BY app_id
	`, periodStart, periodEnd.Add(time.Second)).Scan(&results).Error

	if err != nil {
		log.Error().Err(err).Msg("Failed to aggregate budget usage for sync")
		return
	}

	// Build payload with usage in dollars (convert from stored format)
	appUsages := make(map[uint32]float64)
	for _, r := range results {
		// Convert from cents*10000 to dollars
		appUsages[r.AppID] = r.TotalCost / 10000.0
	}

	// Skip publishing if no usage data
	if len(appUsages) == 0 {
		log.Debug().Msg("No budget usage data to sync")
		return
	}

	payload := BudgetSyncPayload{
		AppUsages:        appUsages,
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		ControlTimestamp: now,
		SequenceNumber:   atomic.AddUint64(&s.sequenceNumber, 1),
	}

	// Publish via event bridge (DirDown = control to edges)
	if err := eventbridge.PublishDown(s.eventBus, "control", BudgetSyncTopic, payload); err != nil {
		log.Error().Err(err).Msg("Failed to publish budget sync event")
		return
	}

	log.Debug().
		Int("app_count", len(appUsages)).
		Uint64("sequence", payload.SequenceNumber).
		Time("period_start", periodStart).
		Msg("Published budget sync to edges")
}

// GetSyncInterval returns the configured sync interval (for testing)
func (s *BudgetSyncService) GetSyncInterval() time.Duration {
	return s.syncInterval
}
