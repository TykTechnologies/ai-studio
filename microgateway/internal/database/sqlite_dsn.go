package database

import (
	"net/url"
	"strings"
)

// sqliteFileDefaults are applied to a file-backed SQLite DSN unless the DSN
// already sets them (mattn/go-sqlite3 parameter names):
//
//   - WAL lets readers proceed while a write is in progress. The gateway reads
//     configuration on every request while analytics and budget writes run
//     alongside; in the default rollback journal every write blocks them.
//   - busy_timeout makes a connection wait for a lock instead of failing at
//     once with SQLITE_BUSY.
//   - synchronous=NORMAL is the recommended pairing with WAL: durable across
//     application crashes, and it skips an fsync per commit.
//   - txlock=immediate takes the write lock when a transaction begins. A
//     deferred transaction that reads and then writes can otherwise fail to
//     upgrade its lock with SQLITE_BUSY, which busy_timeout does not retry.
var sqliteFileDefaults = [][2]string{
	{"_journal_mode", "WAL"},
	{"_busy_timeout", "5000"},
	{"_synchronous", "NORMAL"},
	{"_txlock", "immediate"},
}

// sqliteParamAliases maps each default to the other spellings the driver
// accepts, so a DSN that already sets one is left alone.
var sqliteParamAliases = map[string][]string{
	"_journal_mode": {"_journal_mode", "_journal"},
	"_busy_timeout": {"_busy_timeout", "_timeout"},
	"_synchronous":  {"_synchronous", "_sync"},
	"_txlock":       {"_txlock"},
}

// normalizeSQLiteDSN tunes a file-backed SQLite DSN for concurrent use. It
// reports whether it removed cache=shared.
//
// Shared-cache mode is dropped for file databases. It makes SQLite lock whole
// tables across connections and report contention as SQLITE_LOCKED ("database
// table is locked"), which busy_timeout does not retry. Under load that
// failed the gateway's per-request lookups, and the failures reached clients
// as authentication and budget errors. Shared cache only helps in-memory
// databases, where it lets several connections see one database; those DSNs
// are returned unchanged.
func normalizeSQLiteDSN(dsn string) (string, bool) {
	if isInMemorySQLite(dsn) {
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
	for _, kv := range sqliteFileDefaults {
		set := false
		for _, alias := range sqliteParamAliases[kv[0]] {
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

func isInMemorySQLite(dsn string) bool {
	lower := strings.ToLower(dsn)
	return strings.Contains(lower, ":memory:") || strings.Contains(lower, "mode=memory")
}
