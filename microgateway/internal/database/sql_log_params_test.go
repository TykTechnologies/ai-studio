package database

import (
	"context"
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
