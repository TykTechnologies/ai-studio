// Package pathcheck reports, once at startup, every filesystem path a binary
// is configured with: where the value came from (set, or the built-in
// default), the absolute path it resolves to from the working directory, and
// whether the file or directory is there and usable by the process user.
//
// Each path is one log line with the message "startup path", so
//
//	docker logs <container> 2>&1 | grep 'startup path'
//
// shows the whole picture, and problems are logged at WARN. The report never
// stops startup. A misconfigured path often fails silently (a missing plugin
// config file loads no plugins, a missing .env file is skipped), and this is
// where that shows up.
package pathcheck

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
)

// Kind says how a path is used, which decides what is checked.
type Kind string

const (
	// File is read: it must exist and be readable.
	File Kind = "file"
	// Dir is written: it must be a writable directory, or be creatable
	// under a writable parent.
	Dir Kind = "dir"
	// SQLite is a SQLite database, given as a file path or a DSN
	// ("file:./data/x.db?mode=rwc"). It is opened read-write and created on
	// first use, but its directory must already exist, and be writable for
	// the journal/WAL files.
	SQLite Kind = "sqlite"
	// OutputFile is appended to (a log file): writable if it exists, or
	// creatable under a writable parent.
	OutputFile Kind = "output_file"
)

// Source says where a path's value came from.
type Source string

const (
	SourceSet     Source = "set"     // the environment variable (or flag) was set
	SourceDefault Source = "default" // not set; the built-in default is in use
	SourceUnset   Source = "unset"   // not set and there is no default
)

// Status is the outcome of checking one path.
type Status string

const (
	StatusOK          Status = "ok"
	StatusUnset       Status = "unset"       // not configured
	StatusWillCreate  Status = "will_create" // absent, and the parent is writable
	StatusMissing     Status = "missing"     // absent, and cannot be created
	StatusNotReadable Status = "not_readable"
	StatusNotWritable Status = "not_writable"
	StatusWrongType   Status = "wrong_type" // a directory where a file is expected, or the reverse
	StatusInMemory    Status = "in_memory"  // an in-memory SQLite DSN; nothing on disk
)

// Entry is one configured path.
type Entry struct {
	// Name is how the path is set, usually its environment variable
	// ("PLUGINS_CONFIG_PATH") or a flag ("-env").
	Name string
	// Value is the value the binary uses, after defaults are applied.
	Value string
	// Default is the value used when Name is not set ("" when there is none).
	Default string
	// Kind decides what is checked.
	Kind Kind
	// Required marks a path the binary cannot work without in its current
	// mode. An unset required path is a problem; an unset optional one just
	// means the feature that uses it is off.
	Required bool
	// Use says what the path is for, in a few words.
	Use string
	// Source overrides the source derived from the environment. Set it for
	// values that do not come from an environment variable (flags).
	Source Source
}

// Result is the outcome of checking one Entry.
type Result struct {
	Entry
	Source   Source
	Path     string // the filesystem path checked (a DSN reduced to its file)
	Resolved string // Path made absolute against the working directory
	Status   Status
	Detail   string // why a check failed
}

// Problem reports whether the result should be flagged. A path that was set,
// or is required, must work. A default file that is absent is not a problem
// (a default .env is normally absent in containers), but a default directory
// or database the binary will write to must be usable.
func (r Result) Problem() bool {
	switch r.Status {
	case StatusOK, StatusWillCreate, StatusInMemory:
		return false
	case StatusUnset:
		return r.Required
	case StatusMissing:
		if r.Kind == File && r.Source == SourceDefault && !r.Required {
			return false
		}
		return true
	default:
		return true
	}
}

// Check examines every entry. It only reads the filesystem, except that a
// directory's writability is tested by creating and removing a temporary file
// in it.
func Check(entries []Entry) []Result {
	results := make([]Result, 0, len(entries))
	for _, e := range entries {
		results = append(results, checkOne(e))
	}
	return results
}

func checkOne(e Entry) Result {
	r := Result{Entry: e, Source: sourceOf(e)}

	path := e.Value
	if e.Kind == SQLite {
		p, onDisk := SQLitePath(e.Value)
		if !onDisk && strings.TrimSpace(e.Value) != "" {
			r.Status = StatusInMemory
			return r
		}
		path = p
	}
	if strings.TrimSpace(path) == "" {
		r.Status = StatusUnset
		return r
	}
	r.Path = path
	if abs, err := filepath.Abs(path); err == nil {
		r.Resolved = abs
	} else {
		r.Resolved = path
	}

	switch e.Kind {
	case File:
		r.Status, r.Detail = checkReadableFile(r.Resolved)
	case Dir:
		r.Status, r.Detail = checkWritableDir(r.Resolved)
	case SQLite:
		r.Status, r.Detail = checkSQLite(r.Resolved)
	case OutputFile:
		r.Status, r.Detail = checkOutputFile(r.Resolved)
	default:
		r.Status, r.Detail = StatusOK, ""
	}
	return r
}

func sourceOf(e Entry) Source {
	if e.Source != "" {
		return e.Source
	}
	if e.Name != "" {
		if v, ok := os.LookupEnv(e.Name); ok && strings.TrimSpace(v) != "" {
			return SourceSet
		}
	}
	if e.Default != "" {
		return SourceDefault
	}
	return SourceUnset
}

// SQLitePath reduces a SQLite DSN or path to the file it opens. onDisk is
// false for in-memory databases.
func SQLitePath(dsn string) (path string, onDisk bool) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return "", false
	}
	p := strings.TrimPrefix(dsn, "file:")
	p, query, _ := strings.Cut(p, "?")
	// file:///abs/path is the URI form of /abs/path.
	if strings.HasPrefix(p, "///") {
		p = p[2:]
	}
	if p == "" || p == ":memory:" || strings.Contains(strings.ToLower(query), "mode=memory") {
		return "", false
	}
	return p, true
}

func checkReadableFile(path string) (Status, string) {
	info, err := os.Stat(path)
	if err != nil {
		return statErrStatus(err)
	}
	if info.IsDir() {
		return StatusWrongType, "is a directory, expected a file"
	}
	f, err := os.Open(path)
	if err != nil {
		return StatusNotReadable, err.Error()
	}
	_ = f.Close()
	return StatusOK, ""
}

func checkWritableDir(path string) (Status, string) {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return StatusWrongType, "is a file, expected a directory"
		}
		if err := probeWrite(path); err != nil {
			return StatusNotWritable, err.Error()
		}
		return StatusOK, ""
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return statErrStatus(err)
	}
	return creatableUnder(path)
}

func checkSQLite(path string) (Status, string) {
	info, err := os.Stat(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return statErrStatus(err)
		}
		// SQLite creates the database file but not the directory it is in.
		dir := filepath.Dir(path)
		dirInfo, err := os.Stat(dir)
		switch {
		case err != nil:
			return StatusMissing, "does not exist, and neither does its directory " + dir + " (SQLite does not create directories)"
		case !dirInfo.IsDir():
			return StatusMissing, "does not exist; " + dir + " is not a directory"
		}
		if err := probeWrite(dir); err != nil {
			return StatusMissing, "does not exist and cannot be created: " + dir + " is not writable"
		}
		return StatusWillCreate, ""
	}
	if info.IsDir() {
		return StatusWrongType, "is a directory, expected a database file"
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return StatusNotWritable, err.Error()
	}
	_ = f.Close()
	// SQLite writes its journal or WAL next to the database.
	if err := probeWrite(filepath.Dir(path)); err != nil {
		return StatusNotWritable, "directory not writable (SQLite needs it for journal/WAL files): " + err.Error()
	}
	return StatusOK, ""
}

func checkOutputFile(path string) (Status, string) {
	info, err := os.Stat(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return statErrStatus(err)
		}
		return creatableUnder(path)
	}
	if info.IsDir() {
		return StatusWrongType, "is a directory, expected a file"
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return StatusNotWritable, err.Error()
	}
	_ = f.Close()
	return StatusOK, ""
}

// creatableUnder reports whether path, which does not exist, can be created:
// its nearest existing ancestor must be a writable directory (MkdirAll
// creates any missing levels in between).
func creatableUnder(path string) (Status, string) {
	parent := filepath.Dir(path)
	for {
		info, err := os.Stat(parent)
		if err == nil {
			if !info.IsDir() {
				return StatusMissing, "does not exist; " + parent + " is not a directory"
			}
			if err := probeWrite(parent); err != nil {
				return StatusMissing, "does not exist and cannot be created: " + parent + " is not writable"
			}
			return StatusWillCreate, ""
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return StatusMissing, "does not exist; " + err.Error()
		}
		next := filepath.Dir(parent)
		if next == parent {
			return StatusMissing, "does not exist"
		}
		parent = next
	}
}

// probeWrite tests that dir accepts new files, the way the binary will use
// it, by creating and removing a temporary file.
func probeWrite(dir string) error {
	f, err := os.CreateTemp(dir, ".startup-path-check-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

func statErrStatus(err error) (Status, string) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return StatusMissing, "does not exist"
	case errors.Is(err, fs.ErrPermission):
		return StatusNotReadable, err.Error()
	default:
		return StatusMissing, err.Error()
	}
}

// Log writes one "startup path" line per result, after a line giving the
// working directory and process user that relative paths and permissions
// depend on. Problems are logged at WARN, everything else at INFO. It returns
// the number of problems.
func Log(l zerolog.Logger, component string, results []Result) int {
	wd, err := os.Getwd()
	if err != nil {
		wd = "unknown (" + err.Error() + ")"
	}
	l.Info().
		Str("component", component).
		Str("working_dir", wd).
		Int("uid", os.Getuid()).
		Int("gid", os.Getgid()).
		Msg("startup path check: relative paths resolve against working_dir")

	problems := 0
	for _, r := range results {
		ev := l.Info()
		if r.Problem() {
			problems++
			ev = l.Warn()
		}
		ev = ev.Str("component", component).
			Str("name", r.Name).
			Str("status", string(r.Status)).
			Str("source", string(r.Source)).
			Str("kind", string(r.Kind))
		if r.Path != "" {
			ev = ev.Str("path", r.Path)
		}
		if r.Resolved != "" && r.Resolved != r.Path {
			ev = ev.Str("resolved", r.Resolved)
		}
		if r.Required {
			ev = ev.Bool("required", true)
		}
		if r.Detail != "" {
			ev = ev.Str("detail", r.Detail)
		}
		ev.Str("use", r.Use).Msg("startup path")
	}

	summary := l.Info()
	if problems > 0 {
		summary = l.Warn()
	}
	summary.
		Str("component", component).
		Int("checked", len(results)).
		Int("problems", problems).
		Msg("startup path check complete")
	return problems
}
