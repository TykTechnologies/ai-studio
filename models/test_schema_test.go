package models

import (
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/sqlitetemplate"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

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
