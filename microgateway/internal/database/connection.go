// internal/database/connection.go
package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
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
		db, err = gorm.Open(sqlite.Open(config.DSN), gormConfig)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(config.ConnMaxLifetime)

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
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