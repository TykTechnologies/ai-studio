package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openSchemaVersionDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "schema.db")),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

func setRecordedSchema(t *testing.T, db *gorm.DB, version, minReader int) {
	t.Helper()
	require.NoError(t, db.Save(&StudioSchema{ID: studioSchemaRowID, Version: version, MinReaderVersion: minReader, WrittenBy: "test"}).Error)
}

func TestCheckSchemaVersion(t *testing.T) {
	ctx := context.Background()

	t.Run("no table", func(t *testing.T) {
		db := openSchemaVersionDB(t)
		_, err := CheckSchemaVersion(ctx, db)
		assert.ErrorIs(t, err, ErrSchemaMissing)
		// Read-only: checking must not create the table.
		assert.False(t, db.Migrator().HasTable(&StudioSchema{}))
	})

	t.Run("table without row", func(t *testing.T) {
		db := openSchemaVersionDB(t)
		require.NoError(t, db.AutoMigrate(&StudioSchema{}))
		_, err := CheckSchemaVersion(ctx, db)
		assert.ErrorIs(t, err, ErrSchemaMissing)
	})

	t.Run("recorded by this build", func(t *testing.T) {
		db := openSchemaVersionDB(t)
		require.NoError(t, InitModels(db))
		require.NoError(t, RecordSchemaVersion(db, "v-test"))
		got, err := CheckSchemaVersion(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, SchemaVersion, got.Version)
		assert.Equal(t, MinReaderSchemaVersion, got.MinReaderVersion)
		assert.Equal(t, "v-test", got.WrittenBy)
	})

	t.Run("older than this build", func(t *testing.T) {
		db := openSchemaVersionDB(t)
		require.NoError(t, db.AutoMigrate(&StudioSchema{}))
		setRecordedSchema(t, db, SchemaVersion-1, 0)
		_, err := CheckSchemaVersion(ctx, db)
		assert.ErrorIs(t, err, ErrSchemaTooOld)
		assert.ErrorContains(t, err, strconv.Itoa(SchemaVersion-1))
	})

	t.Run("newer but still readable", func(t *testing.T) {
		db := openSchemaVersionDB(t)
		require.NoError(t, db.AutoMigrate(&StudioSchema{}))
		setRecordedSchema(t, db, SchemaVersion+3, SchemaVersion)
		got, err := CheckSchemaVersion(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, SchemaVersion+3, got.Version)
	})

	t.Run("newer and not readable", func(t *testing.T) {
		db := openSchemaVersionDB(t)
		require.NoError(t, db.AutoMigrate(&StudioSchema{}))
		setRecordedSchema(t, db, SchemaVersion+3, SchemaVersion+1)
		_, err := CheckSchemaVersion(ctx, db)
		assert.ErrorIs(t, err, ErrSchemaTooNew)
		assert.ErrorContains(t, err, strconv.Itoa(SchemaVersion+1))
	})

	t.Run("cancelled context", func(t *testing.T) {
		db := openSchemaVersionDB(t)
		require.NoError(t, InitModels(db))
		require.NoError(t, RecordSchemaVersion(db, "v-test"))
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		_, err := CheckSchemaVersion(cctx, db)
		assert.Error(t, err)
		assert.False(t, errors.Is(err, ErrSchemaMissing))
	})
}

// A build must be able to read the schema it writes.
func TestMinReaderSchemaVersionNotAboveSchemaVersion(t *testing.T) {
	assert.LessOrEqual(t, MinReaderSchemaVersion, SchemaVersion)
	assert.GreaterOrEqual(t, MinReaderSchemaVersion, 1)
}

func TestInitModelsCreatesStudioSchemaTable(t *testing.T) {
	db := openSchemaVersionDB(t)
	require.NoError(t, InitModels(db))
	assert.True(t, db.Migrator().HasTable(&StudioSchema{}))
}

func TestRecordSchemaVersion(t *testing.T) {
	t.Run("writes and refreshes", func(t *testing.T) {
		db := openSchemaVersionDB(t)
		require.NoError(t, InitModels(db))
		require.NoError(t, RecordSchemaVersion(db, "v1"))
		require.NoError(t, RecordSchemaVersion(db, "v2"))
		var rows []StudioSchema
		require.NoError(t, db.Find(&rows).Error)
		require.Len(t, rows, 1)
		assert.Equal(t, "v2", rows[0].WrittenBy)
		assert.Equal(t, SchemaVersion, rows[0].Version)
	})

	t.Run("raises an older record", func(t *testing.T) {
		db := openSchemaVersionDB(t)
		require.NoError(t, InitModels(db))
		setRecordedSchema(t, db, SchemaVersion-1, 0)
		require.NoError(t, RecordSchemaVersion(db, "v-new"))
		var row StudioSchema
		require.NoError(t, db.Take(&row, studioSchemaRowID).Error)
		assert.Equal(t, SchemaVersion, row.Version)
		assert.Equal(t, MinReaderSchemaVersion, row.MinReaderVersion)
		assert.Equal(t, "v-new", row.WrittenBy)
	})

	// An older full Studio started against a database a newer one migrated
	// must not lower the record: the schema is still the newer one.
	t.Run("never lowers a newer record", func(t *testing.T) {
		db := openSchemaVersionDB(t)
		require.NoError(t, InitModels(db))
		setRecordedSchema(t, db, SchemaVersion+2, SchemaVersion+1)
		require.NoError(t, RecordSchemaVersion(db, "v-old"))
		var row StudioSchema
		require.NoError(t, db.Take(&row, studioSchemaRowID).Error)
		assert.Equal(t, SchemaVersion+2, row.Version)
		assert.Equal(t, SchemaVersion+1, row.MinReaderVersion)
		assert.Equal(t, "test", row.WrittenBy)
	})
}

// A host gives Studio a schema of its own (DATABASE_SCHEMA): the check must
// look there, not in public.
func TestCheckSchemaVersion_PostgreSQLSchema(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(postgres.Open(dsn), cfg)
	require.NoError(t, err)
	schema := fmt.Sprintf("schema_version_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		if sqlDB, err := admin.DB(); err == nil {
			sqlDB.Close()
		}
	})
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := gorm.Open(postgres.Open(dsn+sep+"search_path="+schema), cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	ctx := context.Background()
	_, err = CheckSchemaVersion(ctx, db)
	require.ErrorIs(t, err, ErrSchemaMissing)

	require.NoError(t, db.AutoMigrate(&StudioSchema{}))
	require.NoError(t, RecordSchemaVersion(db, "v-pg"))
	got, err := CheckSchemaVersion(ctx, db)
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion, got.Version)

	var inSchema int64
	require.NoError(t, admin.Raw("SELECT count(*) FROM "+schema+".studio_schema").Scan(&inSchema).Error)
	assert.Equal(t, int64(1), inSchema)

	setRecordedSchema(t, db, SchemaVersion+1, SchemaVersion+1)
	_, err = CheckSchemaVersion(ctx, db)
	assert.ErrorIs(t, err, ErrSchemaTooNew)
}

// schemaVersionFile records the schema version the goldens were generated
// for and a hash of them, so a change to the goldens without a version bump
// fails here (and in make schema-golden).
var schemaVersionFile = filepath.Join("testdata", "schema", "VERSION")

func goldenSchemaHash(t *testing.T) string {
	t.Helper()
	h := sha256.New()
	for _, name := range []string{"sqlite.golden", "postgres.golden"} {
		b, err := os.ReadFile(filepath.Join("testdata", "schema", name))
		require.NoError(t, err)
		fmt.Fprintf(h, "%s\n%d\n", name, len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestSchemaVersionMatchesGoldens(t *testing.T) {
	hash := goldenSchemaHash(t)
	recordedVersion, recordedHash := 0, ""
	if b, err := os.ReadFile(schemaVersionFile); err == nil {
		fields := strings.Fields(string(b))
		require.Len(t, fields, 2, "%s must hold \"<version> <sha256>\"", schemaVersionFile)
		recordedVersion, err = strconv.Atoi(fields[0])
		require.NoError(t, err)
		recordedHash = fields[1]
	} else if !os.IsNotExist(err) {
		require.NoError(t, err)
	}

	if os.Getenv("UPDATE_SCHEMA_VERSION") != "" {
		if hash != recordedHash && recordedVersion == SchemaVersion {
			t.Fatalf("schema goldens changed: bump models.SchemaVersion (and MinReaderSchemaVersion if the change breaks older readers), then run make schema-golden")
		}
		require.NoError(t, os.WriteFile(schemaVersionFile, []byte(fmt.Sprintf("%d %s\n", SchemaVersion, hash)), 0o644))
		return
	}

	if hash != recordedHash {
		t.Fatalf("schema goldens changed: bump models.SchemaVersion (and MinReaderSchemaVersion if the change breaks older readers), then run make schema-golden")
	}
	if recordedVersion != SchemaVersion {
		t.Fatalf("%s records schema version %d but models.SchemaVersion is %d: run make schema-golden", schemaVersionFile, recordedVersion, SchemaVersion)
	}
}
