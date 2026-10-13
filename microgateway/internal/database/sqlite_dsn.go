package database

import "github.com/TykTechnologies/midsommar/v2/pkg/sqlitedsn"

// normalizeSQLiteDSN tunes a file-backed SQLite DSN for concurrent use (WAL,
// busy_timeout, synchronous=NORMAL, txlock=immediate, no shared cache) and
// reports whether it removed cache=shared. The gateway reads configuration on
// every request while analytics and budget writes run alongside; shared-cache
// table locks once reached clients as authentication and budget errors. The
// rules are shared with AI Studio's SQLite driver: see sqlitedsn.Normalize.
func normalizeSQLiteDSN(dsn string) (string, bool) {
	return sqlitedsn.Normalize(dsn)
}

// SQLiteFilePath returns the database file a SQLite DSN opens, or false for an
// in-memory database.
func SQLiteFilePath(dsn string) (string, bool) {
	return sqlitedsn.FilePath(dsn)
}

func isInMemorySQLite(dsn string) bool {
	return sqlitedsn.IsInMemory(dsn)
}
