package logger

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

type credentialRow struct {
	ID     uint
	APIKey string
}

// logSQL runs an INSERT through l at debug level, as a slow query and as a
// failed one, and returns what was logged.
func logSQL(t *testing.T, l *GormLogger) string {
	t.Helper()
	var buf bytes.Buffer
	prevLog, prevLevel := Log, currentLevel
	Log, currentLevel = zerolog.New(&buf).Level(zerolog.DebugLevel), zerolog.DebugLevel
	t.Cleanup(func() { Log, currentLevel = prevLog, prevLevel })

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: l})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&credentialRow{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&credentialRow{ID: 1, APIKey: "sk-fake-logged-key"}).Error; err != nil {
		t.Fatal(err)
	}
	// A failed statement (a duplicate key) is logged at WARN at any level.
	_ = db.Create(&credentialRow{ID: 1, APIKey: "sk-fake-failed-key"}).Error
	// So is a slow one.
	l.SlowThreshold = time.Nanosecond
	_ = db.Create(&credentialRow{ID: 2, APIKey: "sk-fake-slow-key"}).Error
	l.SlowThreshold = 200 * time.Millisecond
	return buf.String()
}

// SQL is logged without its values at every level (debug, the slow-query
// and error warnings): an INSERT into llms or cluster_events carries
// credentials (aa5).
func TestGormLoggerLogsNoSQLValues(t *testing.T) {
	t.Setenv(sqlParamsEnv, "")
	out := logSQL(t, NewGormLogger())
	if !strings.Contains(out, "INSERT INTO") {
		t.Fatalf("no statement was logged: %s", out)
	}
	if !strings.Contains(out, "slow_query") || !strings.Contains(out, `"level":"warn"`) {
		t.Fatalf("the slow and failed statements were not logged: %s", out)
	}
	for _, v := range []string{"sk-fake-logged-key", "sk-fake-failed-key", "sk-fake-slow-key"} {
		if strings.Contains(out, v) {
			t.Errorf("a value reached the log: %s", v)
		}
	}
}

// DB_LOG_SQL_PARAMS=true brings the values back, for local debugging.
func TestGormLoggerSQLValuesOptIn(t *testing.T) {
	t.Setenv(sqlParamsEnv, "true")
	l := NewGormLogger()
	if !l.LogParams {
		t.Fatal("DB_LOG_SQL_PARAMS=true did not turn values on")
	}
	if out := logSQL(t, l); !strings.Contains(out, "sk-fake-logged-key") {
		t.Fatalf("the opt-in logged no values: %s", out)
	}
}

// logScanSQL runs statements that end in Scan through l, carrying marker
// values: one at debug level, one that fails and one that is slow (both
// logged at WARN at any level). It returns what was logged at minLevel and
// above.
func logScanSQL(t *testing.T, l *GormLogger, minLevel zerolog.Level) string {
	t.Helper()
	var buf bytes.Buffer
	prevLog, prevLevel := Log, currentLevel
	Log, currentLevel = zerolog.New(&buf).Level(minLevel), minLevel
	t.Cleanup(func() { Log, currentLevel = prevLog, prevLevel })

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: l})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&credentialRow{}); err != nil {
		t.Fatal(err)
	}

	// The shape of the cluster event insert: Raw(...).Scan(&id).
	var id uint
	if err := db.Raw("INSERT INTO credential_rows (id, api_key) VALUES (?, ?) RETURNING id", 7, "sk-fake-scan-key").Scan(&id).Error; err != nil {
		t.Fatal(err)
	}
	if id != 7 {
		t.Fatalf("the statement did not run: id = %d", id)
	}
	// A query built with the model API that ends in Scan.
	var key string
	if err := db.Model(&credentialRow{}).Select("api_key").Where("api_key = ?", "sk-fake-scan-where").Scan(&key).Error; err != nil {
		t.Fatal(err)
	}
	// A failing Scan statement.
	var n int
	if err := db.Raw("SELECT count(*) FROM no_such_table WHERE secret = ?", "sk-fake-scan-failed").Scan(&n).Error; err == nil {
		t.Fatal("the failing statement did not fail")
	}
	// A slow one.
	l.SlowThreshold = time.Nanosecond
	_ = db.Raw("SELECT count(*) FROM credential_rows WHERE api_key = ?", "sk-fake-scan-slow").Scan(&n).Error
	l.SlowThreshold = 200 * time.Millisecond
	return buf.String()
}

var scanMarkers = []string{"sk-fake-scan-key", "sk-fake-scan-where", "sk-fake-scan-failed", "sk-fake-scan-slow"}

// A statement that ends in Scan is logged without its values too, at debug
// level and in the failed and slow warnings that production log levels
// show. gorm runs such a statement with its own recording logger, which
// did not apply our filter: every Raw(...).Scan was logged with its values,
// the cluster event insert with the whole event payload.
func TestGormLoggerLogsNoScanValues(t *testing.T) {
	for _, level := range []zerolog.Level{zerolog.DebugLevel, zerolog.InfoLevel, zerolog.WarnLevel} {
		t.Run(level.String(), func(t *testing.T) {
			t.Setenv(sqlParamsEnv, "")
			out := logScanSQL(t, NewGormLogger(), level)

			if !strings.Contains(out, "no_such_table") || !strings.Contains(out, "slow_query") {
				t.Fatalf("the failed and slow Scan statements were not logged: %s", out)
			}
			if level == zerolog.DebugLevel && !strings.Contains(out, "RETURNING id") {
				t.Fatalf("the Scan statement was not logged at debug level: %s", out)
			}
			if level != zerolog.DebugLevel && strings.Contains(out, "RETURNING id") {
				t.Errorf("an ordinary statement was logged above debug level: %s", out)
			}
			for _, v := range scanMarkers {
				if strings.Contains(out, v) {
					t.Errorf("a Scan statement's value reached the log at %s: %s", level, v)
				}
			}
		})
	}
}

// DB_LOG_SQL_PARAMS=true brings the values of Scan statements back as well.
func TestGormLoggerScanValuesOptIn(t *testing.T) {
	t.Setenv(sqlParamsEnv, "true")
	out := logScanSQL(t, NewGormLogger(), zerolog.DebugLevel)
	for _, v := range scanMarkers {
		if !strings.Contains(out, v) {
			t.Errorf("the opt-in did not log %s: %s", v, out)
		}
	}
}
