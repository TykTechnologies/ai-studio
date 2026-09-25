package pathcheck

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// unwritableDir returns a directory the test process cannot write to, or
// skips when permissions are not enforced (root, Windows).
func unwritableDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission checks are not enforced for this user")
	}
	dir := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	return dir
}

func TestSQLitePath(t *testing.T) {
	tests := []struct {
		dsn    string
		path   string
		onDisk bool
	}{
		{"file:./data/microgateway.db?mode=rwc", "./data/microgateway.db", true},
		{"file:/app/data/edge.db?cache=shared&mode=rwc", "/app/data/edge.db", true},
		{"midsommar.db", "midsommar.db", true},
		{"file:///var/lib/x.db", "/var/lib/x.db", true},
		{":memory:", "", false},
		{"file::memory:?cache=shared", "", false},
		{"file:test?mode=memory&cache=shared", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		path, onDisk := SQLitePath(tt.dsn)
		if path != tt.path || onDisk != tt.onDisk {
			t.Errorf("SQLitePath(%q) = (%q, %v), want (%q, %v)", tt.dsn, path, onDisk, tt.path, tt.onDisk)
		}
	}
}

func TestCheckStatuses(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "present.yaml")
	writeFile(t, existing)
	db := filepath.Join(dir, "edge.db")
	writeFile(t, db)

	tests := []struct {
		name    string
		entry   Entry
		status  Status
		problem bool
	}{
		{"readable file", Entry{Name: "X", Value: existing, Kind: File, Source: SourceSet}, StatusOK, false},
		// The incident this exists for: PLUGINS_CONFIG_PATH set to a path the
		// image no longer has, which the loader treats as "no plugins".
		{"set file missing", Entry{Name: "X", Value: filepath.Join(dir, "nope.yaml"), Kind: File, Source: SourceSet}, StatusMissing, true},
		{"default file missing is fine", Entry{Name: "X", Value: filepath.Join(dir, ".env"), Default: ".env", Kind: File, Source: SourceDefault}, StatusMissing, false},
		{"required default file missing", Entry{Name: "X", Value: filepath.Join(dir, "nope"), Kind: File, Source: SourceDefault, Required: true}, StatusMissing, true},
		{"file is a directory", Entry{Name: "X", Value: dir, Kind: File, Source: SourceSet}, StatusWrongType, true},
		{"optional unset", Entry{Name: "X", Kind: File, Source: SourceUnset}, StatusUnset, false},
		{"required unset", Entry{Name: "X", Kind: File, Source: SourceUnset, Required: true}, StatusUnset, true},
		{"writable dir", Entry{Name: "X", Value: dir, Kind: Dir, Source: SourceSet}, StatusOK, false},
		{"dir creatable", Entry{Name: "X", Value: filepath.Join(dir, "a", "b"), Kind: Dir, Source: SourceDefault}, StatusWillCreate, false},
		{"dir is a file", Entry{Name: "X", Value: existing, Kind: Dir, Source: SourceSet}, StatusWrongType, true},
		{"sqlite existing", Entry{Name: "X", Value: "file:" + db + "?mode=rwc", Kind: SQLite, Source: SourceSet}, StatusOK, false},
		{"sqlite created on first use", Entry{Name: "X", Value: "file:" + filepath.Join(dir, "new.db") + "?mode=rwc", Kind: SQLite, Source: SourceDefault}, StatusWillCreate, false},
		// SQLite creates the file but not its directory; the gateway then
		// fails with "unable to open database file".
		{"sqlite directory missing", Entry{Name: "X", Value: "file:" + filepath.Join(dir, "data", "new.db") + "?mode=rwc", Kind: SQLite, Source: SourceDefault}, StatusMissing, true},
		{"sqlite in memory", Entry{Name: "X", Value: ":memory:", Kind: SQLite, Source: SourceSet}, StatusInMemory, false},
		{"output file creatable", Entry{Name: "X", Value: filepath.Join(dir, "audit", "audit.log"), Kind: OutputFile, Source: SourceDefault}, StatusWillCreate, false},
		{"output file existing", Entry{Name: "X", Value: existing, Kind: OutputFile, Source: SourceSet}, StatusOK, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Check([]Entry{tt.entry})[0]
			if r.Status != tt.status {
				t.Fatalf("status = %s (%s), want %s", r.Status, r.Detail, tt.status)
			}
			if r.Problem() != tt.problem {
				t.Fatalf("Problem() = %v, want %v", r.Problem(), tt.problem)
			}
		})
	}
}

func TestCheckPermissions(t *testing.T) {
	ro := unwritableDir(t)

	r := Check([]Entry{{Name: "X", Value: ro, Kind: Dir, Source: SourceSet}})[0]
	if r.Status != StatusNotWritable || !r.Problem() {
		t.Errorf("read-only dir: status %s problem %v", r.Status, r.Problem())
	}

	// A default directory the binary would write to is still a problem.
	r = Check([]Entry{{Name: "X", Value: filepath.Join(ro, "cache", "plugins"), Kind: Dir, Source: SourceDefault}})[0]
	if r.Status != StatusMissing || !r.Problem() {
		t.Errorf("uncreatable dir: status %s problem %v", r.Status, r.Problem())
	}

	// The distroless trap: a database file that is writable in a directory
	// that is not, so SQLite cannot create its journal.
	parent := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(parent, "edge.db")
	writeFile(t, db)
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	r = Check([]Entry{{Name: "X", Value: db, Kind: SQLite, Source: SourceSet}})[0]
	if r.Status != StatusNotWritable || !strings.Contains(r.Detail, "journal") {
		t.Errorf("sqlite in read-only dir: status %s detail %q", r.Status, r.Detail)
	}

	unreadable := filepath.Join(t.TempDir(), "secret.pem")
	writeFile(t, unreadable)
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	r = Check([]Entry{{Name: "X", Value: unreadable, Kind: File, Source: SourceSet}})[0]
	if r.Status != StatusNotReadable || !r.Problem() {
		t.Errorf("unreadable file: status %s problem %v", r.Status, r.Problem())
	}
}

func TestSourceFromEnvironment(t *testing.T) {
	t.Setenv("PATHCHECK_TEST_SET", "/x")
	t.Setenv("PATHCHECK_TEST_EMPTY", "")

	tests := []struct {
		entry Entry
		want  Source
	}{
		{Entry{Name: "PATHCHECK_TEST_SET", Value: "/x", Default: "/d"}, SourceSet},
		{Entry{Name: "PATHCHECK_TEST_EMPTY", Value: "/d", Default: "/d"}, SourceDefault},
		{Entry{Name: "PATHCHECK_TEST_ABSENT", Value: "/d", Default: "/d"}, SourceDefault},
		{Entry{Name: "PATHCHECK_TEST_ABSENT"}, SourceUnset},
		{Entry{Name: "-env", Value: "x", Source: SourceSet}, SourceSet},
	}
	for _, tt := range tests {
		if got := sourceOf(tt.entry); got != tt.want {
			t.Errorf("sourceOf(%s) = %s, want %s", tt.entry.Name, got, tt.want)
		}
	}
}

func TestResolvedIsAbsolute(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, "mgw-analytics.yaml"))

	r := Check([]Entry{{Name: "X", Value: "./mgw-analytics.yaml", Kind: File, Source: SourceSet}})[0]
	if r.Status != StatusOK {
		t.Fatalf("status %s (%s)", r.Status, r.Detail)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(dir, "mgw-analytics.yaml"))
	got, _ := filepath.EvalSymlinks(r.Resolved)
	if got != want {
		t.Errorf("resolved = %s, want %s", r.Resolved, want)
	}
}

func TestLogLinesAndLevels(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	l := zerolog.New(&buf)

	problems := Log(l, "microgateway", Check([]Entry{
		{Name: "PLUGINS_CONFIG_PATH", Value: filepath.Join(dir, "missing.yaml"), Kind: File, Source: SourceSet, Use: "data collection plugins"},
		{Name: "OCI_PLUGINS_CACHE_DIR", Value: dir, Kind: Dir, Source: SourceSet, Use: "plugin cache"},
		{Name: "TLS_CERT_PATH", Kind: File, Source: SourceUnset, Use: "TLS"},
	}))
	if problems != 1 {
		t.Fatalf("problems = %d, want 1", problems)
	}

	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("bad log line %q: %v", raw, err)
		}
		lines = append(lines, m)
	}
	if len(lines) != 5 { // header, three paths, summary
		t.Fatalf("got %d lines, want 5:\n%s", len(lines), buf.String())
	}
	if lines[0]["working_dir"] == nil || lines[0]["uid"] == nil {
		t.Errorf("header lacks working_dir/uid: %v", lines[0])
	}
	byName := map[string]map[string]any{}
	for _, m := range lines[1:4] {
		if m["message"] != "startup path" {
			t.Errorf("path line message = %v", m["message"])
		}
		byName[m["name"].(string)] = m
	}
	if m := byName["PLUGINS_CONFIG_PATH"]; m["level"] != "warn" || m["status"] != "missing" {
		t.Errorf("missing plugin config line: %v", m)
	}
	if m := byName["OCI_PLUGINS_CACHE_DIR"]; m["level"] != "info" || m["status"] != "ok" {
		t.Errorf("cache dir line: %v", m)
	}
	if m := byName["TLS_CERT_PATH"]; m["level"] != "info" || m["status"] != "unset" || m["path"] != nil {
		t.Errorf("unset line: %v", m)
	}
	if lines[4]["level"] != "warn" || lines[4]["problems"] != float64(1) {
		t.Errorf("summary: %v", lines[4])
	}
}
