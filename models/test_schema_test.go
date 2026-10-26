package models

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/sqlitetemplate"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

var privateDBSeq atomic.Int64

// privateMemoryDSN names a shared-cache in-memory database private to one run
// of the test: a fixed name (or "file::memory:?cache=shared", which is one
// database for the whole process) keeps the rows of an earlier run (go test
// -count=2) while any connection to it is open. Close the database in
// t.Cleanup (openPrivateMemoryDB does) so it goes when the test does.
func privateMemoryDSN(t *testing.T) string {
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	return fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", name, privateDBSeq.Add(1))
}

// openPrivateMemoryDB opens privateMemoryDSN(t) and closes it when t ends.
func openPrivateMemoryDB(t *testing.T, cfg *gorm.Config) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(privateMemoryDSN(t)), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// testSchema is Studio's schema, migrated by InitModels once per test binary.
var testSchema = sqlitetemplate.New(InitModels)

// migrateTestSchema gives a test's fresh SQLite database Studio's schema by
// copying testSchema, where InitModels would cost ~300 ms a test under
// -race; pkg/testinfra/studiodb does the same for other packages, which
// models cannot import. Tests of migration itself (schema goldens, schema
// versions, upgrade paths) still call InitModels.
func migrateTestSchema(db *gorm.DB) error {
	if err := testSchema.CopyInto(db); err != nil {
		return err
	}
	return RegisterJoinModels(db)
}
