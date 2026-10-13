// Package sqlitedsn tunes SQLite DSNs (mattn/go-sqlite3 parameter names) for
// concurrent use. AI Studio (pkg/studio/sqlitedb) and the microgateway open
// file databases through it. It is plain string handling, so importing it
// does not link the cgo driver.
package sqlitedsn

import (
	"net/url"
	"strings"
)

// fileDefaults are applied to a file-backed SQLite DSN unless the DSN
// already sets them:
//
//   - WAL lets readers proceed while a write is in progress; in the default
//     rollback journal every write blocks them.
//   - busy_timeout makes a connection wait for a lock instead of failing at
//     once with SQLITE_BUSY.
//   - synchronous=NORMAL is the recommended pairing with WAL: durable across
//     application crashes, and it skips an fsync per commit.
//   - txlock=immediate takes the write lock when a transaction begins. A
//     deferred transaction that reads and then writes (a create whose hooks
//     check uniqueness first, for one) can otherwise fail to upgrade its lock
//     with SQLITE_BUSY or SQLITE_BUSY_SNAPSHOT, which busy_timeout does not
//     retry.
var fileDefaults = [][2]string{
	{"_journal_mode", "WAL"},
	{"_busy_timeout", "5000"},
	{"_synchronous", "NORMAL"},
	{"_txlock", "immediate"},
}

// paramAliases maps each default to the other spellings the driver accepts,
// so a DSN that already sets one is left alone.
var paramAliases = map[string][]string{
	"_journal_mode": {"_journal_mode", "_journal"},
	"_busy_timeout": {"_busy_timeout", "_timeout"},
	"_synchronous":  {"_synchronous", "_sync"},
	"_txlock":       {"_txlock"},
}

// Normalize tunes a file-backed SQLite DSN for concurrent use: it adds the
// defaults above unless the DSN sets them. It reports whether it removed
// cache=shared.
//
// Shared-cache mode is dropped for file databases. It makes SQLite lock whole
// tables across connections and report contention as SQLITE_LOCKED ("database
// table is locked"), which busy_timeout does not retry. Shared cache only
// helps in-memory databases, where it lets several connections see one
// database; those DSNs are returned unchanged.
func Normalize(dsn string) (string, bool) {
	if IsInMemory(dsn) {
		return dsn, false
	}
	path, rawQuery, _ := strings.Cut(dsn, "?")
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return dsn, false
	}

	removedShared := false
	if strings.EqualFold(q.Get("cache"), "shared") {
		q.Del("cache")
		removedShared = true
	}
	for _, kv := range fileDefaults {
		set := false
		for _, alias := range paramAliases[kv[0]] {
			if q.Has(alias) {
				set = true
				break
			}
		}
		if !set {
			q.Set(kv[0], kv[1])
		}
	}
	return path + "?" + q.Encode(), removedShared
}

// FilePath returns the database file a SQLite DSN opens, or false for an
// in-memory database.
func FilePath(dsn string) (string, bool) {
	if IsInMemory(dsn) {
		return "", false
	}
	path, _, _ := strings.Cut(dsn, "?")
	path = strings.TrimPrefix(path, "file:")
	if path == "" {
		return "", false
	}
	return path, true
}

// IsInMemory reports whether dsn opens an in-memory database.
func IsInMemory(dsn string) bool {
	lower := strings.ToLower(dsn)
	return strings.Contains(lower, ":memory:") || strings.Contains(lower, "mode=memory")
}
