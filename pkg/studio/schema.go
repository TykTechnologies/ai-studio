package studio

import (
	"context"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// The errors CheckSchema wraps.
var (
	// ErrSchemaMissing: no Studio schema version is recorded. No Studio
	// has migrated the database, or the one that did predates schema
	// versions.
	ErrSchemaMissing = models.ErrSchemaMissing
	// ErrSchemaTooOld: the database was migrated by an older Studio than
	// this build needs; upgrade the Studio that migrates it first.
	ErrSchemaTooOld = models.ErrSchemaTooOld
	// ErrSchemaTooNew: a newer Studio migrated the database in a way this
	// build cannot read; upgrade this build.
	ErrSchemaTooNew = models.ErrSchemaTooNew
)

// CheckSchema reports whether this build of Studio can use db, as a full
// Studio (New) left it, without migrating it. It only reads: it creates and
// alters nothing, so it is safe against a database another process owns.
// New records the schema version after its migrations; a newer Studio's
// schema is accepted as long as it still declares this build a reader (see
// models.SchemaVersion). The error wraps ErrSchemaMissing, ErrSchemaTooOld
// or ErrSchemaTooNew, or is the database's own.
func CheckSchema(ctx context.Context, db *gorm.DB) error {
	if _, err := models.CheckSchemaVersion(ctx, db); err != nil {
		return fmt.Errorf("studio: %w", err)
	}
	return nil
}

// schemaWriter is the Studio version recorded with the schema version.
func schemaWriter(version string) string {
	if version == "" {
		return "unknown"
	}
	return version
}
