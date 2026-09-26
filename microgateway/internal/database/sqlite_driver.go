package database

import (
	"database/sql"
	"fmt"

	"github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// sqliteDriverName is mattn/go-sqlite3 with the gateway's per-connection
// settings, which the driver does not accept in the DSN.
const sqliteDriverName = "sqlite3_microgateway"

// SQLiteJournalSizeLimit caps the write-ahead log's file size once a
// checkpoint has copied it back into the database. Without it (-1, SQLite's
// default) the file keeps the size of its largest burst, which under load
// reached several gigabytes.
const SQLiteJournalSizeLimit = 64 << 20

func init() {
	sql.Register(sqliteDriverName, &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			_, err := conn.Exec(fmt.Sprintf("PRAGMA journal_size_limit = %d", SQLiteJournalSizeLimit), nil)
			return err
		},
	})
}

// openSQLite returns the dialector for a SQLite DSN on the gateway's driver.
func openSQLite(dsn string) gorm.Dialector {
	return sqlite.New(sqlite.Config{DriverName: sqliteDriverName, DSN: dsn})
}
