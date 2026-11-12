package metrics

import (
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

// sample reads a series without labels of its own from a scrape ("" when
// it is not there).
func sample(body, name string) string {
	m := regexp.MustCompile(`(?m)^` + name + `(?:\{[^}]*\})? (\S+)$`).FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return m[1]
}

// The database instruments report what the state function says, and
// nothing without one (SQLite).
func TestDatabaseState(t *testing.T) {
	h := Init()
	t.Cleanup(func() { SetDatabaseState(nil) })

	SetDatabaseState(nil)
	body := scrape(t, h)
	if sample(body, "aistudio_db_writable") != "" || sample(body, "aistudio_db_failovers_total") != "" {
		t.Fatalf("no database series without a state function:\n%s", body)
	}

	var writable atomic.Bool
	var failovers atomic.Int64
	writable.Store(true)
	SetDatabaseState(func() (bool, int64) { return writable.Load(), failovers.Load() })
	check := func(wantWritable, wantFailovers string) {
		t.Helper()
		body := scrape(t, h)
		if got := sample(body, "aistudio_db_writable"); got != wantWritable {
			t.Errorf("aistudio_db_writable = %q, want %q", got, wantWritable)
		}
		if got := sample(body, "aistudio_db_failovers_total"); got != wantFailovers {
			t.Errorf("aistudio_db_failovers_total = %q, want %q", got, wantFailovers)
		}
		if !strings.Contains(body, "# TYPE aistudio_db_failovers_total counter") {
			t.Error("aistudio_db_failovers_total is a counter")
		}
	}
	check("1", "0")
	writable.Store(false)
	failovers.Store(2)
	check("0", "2")
}
