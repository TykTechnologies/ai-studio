package chat_session

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// A failed statement on the PostgreSQL chat queue's connection is logged without its values. The connection
// was opened with an empty gorm.Config, so gorm's default logger printed a
// failed or slow statement whole, and its statements carry a chat message. The
// configuration the connection is opened with is run here on SQLite: what is
// logged depends on the configuration, not on the database.
func TestFailedStatementIsLoggedWithoutItsValues(t *testing.T) {
	t.Setenv("DB_LOG_SQL_PARAMS", "")
	var buf bytes.Buffer
	saved := logger.Log
	logger.Log = zerolog.New(&buf)
	t.Cleanup(func() { logger.Log = saved })

	db, err := gorm.Open(sqlite.Open(":memory:"), postgresQueueGormConfig())
	if err != nil {
		t.Fatal(err)
	}

	const marker = "MARKER-content-that-must-not-be-logged"
	// Through gorm's ordinary path, and through Scan.
	if err := db.Exec("INSERT INTO queue_messages_missing (payload) VALUES (?)", marker).Error; err == nil {
		t.Fatal("the statement did not fail")
	}
	var n int
	if err := db.Raw("SELECT count(*) FROM queue_messages_missing WHERE payload = ?", marker).Scan(&n).Error; err == nil {
		t.Fatal("the Scan statement did not fail")
	}

	out := buf.String()
	if strings.Count(out, "queue_messages_missing") < 2 {
		t.Fatalf("the failed statements were not logged through the application's logger: %q", out)
	}
	if strings.Contains(out, marker) {
		t.Errorf("a failed statement's value reached the log: %s", out)
	}
}
