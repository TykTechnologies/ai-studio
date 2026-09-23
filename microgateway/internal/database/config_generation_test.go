package database

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNormalizeSQLiteDSN(t *testing.T) {
	dsn, removed := normalizeSQLiteDSN("file:./data/microgateway.db?cache=shared&mode=rwc")
	if !removed {
		t.Fatal("cache=shared on a file database should be reported as removed")
	}
	path, raw, _ := strings.Cut(dsn, "?")
	q, _ := url.ParseQuery(raw)
	if path != "file:./data/microgateway.db" || q.Has("cache") || q.Get("mode") != "rwc" {
		t.Fatalf("unexpected DSN %q", dsn)
	}
	for k, want := range map[string]string{"_journal_mode": "WAL", "_busy_timeout": "5000", "_synchronous": "NORMAL", "_txlock": "immediate"} {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}

	// Explicit settings, in either spelling, are kept.
	dsn, _ = normalizeSQLiteDSN("gw.db?_journal=DELETE&_timeout=100")
	q, _ = url.ParseQuery(strings.SplitN(dsn, "?", 2)[1])
	if q.Get("_journal") != "DELETE" || q.Has("_journal_mode") || q.Get("_timeout") != "100" || q.Has("_busy_timeout") {
		t.Fatalf("explicit settings overridden: %q", dsn)
	}

	// In-memory databases need shared cache to be seen by every connection.
	for _, mem := range []string{"file::memory:?cache=shared", ":memory:", "file:x?mode=memory&cache=shared"} {
		if got, removed := normalizeSQLiteDSN(mem); got != mem || removed {
			t.Errorf("in-memory DSN changed: %q -> %q", mem, got)
		}
	}
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := Connect(DatabaseConfig{
		Type: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "gw.db") + "?cache=shared&mode=rwc",
		MaxOpenConns: 8, MaxIdleConns: 8, LogLevel: "silent",
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Logger = logger.Default.LogMode(logger.Silent)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestConnectEnablesWAL(t *testing.T) {
	db := openTestDB(t)
	var mode string
	if err := db.Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

func TestConfigWritesBumpGenerationRuntimeWritesDoNot(t *testing.T) {
	db := openTestDB(t)

	before := ConfigGeneration()
	if err := db.Create(&BudgetUsage{AppID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&AnalyticsEvent{RequestID: "r1"}).Error; err != nil {
		t.Fatal(err)
	}
	if ConfigGeneration() != before {
		t.Fatal("runtime-table writes must not invalidate configuration caches")
	}

	llm := &LLM{Name: "a", Slug: "a", Vendor: "openai", IsActive: true}
	if err := db.Create(llm).Error; err != nil {
		t.Fatal(err)
	}
	afterCreate := ConfigGeneration()
	if afterCreate == before {
		t.Fatal("create on llms must bump the generation")
	}
	if err := db.Model(llm).Update("default_model", "m").Error; err != nil {
		t.Fatal(err)
	}
	afterUpdate := ConfigGeneration()
	if afterUpdate == afterCreate {
		t.Fatal("update on llms must bump the generation")
	}
	// Raw statements (the sync's bulk deletes) cannot be attributed to a
	// table reliably, so they always invalidate.
	if err := db.Exec("DELETE FROM llms WHERE id = ?", llm.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ConfigGeneration() == afterUpdate {
		t.Fatal("raw statements must bump the generation")
	}
}

func TestGenCacheInvalidatesOnConfigWrite(t *testing.T) {
	db := openTestDB(t)
	c := NewGenCache[string, string]()
	loads := 0
	load := func() (string, error) {
		loads++
		var llm LLM
		err := db.Where("slug = ?", "a").First(&llm).Error
		return llm.DefaultModel, err
	}

	if err := db.Create(&LLM{Name: "a", Slug: "a", Vendor: "openai", DefaultModel: "m1", IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if v, err := c.Load("a", load); err != nil || v != "m1" {
			t.Fatalf("got %q %v", v, err)
		}
	}
	if loads != 1 {
		t.Fatalf("expected one load, got %d", loads)
	}

	if err := db.Model(&LLM{}).Where("slug = ?", "a").Update("default_model", "m2").Error; err != nil {
		t.Fatal(err)
	}
	if v, _ := c.Load("a", load); v != "m2" {
		t.Fatalf("stale value %q after a config write", v)
	}
	if _, err := c.Load("missing", func() (string, error) { return "", fmt.Errorf("boom") }); err == nil {
		t.Fatal("errors must be returned")
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatal("errors must not be cached")
	}
}

// With the old shared-cache DSN, concurrent readers and writers failed with
// "database table is locked". With the normalised DSN they must all succeed.
func TestConcurrentReadWriteDoesNotFailWithLocks(t *testing.T) {
	db := openTestDB(t)
	if err := db.Create(&LLM{Name: "a", Slug: "a", Vendor: "openai", IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 400)
	for i := 0; i < 200; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			errs <- db.Create(&BudgetUsage{AppID: uint(i)}).Error
		}(i)
		go func() {
			defer wg.Done()
			var llm LLM
			errs <- db.Where("slug = ?", "a").First(&llm).Error
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent access failed: %v", err)
		}
	}
}
