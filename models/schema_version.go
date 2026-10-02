package models

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
)

// The schema version lets an instance that does not migrate (a headless
// control plane sharing a full Studio's database) check that the schema is
// one it understands.
//
// Rules for a change to the models:
//   - Every change to the schema (the goldens in testdata/schema) bumps
//     SchemaVersion; TestSchemaVersionMatchesGoldens and make schema-golden
//     fail until it is bumped.
//   - MinReaderSchemaVersion is raised, to the new SchemaVersion, only when
//     the change breaks code built for an older schema: a dropped or renamed
//     column or table, a changed column type, or a new NOT NULL column
//     without a default. Added tables, nullable or defaulted columns and
//     indexes leave it alone, so older readers keep working.
const (
	// 2: cluster_nodes.label and leader_eligible (nullable, additive).
	// 3: tyk_connections.host_key and its unique index (nullable, additive).
	// 4: endpoint_auth_plugins (new table, additive).
	// 5: proxy_logs and llm_chat_records on_behalf_of, acting_agent (nullable, additive).
	// 6: plugin_resource_types.default_access (nullable, additive).
	SchemaVersion          = 6
	MinReaderSchemaVersion = 1
)

// studioSchemaRowID is the one row of studio_schema.
const studioSchemaRowID = 1

// StudioSchema records the schema version the database was last migrated
// to by a full Studio, and the oldest schema version whose code can still
// read it.
type StudioSchema struct {
	ID               uint `gorm:"primaryKey;autoIncrement:false"`
	Version          int  `gorm:"not null"`
	MinReaderVersion int  `gorm:"not null"`
	// WrittenBy is the Studio version that recorded it.
	WrittenBy string
	UpdatedAt time.Time
}

// TableName keeps the table name singular: it holds one row.
func (StudioSchema) TableName() string { return "studio_schema" }

var (
	// ErrSchemaMissing: no schema version is recorded. No Studio has
	// migrated the database, or the one that did predates schema versions.
	ErrSchemaMissing = errors.New("no Studio schema version is recorded in the database")
	// ErrSchemaTooOld: the database was migrated by an older Studio than
	// this build needs.
	ErrSchemaTooOld = errors.New("the database schema is older than this build needs")
	// ErrSchemaTooNew: a newer Studio migrated the database in a way this
	// build cannot read.
	ErrSchemaTooNew = errors.New("the database schema is newer than this build can read")
)

// RecordSchemaVersion records that the database now has this build's
// schema. Call it after every migration has run, under the migration lock.
// It never lowers the record: an older Studio started against a database a
// newer one migrated leaves the newer version in place (its own migrations
// only add), and logs that it did.
func RecordSchemaVersion(db *gorm.DB, writtenBy string) error {
	var current StudioSchema
	err := db.Take(&current, studioSchemaRowID).Error
	switch {
	case err == nil && current.Version > SchemaVersion:
		logger.Warnf("The database schema is version %d (recorded by Studio %s), newer than this build's %d; keeping the newer record", current.Version, current.WrittenBy, SchemaVersion)
		return nil
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		return fmt.Errorf("read schema version: %w", err)
	}

	row := StudioSchema{
		ID:               studioSchemaRowID,
		Version:          SchemaVersion,
		MinReaderVersion: MinReaderSchemaVersion,
		WrittenBy:        writtenBy,
		UpdatedAt:        time.Now(),
	}
	// Conditional in the write too, in case another instance recorded a
	// newer version since the read.
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"version", "min_reader_version", "written_by", "updated_at"}),
		Where: clause.Where{Exprs: []clause.Expression{
			clause.Lte{Column: clause.Column{Table: "studio_schema", Name: "version"}, Value: SchemaVersion},
		}},
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}
	return nil
}

// CheckSchemaVersion reports whether this build can use the database's
// schema without migrating it, and returns the record. It only reads: it
// creates nothing. The errors wrap ErrSchemaMissing, ErrSchemaTooOld or
// ErrSchemaTooNew.
func CheckSchemaVersion(ctx context.Context, db *gorm.DB) (StudioSchema, error) {
	db = db.WithContext(ctx)
	var row StudioSchema
	if err := ctx.Err(); err != nil {
		return row, err
	}
	if !db.Migrator().HasTable(&StudioSchema{}) {
		if err := ctx.Err(); err != nil {
			return row, err
		}
		return row, fmt.Errorf("%w: start a full Studio (schema version %d or later) against the database first", ErrSchemaMissing, SchemaVersion)
	}
	if err := db.Take(&row, studioSchemaRowID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, fmt.Errorf("%w: start a full Studio (schema version %d or later) against the database first", ErrSchemaMissing, SchemaVersion)
		}
		return row, fmt.Errorf("read schema version: %w", err)
	}
	if row.Version < SchemaVersion {
		return row, fmt.Errorf("%w: the database is at schema version %d (recorded by Studio %s), this build needs %d; upgrade the full Studio first",
			ErrSchemaTooOld, row.Version, row.WrittenBy, SchemaVersion)
	}
	if row.MinReaderVersion > SchemaVersion {
		return row, fmt.Errorf("%w: the database is at schema version %d (recorded by Studio %s), which needs schema version %d or later to read; this build has %d",
			ErrSchemaTooNew, row.Version, row.WrittenBy, row.MinReaderVersion, SchemaVersion)
	}
	return row, nil
}
