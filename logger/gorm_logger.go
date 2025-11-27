package logger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// GormLogger implements gorm's logger.Interface using zerolog
type GormLogger struct {
	SlowThreshold         time.Duration
	IgnoreRecordNotFound  bool
}

// NewGormLogger creates a new GORM logger that uses zerolog
func NewGormLogger() *GormLogger {
	return &GormLogger{
		SlowThreshold:        200 * time.Millisecond,
		IgnoreRecordNotFound: true,
	}
}

// LogMode implements gorm's logger.Interface
func (l *GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	// We control log level via zerolog global level, so just return self
	return l
}

// Info implements gorm's logger.Interface
func (l *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	Log.Info().Msgf(msg, data...)
}

// Warn implements gorm's logger.Interface
func (l *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	Log.Warn().Msgf(msg, data...)
}

// Error implements gorm's logger.Interface
func (l *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	Log.Error().Msgf(msg, data...)
}

// Trace implements gorm's logger.Interface - logs SQL queries
func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()

	// Determine log level based on error and duration
	var logEvent *zerolog.Event

	switch {
	case err != nil && (!errors.Is(err, gorm.ErrRecordNotFound) || !l.IgnoreRecordNotFound):
		// Error case (excluding record not found if ignored)
		logEvent = Log.Warn()
		logEvent.Err(err)
	case elapsed > l.SlowThreshold && l.SlowThreshold != 0:
		// Slow query warning
		logEvent = Log.Warn()
		logEvent.Str("slow_query", fmt.Sprintf(">= %v", l.SlowThreshold))
	default:
		// Normal trace - only log at debug level
		if currentLevel > zerolog.DebugLevel {
			return // Skip trace logs when not in debug mode
		}
		logEvent = Log.Debug()
	}

	logEvent.
		Dur("elapsed", elapsed).
		Int64("rows", rows).
		Str("sql", sql).
		Msg("gorm")
}

// GetGormConfig returns a gorm.Config with the zerolog-based logger
func GetGormConfig() *gorm.Config {
	return &gorm.Config{
		Logger: NewGormLogger(),
	}
}
