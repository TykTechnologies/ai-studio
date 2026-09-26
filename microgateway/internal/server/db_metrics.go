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
