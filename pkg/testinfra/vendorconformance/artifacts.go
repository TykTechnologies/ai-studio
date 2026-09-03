package vendorconformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Recorder collects drift and failure artifacts across a whole run and writes
// them out once, at the end. It is safe for concurrent use: cases run in
// parallel.
//
// Drift is recorded rather than asserted because a vendor adding a field is not
// itself a bug — it is a heads-up that our schema is now incomplete. Collecting
// it into one reviewable report is what turns "a vendor changed something" from
// a production surprise into a release-checklist item.
type Recorder struct {
	dir string

	mu     sync.Mutex
	drift  map[string]map[string]string // pointer -> kind, keyed by "vendor/surface/model"
	failed []failureArtifact
}

type failureArtifact struct {
	Case    string
	Reason  string
	Payload []byte
}

// NewRecorder returns a Recorder writing under dir. A blank dir disables all
// artifact writing, which keeps unit tests from littering the tree.
func NewRecorder(dir string) *Recorder {
	return &Recorder{dir: dir, drift: map[string]map[string]string{}}
}

// RecordDrift files undocumented fields under a scope key.
func (r *Recorder) RecordDrift(scope string, drift []Drift) {
	if r == nil || len(drift) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.drift[scope]
	if !ok {
		m = map[string]string{}
		r.drift[scope] = m
	}
	for _, d := range drift {
		m[d.Pointer] = d.Kind
	}
}

// RecordFailure saves the payload that failed, so a schema violation can be
// inspected without re-running (and re-paying for) the request.
func (r *Recorder) RecordFailure(caseName, reason string, payload []byte) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed = append(r.failed, failureArtifact{Case: caseName, Reason: reason, Payload: payload})
}

// DriftCount returns the number of distinct undocumented fields seen.
func (r *Recorder) DriftCount() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, m := range r.drift {
		n += len(m)
	}
	return n
}

// Flush writes the drift summary and any failure payloads to disk and returns a
// one-line summary for the test log.
func (r *Recorder) Flush() (string, error) {
	if r == nil || r.dir == "" {
		return "", nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.drift) == 0 && len(r.failed) == 0 {
		return "no drift, no failure payloads", nil
	}
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return "", fmt.Errorf("creating artifact dir: %w", err)
	}
	// Drop the previous run's payloads. Stale artifacts from a differently-
	// filtered run are worse than none: they look like current findings and
	// send people chasing failures that no longer happen.
	if err := os.RemoveAll(filepath.Join(r.dir, "failures")); err != nil {
		return "", fmt.Errorf("clearing stale failure payloads: %w", err)
	}

	for _, f := range r.failed {
		name := sanitize(f.Case) + ".json"
		path := filepath.Join(r.dir, "failures", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		body := f.Payload
		if len(body) == 0 {
			body = []byte("(empty response body)")
		}
		header := fmt.Sprintf("// case:   %s\n// reason: %s\n", f.Case, strings.ReplaceAll(f.Reason, "\n", "\n//         "))
		if err := os.WriteFile(path, append([]byte(header), body...), 0o644); err != nil {
			return "", err
		}
	}

	if len(r.drift) > 0 {
		if err := r.writeDriftSummary(); err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("%d undocumented field(s) across %d scope(s), %d failure payload(s) -> %s",
		r.driftCountLocked(), len(r.drift), len(r.failed), r.dir), nil
}

func (r *Recorder) driftCountLocked() int {
	n := 0
	for _, m := range r.drift {
		n += len(m)
	}
	return n
}

func (r *Recorder) writeDriftSummary() error {
	scopes := make([]string, 0, len(r.drift))
	for k := range r.drift {
		scopes = append(scopes, k)
	}
	sort.Strings(scopes)

	var md strings.Builder
	md.WriteString("# Upstream schema drift\n\n")
	md.WriteString("Fields present in a response but absent from our canonical schema in\n")
	md.WriteString("`pkg/testinfra/vendorconformance/schema/`.\n\n")
	md.WriteString("Drift is not a failure. It is the early-warning signal: a vendor added a\n")
	md.WriteString("field, and our schema is now incomplete. Triage each entry, then either add\n")
	md.WriteString("it to the schema (if we should carry it through) or note why we drop it.\n")
	md.WriteString("Once this file is empty, set `VENDOR_TESTS_STRICT_UNKNOWN_FIELDS=true` to\n")
	md.WriteString("keep it that way.\n\n")

	structured := map[string]map[string]string{}
	for _, scope := range scopes {
		fields := r.drift[scope]
		structured[scope] = fields

		pointers := make([]string, 0, len(fields))
		for p := range fields {
			pointers = append(pointers, p)
		}
		sort.Strings(pointers)

		fmt.Fprintf(&md, "## %s\n\n", scope)
		for _, p := range pointers {
			fmt.Fprintf(&md, "- `%s` (%s)\n", p, fields[p])
		}
		md.WriteString("\n")
	}

	if err := os.WriteFile(filepath.Join(r.dir, "drift-summary.md"), []byte(md.String()), 0o644); err != nil {
		return err
	}
	data, err := json.MarshalIndent(structured, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.dir, "drift.json"), data, 0o644)
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, s)
}
