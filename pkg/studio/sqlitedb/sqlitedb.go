// Package sqlitedb adds SQLite to studio.OpenDatabase. Import it for its side
// effect:
//
//	import _ "github.com/TykTechnologies/midsommar/v2/pkg/studio/sqlitedb"
//
// The SQLite driver (mattn/go-sqlite3) needs cgo, which is why pkg/studio
// leaves it out: a host that builds with CGO_ENABLED=0 and runs Studio on
// Postgres never links it. The standalone binary imports this package.
package sqlitedb

import (
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/sqlitedsn"
	"github.com/TykTechnologies/midsommar/v2/pkg/studio"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

func init() {
	studio.RegisterDatabaseDriver("sqlite", Open)
}

// Open is the dialector Studio opens SQLite with. A file database gets the
// microgateway's concurrency defaults unless DATABASE_URL sets them: WAL,
// _busy_timeout=5000, _synchronous=NORMAL and _txlock=immediate, and
// cache=shared is dropped (see sqlitedsn.Normalize). Without them a create
// whose hooks read before writing (the LLM and tool slug checks) fails with
// "database is locked" whenever a background writer commits in between. An
// in-memory DSN is used as is.
func Open(dsn string) gorm.Dialector {
	tuned, removedShared := sqlitedsn.Normalize(dsn)
	if removedShared {
		logger.Warn("SQLite: ignoring cache=shared in DATABASE_URL for a file database; it causes table-level lock errors under concurrent load")
	}
	return sqlite.Open(tuned)
}
