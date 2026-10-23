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
