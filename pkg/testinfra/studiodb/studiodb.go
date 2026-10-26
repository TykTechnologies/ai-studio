// Package studiodb gives a test's SQLite database Studio's schema without
// migrating it again. Migrate is the test-side replacement for
// models.InitModels on a fresh SQLite database: it copies a template that
// models.InitModels migrated once per test binary (pkg/testinfra/sqlitetemplate),
// which takes milliseconds where a migration takes ~300 ms under -race.
//
// Tests that test migration itself (schema goldens, upgrade paths, a database
// that already holds tables) still call models.InitModels.
package studiodb

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/sqlitetemplate"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

var schema = sqlitetemplate.New(models.InitModels)

// Migrate replaces db's contents with Studio's freshly migrated schema and
// registers on db's handle what models.InitModels registers there (the join
// models). db must be a SQLite database whose connection pool keeps it on one
// connection, as an in-memory one opened for a test does in practice.
func Migrate(db *gorm.DB) error {
	if err := schema.CopyInto(db); err != nil {
		return err
	}
	return models.RegisterJoinModels(db)
}
