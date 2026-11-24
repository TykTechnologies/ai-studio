package logger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	gormlogger "github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// GormLogger implements gorm's logger.Interface using zerolog
type GormLogger struct {
	SlowThreshold         time.Duration
	IgnoreRecordNotFound  bool
	// LogParams logs the SQL with its values. Off, a statement is logged
	// with its placeholders only (ParamsFilter, and recorderParamsFilter
	// for statements that end in Scan), at every level: the
	// values are rows' contents, credentials included (an LLM's API key, a
	// cluster event's payload). DB_LOG_SQL_PARAMS=true turns it on, for
	// local debugging only.
	LogParams bool
}

// sqlParamsEnv opts SQL values back into the logs.
const sqlParamsEnv = "DB_LOG_SQL_PARAMS"

var sqlParamsWarnOnce sync.Once

// sqlParamsEnabled reports whether DB_LOG_SQL_PARAMS opts SQL values into
// the logs.
func sqlParamsEnabled() bool {
	return strings.EqualFold(os.Getenv(sqlParamsEnv), "true")
}

// gorm does not run every statement through the logger's ParamsFilter. A
// statement that ends in Scan (Raw(...).Scan, Model(...).Select(...).Scan:
// about 70 call sites, the cluster event insert among them) is run with
// gorm's own recording logger in place of ours, and the text that recorder
// builds is what our Trace is then handed. The recorder's filter is the
// package variable below, a no-op by default, so those statements were
// logged with their values whatever DB_LOG_SQL_PARAMS said: at debug level
// always, and at any level when one failed or was slow. The same rule is
// set here for it. The variable is process-wide, which is what is wanted:
// no gorm.DB in this process logs Scan statements with their values unless
// the opt-in is set.
func init() {
	gormlogger.RecorderParamsFilter = recorderParamsFilter
}

// recorderParamsFilter drops a Scan statement's values unless
// DB_LOG_SQL_PARAMS=true. It reads the setting on each call: the recorder
// has no logger instance to ask.
func recorderParamsFilter(_ context.Context, sql string, params ...interface{}) (string, []interface{}) {
	if sqlParamsEnabled() {
		return sql, params
	}
	return sql, nil
}

// NewGormLogger creates a new GORM logger that uses zerolog
func NewGormLogger() *GormLogger {
	logParams := sqlParamsEnabled()
	if logParams {
		sqlParamsWarnOnce.Do(func() {
			Log.Warn().Msgf("%s=true: SQL statements are logged with their values, which include credentials and other row contents. Use it for local debugging only.", sqlParamsEnv)
		})
	}
	return &GormLogger{
		SlowThreshold:        200 * time.Millisecond,
		IgnoreRecordNotFound: true,
		LogParams:            logParams,
	}
}

// ParamsFilter implements gorm.ParamsFilter: without LogParams, a statement
// is logged with no values.
func (l *GormLogger) ParamsFilter(_ context.Context, sql string, params ...interface{}) (string, []interface{}) {
	if l.LogParams {
		return sql, params
	}
	return sql, nil
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
