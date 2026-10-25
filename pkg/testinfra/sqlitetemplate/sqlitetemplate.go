// Package sqlitetemplate gives tests a migrated SQLite database without
// migrating it for every test. A Template is migrated once per test binary,
// into an in-memory database kept open for the life of the binary, and
// CopyInto copies it into a test's own database with SQLite's online backup
// API. Migrating Studio's schema (models.InitModels) costs ~300 ms under
// -race; copying it costs a few milliseconds.
//
// The package imports no Studio package, so models' own tests can use it.
// gorm keeps some state on the *gorm.DB handle rather than in the database
// (custom join models: models.RegisterJoinModels); a caller registers that on
// its handle after CopyInto.
package sqlitetemplate

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/mattn/go-sqlite3"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Template is a database migrated once, on first use, and copied from then on.
type Template struct {
	migrate func(*gorm.DB) error

	once sync.Once
	src  *sql.DB
	err  error
}

// New returns a Template that migrate prepares on first use.
func New(migrate func(*gorm.DB) error) *Template {
	return &Template{migrate: migrate}
}

var templateSeq atomic.Int64

func (tp *Template) source() (*sql.DB, error) {
	tp.once.Do(func() {
		name := fmt.Sprintf("sqlitetemplate_%d", templateSeq.Add(1))
		db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared&_busy_timeout=5000"), &gorm.Config{
			SkipDefaultTransaction: true,
		})
		if err != nil {
			tp.err = err
			return
		}
		sqlDB, err := db.DB()
		if err != nil {
			tp.err = err
			return
		}
		// One connection that is never closed: the in-memory database lives
		// as long as a connection to it does.
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetConnMaxLifetime(0)
		if err := tp.migrate(db); err != nil {
			tp.err = fmt.Errorf("migrate the template database: %w", err)
			return
		}
		tp.src = sqlDB
	})
	return tp.src, tp.err
}

// CopyInto replaces the main database of dst with a copy of the template.
//
// dst must keep its database on one connection (SetMaxOpenConns(1)): every
// connection to ":memory:" is a database of its own, so with a pool the copy
// would land in one connection's database and the others would stay empty.
func (tp *Template) CopyInto(dst *gorm.DB) error {
	src, err := tp.source()
	if err != nil {
		return err
	}
	dstDB, err := dst.DB()
	if err != nil {
		return err
	}
	return Copy(context.Background(), dstDB, src)
}

// Copy copies every page of src's main database into dst's with SQLite's
// online backup API.
func Copy(ctx context.Context, dst, src *sql.DB) error {
	dstConn, err := dst.Conn(ctx)
	if err != nil {
		return err
	}
	defer dstConn.Close()
	srcConn, err := src.Conn(ctx)
	if err != nil {
		return err
	}
	defer srcConn.Close()
	return dstConn.Raw(func(d any) error {
		return srcConn.Raw(func(s any) error {
			dc, ok := d.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("destination connection is a %T, not a *sqlite3.SQLiteConn", d)
			}
			sc, ok := s.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("source connection is a %T, not a *sqlite3.SQLiteConn", s)
			}
			b, err := dc.Backup("main", sc, "main")
			if err != nil {
				return err
			}
			if _, err := b.Step(-1); err != nil {
				_ = b.Finish()
				return err
			}
			return b.Finish()
		})
	})
}
