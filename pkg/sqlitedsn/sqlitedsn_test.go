package sqlitedsn

import (
	"net/url"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	dsn, removed := Normalize("file:./data/studio.db?cache=shared&mode=rwc")
	if !removed {
		t.Fatal("cache=shared on a file database should be reported as removed")
	}
	path, raw, _ := strings.Cut(dsn, "?")
	q, _ := url.ParseQuery(raw)
	if path != "file:./data/studio.db" || q.Has("cache") || q.Get("mode") != "rwc" {
		t.Fatalf("unexpected DSN %q", dsn)
	}
	for k, want := range map[string]string{"_journal_mode": "WAL", "_busy_timeout": "5000", "_synchronous": "NORMAL", "_txlock": "immediate"} {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}

	// A bare path, Studio's default form, gets the defaults too.
	dsn, removed = Normalize("midsommar.db")
	if removed || !strings.HasPrefix(dsn, "midsommar.db?") || !strings.Contains(dsn, "_txlock=immediate") {
		t.Fatalf("bare path: %q (removed %v)", dsn, removed)
	}

	// Explicit settings, in either spelling, are kept.
	dsn, _ = Normalize("gw.db?_journal=DELETE&_timeout=100&_txlock=deferred&_sync=FULL")
	q, _ = url.ParseQuery(strings.SplitN(dsn, "?", 2)[1])
	if q.Get("_journal") != "DELETE" || q.Has("_journal_mode") || q.Get("_timeout") != "100" || q.Has("_busy_timeout") ||
		q.Get("_txlock") != "deferred" || q.Get("_sync") != "FULL" || q.Has("_synchronous") {
		t.Fatalf("explicit settings overridden: %q", dsn)
	}

	// In-memory databases need shared cache to be seen by every connection.
	for _, mem := range []string{"file::memory:?cache=shared", ":memory:", "file:x?mode=memory&cache=shared"} {
		if got, removed := Normalize(mem); got != mem || removed {
			t.Errorf("in-memory DSN changed: %q -> %q", mem, got)
		}
	}
}

func TestFilePath(t *testing.T) {
	for dsn, want := range map[string]string{
		"midsommar.db":                   "midsommar.db",
		"file:/data/studio.db?mode=rwc":  "/data/studio.db",
		"file:./gw.db?_journal_mode=WAL": "./gw.db",
	} {
		if got, ok := FilePath(dsn); !ok || got != want {
			t.Errorf("FilePath(%q) = %q, %v; want %q", dsn, got, ok, want)
		}
	}
	for _, dsn := range []string{":memory:", "file:x?mode=memory&cache=shared", ""} {
		if _, ok := FilePath(dsn); ok {
			t.Errorf("FilePath(%q) reported a file", dsn)
		}
	}
}
