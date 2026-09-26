// internal/database/connection.go
package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Type            string
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	AutoMigrate     bool
	LogLevel        string
}

// Connect establishes a database connection based on configuration
func Connect(config DatabaseConfig) (*gorm.DB, error) {
	var db *gorm.DB
	var err error

	// Configure GORM logger
	gormConfig := &gorm.Config{
		Logger: getGormLogger(config.LogLevel),
	}

	// Connect based on database type
	switch config.Type {
	case "postgres":
		db, err = gorm.Open(postgres.Open(config.DSN), gormConfig)
	case "sqlite":
		dsn, removedShared := normalizeSQLiteDSN(config.DSN)
		if removedShared {
			log.Printf("SQLite: ignoring cache=shared in DATABASE_DSN for a file database; it causes table-level lock errors under concurrent load")
		}
		db, err = gorm.Open(openSQLite(dsn), gormConfig)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := EnsureConfigGenerationCallbacks(db); err != nil {
		return nil, fmt.Errorf("failed to register config generation callbacks: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime(config))

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// connMaxLifetime is DB_CONN_MAX_LIFETIME for a server database and 0 (never
// recycle) for SQLite. Recycling lets a server database rebalance
// connections; a SQLite connection is a handle on a local file, and
// recycling only discarded its page cache. Worse, a pool whose connections
// all opened together under load then expired together every lifetime (5 min
// by default): on AWS the edge showed small request pile-ups exactly 301 s
// apart while every connection reopened cold.
func connMaxLifetime(config DatabaseConfig) time.Duration {
	if config.Type == "sqlite" {
		return 0
	}
	return config.ConnMaxLifetime
}

// OpenWriter returns the handle for the gateway's background writes (analytics
// events, budget usage). For a file-backed SQLite database it is a second pool
// on the same file holding one connection. SQLite has a single writer, so more
// connections only queue on the write lock, and while they wait (up to
// busy_timeout) they hold connections from the pool that serves request-path
// reads. With their own connection, the background writes wait for each other
// instead, and the request path always finds a free connection.
//
// For Postgres and in-memory SQLite it returns db: Postgres has no single
// writer, and a second pool would not see an in-memory database.
func OpenWriter(config DatabaseConfig, db *gorm.DB) (*gorm.DB, error) {
	if config.Type != "sqlite" || isInMemorySQLite(config.DSN) {
		return db, nil
	}
	dsn, _ := normalizeSQLiteDSN(config.DSN)
	w, err := gorm.Open(openSQLite(dsn), &gorm.Config{Logger: getGormLogger(config.LogLevel)})
	if err != nil {
		return nil, fmt.Errorf("failed to open database writer: %w", err)
	}
	if err := EnsureConfigGenerationCallbacks(w); err != nil {
		return nil, fmt.Errorf("failed to register config generation callbacks on writer: %w", err)
	}
	sqlDB, err := w.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB for writer: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database writer: %w", err)
	}
	return w, nil
}

// Migrate runs auto-migration for all models
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&APIToken{},
		&LLM{},
		&App{},
		&Credential{},
		&AppLLM{},
		&ModelPrice{},
		&BudgetUsage{},
		&AnalyticsEvent{},
		&Filter{},
		&LLMFilter{},
		&Plugin{},
		&LLMPlugin{},
		&PluginKV{},
		// Hub-and-Spoke models
		&EdgeInstance{},
		&ControlPayload{},
		&SyncState{},
		&BudgetBlock{},
		// Model Router models (Enterprise)
		&ModelRouter{},
		&ModelPool{},
		&PoolVendor{},
		&ModelMapping{},
		// Tool, Datasource, and OAuth models (for edge tool/datasource proxy)
		&Tool{},
		&Datasource{},
		&OAuthClientEdge{},
		&AccessTokenEdge{},
		&AppTool{},
		&AppModelRouter{},
		&SemanticRouter{},
		&AppSemanticRouter{},
		&AppDatasource{},
		&ToolFilter{},
	)
}

// getGormLogger returns appropriate GORM logger based on log level
func getGormLogger(logLevel string) logger.Interface {
	var level logger.LogLevel

	switch logLevel {
	case "silent":
		level = logger.Silent
	case "error":
		level = logger.Error
	case "warn":
		level = logger.Warn
	case "info":
		level = logger.Info
	default:
		level = logger.Warn
	}

	return logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  level,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
}

// Close closes the database connection
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}
	return sqlDB.Close()
}

// IsHealthy checks if the database connection is healthy
func IsHealthy(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return sqlDB.PingContext(ctx)
}