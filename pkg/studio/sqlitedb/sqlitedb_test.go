package sqlitedb_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/studio"
	_ "github.com/TykTechnologies/midsommar/v2/pkg/studio/sqlitedb"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// The standalone binary opens SQLite this way: importing the package is all
// it takes for studio.OpenDatabase to accept DatabaseType "sqlite".
func TestImportEnablesSQLite(t *testing.T) {
	conf := &config.AppConf{DatabaseType: "sqlite", DatabaseURL: filepath.Join(t.TempDir(), "studio.db")}
	db, err := studio.OpenDatabase(conf)
	if err != nil {
		t.Fatalf("OpenDatabase(sqlite): %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if got := db.Dialector.Name(); got != "sqlite" {
		t.Fatalf("dialector = %q, want sqlite", got)
	}
}

// Creating an LLM reads before it writes (the slug check runs in BeforeSave).
// Another connection committing a write between the two must not fail the
// create: in a deferred transaction SQLite cannot upgrade the read lock
// (SQLITE_BUSY / SQLITE_BUSY_SNAPSHOT, which busy_timeout does not retry),
// so the driver opens file databases with _txlock=immediate and a busy
// timeout.
func TestLLMCreateSurvivesConcurrentWriter(t *testing.T) {
	conf := &config.AppConf{DatabaseType: "sqlite", DatabaseURL: filepath.Join(t.TempDir(), "studio.db")}
	db, err := studio.OpenDatabase(conf)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := models.InitModels(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE concurrent_writes (id INTEGER PRIMARY KEY, at TEXT)").Error; err != nil {
		t.Fatal(err)
	}

	// After the slug check has read, and before the insert, another
	// connection writes. Under immediate transactions that write waits for
	// the create to commit, so the hook only waits briefly for it.
	writerDone := make(chan error, 1)
	fired := false
	err = db.Callback().Create().Before("gorm:create").Register("test:concurrent_writer", func(tx *gorm.DB) {
		if fired || tx.Statement.Table != "llms" {
			return
		}
		fired = true
		go func() {
			writerDone <- db.Exec("INSERT INTO concurrent_writes (at) VALUES (?)", time.Now().String()).Error
		}()
		select {
		case err := <-writerDone:
			writerDone <- err
		case <-time.After(300 * time.Millisecond):
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	llm := &models.LLM{Name: "Raced LLM", Vendor: models.OPENAI, Active: true, APIEndpoint: "https://api.openai.com/v1"}
	if err := llm.Create(db); err != nil {
		t.Fatalf("LLM create with a concurrent writer: %v", err)
	}
	if !fired {
		t.Fatal("the concurrent writer never ran")
	}
	select {
	case err := <-writerDone:
		if err != nil {
			t.Fatalf("the concurrent write failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the concurrent write never finished")
	}
}
