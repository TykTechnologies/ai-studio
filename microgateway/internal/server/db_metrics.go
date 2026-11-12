package server

import (
	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/rs/zerolog/log"
)

// registerDatabaseMetrics exports the connection pools' statistics and, for a
// SQLite file, the sizes of the database and its write-ahead log. A queue for
// connections (go_sql_wait_*) and a WAL that keeps growing are the two signs
// of write contention on the gateway's database.
func registerDatabaseMetrics(cfg *config.Config, sc *services.ServiceContainer) {
	if sqlDB, err := sc.DB.DB(); err == nil {
		if err := metrics.RegisterDBStats("main", sqlDB); err != nil {
			log.Warn().Err(err).Msg("Failed to register database pool metrics")
		}
	}
	if sc.WriteDB != nil && sc.WriteDB != sc.DB {
		if sqlDB, err := sc.WriteDB.DB(); err == nil {
			if err := metrics.RegisterDBStats("writer", sqlDB); err != nil {
				log.Warn().Err(err).Msg("Failed to register database writer pool metrics")
			}
		}
	}

	if w := sc.AnalyticsWriter; w != nil {
		for _, m := range []struct {
			name, help string
			counter    bool
			value      func(services.AnalyticsWriterStats) float64
		}{
			{"microgateway_analytics_writer_queue_depth", "Analytics rows waiting for the writer", false,
				func(s services.AnalyticsWriterStats) float64 { return float64(s.Queued) }},
			{"microgateway_analytics_writer_events_written_total", "Analytics rows written", true,
				func(s services.AnalyticsWriterStats) float64 { return float64(s.Written) }},
			{"microgateway_analytics_writer_events_dropped_total", "Analytics rows dropped because the writer queue was full", true,
				func(s services.AnalyticsWriterStats) float64 { return float64(s.Dropped) }},
			{"microgateway_analytics_writer_events_failed_total", "Analytics rows dropped because their batch failed", true,
				func(s services.AnalyticsWriterStats) float64 { return float64(s.Failed) }},
			{"microgateway_analytics_writer_events_expired_total", "Analytics rows deleted by retention", true,
				func(s services.AnalyticsWriterStats) float64 { return float64(s.Expired) }},
			{"microgateway_analytics_writer_batches_total", "Writer transactions", true,
				func(s services.AnalyticsWriterStats) float64 { return float64(s.Batches) }},
			{"microgateway_analytics_writer_commit_seconds_total", "Time spent in writer transactions", true,
				func(s services.AnalyticsWriterStats) float64 { return s.CommitSeconds }},
		} {
			value := m.value
			fn := func() float64 { return value(w.Stats()) }
			var err error
			if m.counter {
				err = metrics.RegisterCounterFunc(m.name, m.help, fn)
			} else {
				err = metrics.RegisterGaugeFunc(m.name, m.help, fn)
			}
			if err != nil {
				log.Warn().Err(err).Str("metric", m.name).Msg("Failed to register analytics writer metric")
			}
		}
	}

	if cfg.Database.Type != "sqlite" {
		return
	}
	path, ok := database.SQLiteFilePath(cfg.Database.DSN)
	if !ok {
		return
	}
	if err := metrics.RegisterFileSize("microgateway_sqlite_db_bytes", "Size of the gateway's SQLite database file", path); err != nil {
		log.Warn().Err(err).Msg("Failed to register SQLite size metric")
	}
	if err := metrics.RegisterFileSize("microgateway_sqlite_wal_bytes", "Size of the gateway's SQLite write-ahead log", path+"-wal"); err != nil {
		log.Warn().Err(err).Msg("Failed to register SQLite WAL size metric")
	}
}

// registerStorageFullMetrics exports whether the gateway's database is
// refusing writes for lack of room (a full volume, or a database at its
// size limit):
//
//   - microgateway_database_full is 1 while any kind of write is refused
//     for it, 0 otherwise. The gateway keeps serving; alert on it.
//   - microgateway_database_full_writes_total{write} counts the refused
//     writes by kind: configuration (a snapshot an edge could not apply and
//     store), control_contact, budget_blocks, budget_sync_position,
//     analytics.
func registerStorageFullMetrics() {
	if err := metrics.RegisterGaugeFunc(
		"microgateway_database_full",
		"1 while the gateway's database refuses writes for lack of room (full volume or size limit), 0 otherwise",
		func() float64 {
			if full, _, _ := database.StorageStatus(); full {
				return 1
			}
			return 0
		},
	); err != nil {
		log.Warn().Err(err).Msg("Failed to register database full metric")
	}
	if err := metrics.RegisterLabelledCounterFunc(
		"microgateway_database_full_writes_total",
		"Writes the gateway's database refused for lack of room, by kind (configuration, control_contact, budget_blocks, budget_sync_position, analytics)",
		"write",
		func() map[string]float64 { return storageFullValues(database.StorageFullFailures()) },
	); err != nil {
		log.Warn().Err(err).Msg("Failed to register database full writes metric")
	}
}

// storageFullValues is every kind of write as a series, at 0 until one is
// refused, so that an alert on the counter has something to watch.
func storageFullValues(counts map[string]uint64) map[string]float64 {
	values := map[string]float64{
		database.WriteConfiguration: 0, database.WriteControlContact: 0, database.WriteBudgetBlocks: 0,
		database.WriteBudgetSyncPosition: 0, database.WriteAnalytics: 0,
	}
	for kind, n := range counts {
		values[kind] = float64(n)
	}
	return values
}
