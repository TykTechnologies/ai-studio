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
	"github.com/TykTechnologies/midsommar/v2/pkg/studio"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
)

func init() {
	studio.RegisterDatabaseDriver("sqlite", sqlite.Open)
}
