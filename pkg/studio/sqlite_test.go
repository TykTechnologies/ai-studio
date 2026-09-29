package studio

import "github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"

// The tests open SQLite databases through OpenDatabase. pkg/studio/sqlitedb
// cannot be imported here (it imports this package), so register the driver
// the same way it does.
func init() {
	RegisterDatabaseDriver("sqlite", sqlite.Open)
}
