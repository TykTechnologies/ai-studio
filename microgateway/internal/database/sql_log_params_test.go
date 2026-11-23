package database

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// The edge logs SQL with placeholders, never the values (rows' contents,
// credentials included), at every log level (aa5); DB_LOG_SQL_PARAMS=true
// brings them back for local debugging.
func TestGormLoggerLogsNoSQLValues(t *testing.T) {
	for _, level := range []string{"info", "warn", ""} {
		t.Setenv("DB_LOG_SQL_PARAMS", "")
		filter, ok := getGormLogger(level).(gorm.ParamsFilter)
		if !ok {
			t.Fatalf("level %q: the logger does not filter parameters", level)
		}
		if _, params := filter.ParamsFilter(context.Background(), "INSERT INTO llms (api_key) VALUES (?)", "sk-fake-edge-key"); len(params) != 0 {
			t.Errorf("level %q: values are logged: %v", level, params)
		}
	}

	t.Setenv("DB_LOG_SQL_PARAMS", "true")
	filter := getGormLogger("info").(gorm.ParamsFilter)
	if _, params := filter.ParamsFilter(context.Background(), "INSERT INTO llms (api_key) VALUES (?)", "sk-fake-edge-key"); len(params) != 1 {
		t.Errorf("DB_LOG_SQL_PARAMS=true logged no values")
	}
}

// logCapture collects what the gorm logger prints.
type logCapture struct{ lines []string }

func (c *logCapture) Printf(format string, args ...interface{}) {
	c.lines = append(c.lines, fmt.Sprintf(format, args...))
}

// edgeScanLog runs statements that end in Scan through the edge's logger at
// logLevel, with marker values: an ordinary one, a failing one and (at the
// "info" level, which logs every statement) a model query. It returns what
// was logged.
func edgeScanLog(t *testing.T, logLevel string) string {
	t.Helper()
	out := &logCapture{}
	db, err := gorm.Open(openSQLite("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: newGormLogger(logLevel, out)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE scan_probe (id INTEGER PRIMARY KEY, api_key TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	var id uint
	if err := db.Raw("INSERT INTO scan_probe (id, api_key) VALUES (?, ?) RETURNING id", 7, "sk-fake-edge-scan-key").Scan(&id).Error; err != nil {
		t.Fatal(err)
	}
	if id != 7 {
		t.Fatalf("the statement did not run: id = %d", id)
	}
	var n int
	if err := db.Raw("SELECT count(*) FROM no_such_table WHERE secret = ?", "sk-fake-edge-scan-failed").Scan(&n).Error; err == nil {
		t.Fatal("the failing statement did not fail")
	}
	return strings.Join(out.lines, "\n")
}

// A statement that ends in Scan is logged without its values on the edge
// too. gorm runs it with its own recording logger, which ParameterizedQueries
// does not reach: a failed Raw(...).Scan was logged with its values at the
// default level, and every one at "info".
func TestEdgeLogsNoScanValues(t *testing.T) {
	for _, level := range []string{"info", "warn", "error"} {
		t.Run(level, func(t *testing.T) {
			t.Setenv("DB_LOG_SQL_PARAMS", "")
			out := edgeScanLog(t, level)
			if !strings.Contains(out, "no_such_table") {
				t.Fatalf("the failed Scan statement was not logged: %q", out)
			}
			if level == "info" && !strings.Contains(out, "RETURNING id") {
				t.Fatalf("the Scan statement was not logged at info: %q", out)
			}
			for _, v := range []string{"sk-fake-edge-scan-key", "sk-fake-edge-scan-failed"} {
				if strings.Contains(out, v) {
					t.Errorf("a Scan statement's value reached the edge's log at %s: %s", level, v)
				}
			}
		})
	}
}

// DB_LOG_SQL_PARAMS=true brings the values of Scan statements back.
func TestEdgeScanValuesOptIn(t *testing.T) {
	t.Setenv("DB_LOG_SQL_PARAMS", "true")
	out := edgeScanLog(t, "info")
	for _, v := range []string{"sk-fake-edge-scan-key", "sk-fake-edge-scan-failed"} {
		if !strings.Contains(out, v) {
			t.Errorf("the opt-in did not log %s: %q", v, out)
		}
	}
}
