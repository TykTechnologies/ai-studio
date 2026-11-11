package database

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// A gateway whose volume is full keeps serving: requests are proxied from
// memory and from what the database already holds. What it can no longer do
// is store what is new, and until this file nothing said so apart from one
// error per dropped analytics row. The writes that matter after a restart
// (the configuration an edge starts from without a control plane, its last
// control contact, budget blocks, the budget sync's position) report their
// outcome here, so that the condition is logged once when it begins and
// once when it ends, shown by /ready and exported as metrics.
//
// The state is per process: a gateway has one database.

// IsStorageFull reports whether err is the database refusing a write for
// lack of room: SQLite's "database or disk is full" (SQLITE_FULL: the
// volume is full, or the database reached its page limit), or PostgreSQL's
// disk_full (SQLSTATE 53100) and "No space left on device".
func IsStorageFull(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database or disk is full") ||
		strings.Contains(msg, "sqlstate 53100") ||
		strings.Contains(msg, "no space left on device")
}

// The kinds of write that report here.
const (
	// WriteConfiguration: a configuration snapshot applied by an edge, with
	// the encrypted copy it starts from when no control plane answers.
	WriteConfiguration = "configuration"
	// WriteControlContact: the time of the edge's last contact with a
	// control plane, which bounds a start without one.
	WriteControlContact = "control_contact"
	// WriteBudgetBlocks: the Apps and LLMs the control plane said to refuse
	// on budget grounds, kept for a restart.
	WriteBudgetBlocks = "budget_blocks"
	// WriteBudgetSyncPosition: the sequence number of the last budget sync
	// applied.
	WriteBudgetSyncPosition = "budget_sync_position"
	// WriteAnalytics: the gateway's own analytics rows and the usage ledger
	// budgets are enforced from.
	WriteAnalytics = "analytics"
)

var storage struct {
	mu sync.Mutex
	// failing holds the kinds of write whose last attempt found no room,
	// with when each began to fail.
	failing map[string]time.Time
	// since is when the first of them began to fail.
	since time.Time
	// failures counts refused writes by kind, since the process started.
	failures map[string]*atomic.Uint64
}

// NoteWrite records the outcome of a write of the given kind ("configuration
// snapshot", "control contact", ...). A write refused for lack of room marks
// the kind as failing; a write of that kind that succeeds clears it. Other
// errors say nothing about room and change nothing. The first failure of a
// kind and its recovery are logged, once each; it returns err unchanged.
func NoteWrite(kind string, err error) error {
	full := IsStorageFull(err)
	if err != nil && !full {
		return err
	}
	storage.mu.Lock()
	if storage.failing == nil {
		storage.failing = map[string]time.Time{}
		storage.failures = map[string]*atomic.Uint64{}
	}
	counter := storage.failures[kind]
	if counter == nil {
		counter = &atomic.Uint64{}
		storage.failures[kind] = counter
	}
	began, wasFailing := storage.failing[kind]
	switch {
	case full && !wasFailing:
		now := time.Now()
		storage.failing[kind] = now
		if len(storage.failing) == 1 {
			storage.since = now
		}
	case !full && wasFailing:
		delete(storage.failing, kind)
		if len(storage.failing) == 0 {
			storage.since = time.Time{}
		}
	}
	storage.mu.Unlock()

	if full {
		counter.Add(1)
	}
	switch {
	case full && !wasFailing:
		log.Error().Err(err).Str("write", kind).
			Msg("The gateway's database has no room left (its volume is full, or the database reached its size limit): this is no longer being stored. The gateway keeps serving; free or add space. Reported once until it works again.")
	case !full && wasFailing:
		log.Info().Str("write", kind).Dur("failed_for", time.Since(began)).
			Msg("The gateway's database has room again: this is being stored again")
	}
	return err
}

// StorageStatus reports whether any kind of write is being refused for lack
// of room, since when, and which kinds (sorted).
func StorageStatus() (full bool, since time.Time, failing []string) {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	for kind := range storage.failing {
		failing = append(failing, kind)
	}
	sort.Strings(failing)
	return len(failing) > 0, storage.since, failing
}

// StorageFullFailures returns, per kind of write, how many were refused for
// lack of room since the process started.
func StorageFullFailures() map[string]uint64 {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	out := make(map[string]uint64, len(storage.failures))
	for kind, n := range storage.failures {
		out[kind] = n.Load()
	}
	return out
}

// resetStorageStatus forgets everything, for tests.
func resetStorageStatus() {
	storage.mu.Lock()
	storage.failing, storage.failures, storage.since = nil, nil, time.Time{}
	storage.mu.Unlock()
}
