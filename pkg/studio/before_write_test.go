//go:build !enterprise

package studio

import (
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/licensing"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// setSchemaRecord rewrites the schema record as another build would have
// left it.
func setSchemaRecord(t *testing.T, db *gorm.DB, version, minReader int, writtenBy string) {
	t.Helper()
	require.NoError(t, db.Model(&models.StudioSchema{}).Where("1 = 1").
		Updates(map[string]interface{}{"version": version, "min_reader_version": minReader, "written_by": writtenBy}).Error)
}

// A full Studio started on a database that a newer build has made unreadable
// for it (the minimum reader version is above this build's schema version)
// refuses, as a headless control plane does, and changes nothing: no
// migration, no schema record, no row. It used to log one warning, migrate
// and serve.
func TestNewRefusesADatabaseItCannotRead_Postgres(t *testing.T) {
	forEachStudioDatabase(t, func(t *testing.T, opts Options) {
		s, err := New(opts)
		require.NoError(t, err)
		stopStudio(t, s)
		setSchemaRecord(t, opts.DB, models.SchemaVersion+3, models.SchemaVersion+2, "9.9.9")
		// A table of the newer build, to show nothing is altered or dropped.
		require.NoError(t, opts.DB.Exec("CREATE TABLE from_a_newer_build (id integer PRIMARY KEY, note text)").Error)
		require.NoError(t, opts.DB.Exec("INSERT INTO from_a_newer_build (id, note) VALUES (1, 'kept')").Error)
		before := databaseFingerprint(t, opts.DB)

		_, err = New(opts)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSchemaTooNew)
		for _, part := range []string{
			"schema version " + strconv.Itoa(models.SchemaVersion+3), "recorded by Studio 9.9.9",
			"needs schema version " + strconv.Itoa(models.SchemaVersion+2) + " or later",
			"this build has " + strconv.Itoa(models.SchemaVersion),
			"Nothing was changed", "restore the database backup taken before the upgrade",
		} {
			assert.Contains(t, err.Error(), part)
		}
		assert.Equal(t, before, databaseFingerprint(t, opts.DB), "the database is untouched")

		// The refusal leaves nothing running: a start that can read works.
		setSchemaRecord(t, opts.DB, models.SchemaVersion, models.MinReaderSchemaVersion, "test")
		s, err = New(opts)
		require.NoError(t, err, "a failed New released the instance slot")
		stopStudio(t, s)
	})
}

// The same refusal on the two databases an upgrade starts from, had a newer
// build left its mark on them: a 2.2-shaped one and one from before App
// keys were converted. Neither is migrated, and no key is rewritten.
func TestNewRefusesAnUnreadableDatabaseBeforeMigratingIt_Postgres(t *testing.T) {
	for name, seed := range map[string]func(t *testing.T, opts Options){
		"a 2.2-shaped database": func(t *testing.T, opts Options) {
			seed222Database(t, opts.DB)
			require.NoError(t, opts.DB.AutoMigrate(&models.StudioSchema{}))
			require.NoError(t, opts.DB.Create(&models.StudioSchema{ID: 1, Version: models.SchemaVersion + 1, MinReaderVersion: models.SchemaVersion + 1, WrittenBy: "9.9.9"}).Error)
		},
		"a database from before App keys were converted": func(t *testing.T, opts Options) {
			require.NoError(t, models.InitModels(opts.DB))
			require.NoError(t, opts.DB.Exec("DROP INDEX idx_credentials_secret_hash").Error)
			require.NoError(t, opts.DB.Exec("ALTER TABLE credentials DROP COLUMN secret_hash").Error)
			require.NoError(t, opts.DB.Create(&models.StudioSchema{ID: 1, Version: models.SchemaVersion + 1, MinReaderVersion: models.SchemaVersion + 1, WrittenBy: "9.9.9"}).Error)
			legacyKeys(t, opts.DB, 5)
		},
	} {
		t.Run(name, func(t *testing.T) {
			forEachStudioDatabase(t, func(t *testing.T, opts Options) {
				seed(t, opts)
				before := databaseFingerprint(t, opts.DB)
				_, err := New(opts)
				require.ErrorIs(t, err, ErrSchemaTooNew)
				assert.Equal(t, before, databaseFingerprint(t, opts.DB), "the database is untouched")
			})
		})
	}
}

// Newer but readable, equal and older databases start as before; the newer
// one keeps its record (with RecordSchemaVersion's warning).
func TestNewStartsOnDatabasesItCanRead_Postgres(t *testing.T) {
	forEachStudioDatabase(t, func(t *testing.T, opts Options) {
		s, err := New(opts) // no record yet: a new database
		require.NoError(t, err)
		stopStudio(t, s)

		for _, tc := range []struct {
			name               string
			version, minReader int
			wantVersion        int
		}{
			{"newer, this build still a reader", models.SchemaVersion + 3, models.SchemaVersion, models.SchemaVersion + 3},
			{"newer, an older minimum reader", models.SchemaVersion + 1, 1, models.SchemaVersion + 1},
			{"equal", models.SchemaVersion, models.MinReaderSchemaVersion, models.SchemaVersion},
			{"older", models.SchemaVersion - 1, 1, models.SchemaVersion},
		} {
			setSchemaRecord(t, opts.DB, tc.version, tc.minReader, "other")
			s, err := New(opts)
			require.NoError(t, err, tc.name)
			var row models.StudioSchema
			require.NoError(t, opts.DB.Take(&row).Error)
			assert.Equal(t, tc.wantVersion, row.Version, tc.name)
			stopStudio(t, s)
		}

		// A record table with no row (a start interrupted before the
		// record was written) is a database to migrate, not to refuse.
		require.NoError(t, opts.DB.Exec("DELETE FROM studio_schema").Error)
		s, err = New(opts)
		require.NoError(t, err)
		stopStudio(t, s)
	})
}

// The headless control plane's check is unchanged: it still refuses a
// database it cannot read, and one that no full Studio has migrated.
func TestCheckSchemaIsUnchangedByTheFullStudioCheck(t *testing.T) {
	opts := newTestOptions(t)
	assert.ErrorIs(t, CheckSchema(t.Context(), opts.DB), ErrSchemaMissing)
	assert.NoError(t, models.CheckSchemaReadable(t.Context(), opts.DB), "a full Studio may start on (and migrate) an empty database")
	s, err := New(opts)
	require.NoError(t, err)
	stopStudio(t, s)
	setSchemaRecord(t, opts.DB, models.SchemaVersion+1, models.SchemaVersion+1, "9.9.9")
	assert.ErrorIs(t, CheckSchema(t.Context(), opts.DB), ErrSchemaTooNew)
	setSchemaRecord(t, opts.DB, models.SchemaVersion-1, 1, "old")
	assert.ErrorIs(t, CheckSchema(t.Context(), opts.DB), ErrSchemaTooOld)
	assert.NoError(t, models.CheckSchemaReadable(t.Context(), opts.DB))
}

// refusedLicense is a licensing service whose license does not validate.
type refusedLicense struct {
	err     error
	started *atomic.Bool // Start was called: the background loops would run
}

func (r refusedLicense) Start() error      { r.started.Store(true); return r.err }
func (r refusedLicense) Revalidate() error { return r.err }
func (refusedLicense) Stop()               {}
func (refusedLicense) IsValid() bool       { return false }
func (refusedLicense) Entitlement(string) (licensing.Feature, bool) {
	return licensing.Feature{}, false
}
func (refusedLicense) DaysLeft() int                          { return 0 }
func (refusedLicense) TelemetryMiddleware() gin.HandlerFunc   { return func(*gin.Context) {} }
func (refusedLicense) SendTelemetry() error                   { return nil }
func (refusedLicense) GetLicenseInfo() *licensing.LicenseInfo { return nil }

// A Studio whose license is refused does not touch the database: the
// license is validated before the migrations, the schema record and the
// conversion of App keys, which an older version could not undo. Shown here
// with a licensing service that refuses (the Community build has none that
// can); the Enterprise build runs the same check with real refused licenses
// (enterprise/_coretests/pkg/studio).
func TestRefusedLicenseLeavesTheDatabaseUntouched_Postgres(t *testing.T) {
	refused := errors.New("license does not include the Tyk AI Studio SKU")
	prev := newLicensingService
	t.Cleanup(func() { newLicensingService = prev })

	for name, seed := range map[string]func(t *testing.T, opts Options) map[uint]string{
		"a 2.2 database": func(t *testing.T, opts Options) map[uint]string { return seed222Database(t, opts.DB) },
		"a database from before App keys were converted": func(t *testing.T, opts Options) map[uint]string {
			newLicensingService = prev
			s, err := New(opts)
			require.NoError(t, err)
			stopStudio(t, s)
			dropIndex := "DROP INDEX idx_credentials_secret_hash"
			require.NoError(t, opts.DB.Exec(dropIndex).Error)
			require.NoError(t, opts.DB.Exec("ALTER TABLE credentials DROP COLUMN secret_hash").Error)
			require.NoError(t, opts.DB.Exec("DELETE FROM credentials").Error)
			setSchemaRecord(t, opts.DB, 41, 26, "2.3.0-rc")
			return legacyKeys(t, opts.DB, 5)
		},
	} {
		t.Run(name, func(t *testing.T) {
			forEachStudioDatabase(t, func(t *testing.T, opts Options) {
				keys := seed(t, opts)
				before := databaseFingerprint(t, opts.DB)

				var started atomic.Bool
				newLicensingService = func(licensing.Config, *gorm.DB) licensing.Service {
					return refusedLicense{err: refused, started: &started}
				}
				_, err := New(opts)
				require.ErrorIs(t, err, refused)
				assert.False(t, started.Load(), "the license is only validated up front: its periodic checks and telemetry are not started")
				assert.Contains(t, err.Error(), "studio: start licensing: license validation failed at boot", "today's message")
				assert.Equal(t, before, databaseFingerprint(t, opts.DB), "a refused license leaves the database untouched")

				// With a license that validates the same database is
				// migrated as before.
				newLicensingService = prev
				s, err := New(opts)
				require.NoError(t, err)
				assertCredentialsAtRest(t, opts.DB, keys)
				assert.NotEqual(t, before, databaseFingerprint(t, opts.DB))
				stopStudio(t, s)
			})
		})
	}
}

// orderedLicense records when its license is validated and when its
// background work is started, against the state of the database.
type orderedLicense struct {
	refusedLicense
	db                                  *gorm.DB
	migratedAtValidate, migratedAtStart *bool
}

func (o orderedLicense) migrated() bool { return o.db.Migrator().HasTable(&models.StudioSchema{}) }
func (o orderedLicense) Revalidate() error {
	*o.migratedAtValidate = o.migrated()
	return nil
}
func (o orderedLicense) Start() error {
	*o.migratedAtStart = o.migrated()
	return nil
}
func (orderedLicense) IsValid() bool { return true }

// The license is validated before the database is migrated, and the
// service's periodic re-validation and telemetry, which read the database,
// are started only after: they must not run against tables in the middle of
// a migration.
func TestLicenseIsValidatedBeforeMigrationsAndStartedAfter(t *testing.T) {
	prev := newLicensingService
	t.Cleanup(func() { newLicensingService = prev })
	opts := newTestOptions(t)
	var atValidate, atStart bool
	newLicensingService = func(_ licensing.Config, db *gorm.DB) licensing.Service {
		return orderedLicense{db: db, migratedAtValidate: &atValidate, migratedAtStart: &atStart}
	}
	s, err := New(opts)
	require.NoError(t, err)
	defer stopStudio(t, s)
	assert.False(t, atValidate, "validated before anything is migrated")
	assert.True(t, atStart, "background work started after the migrations")
}

// lifecycleLicense counts what New does with the licensing service.
type lifecycleLicense struct {
	refusedLicense
	validated, started, stopped *atomic.Int32
}

func (l lifecycleLicense) Revalidate() error { l.validated.Add(1); return nil }
func (l lifecycleLicense) Start() error      { l.started.Add(1); return nil }
func (l lifecycleLicense) Stop()             { l.stopped.Add(1) }
func (lifecycleLicense) IsValid() bool       { return true }

// When New fails after licensing has started (here: a node label the
// cluster registry refuses, which is checked after the migrations), the
// service is stopped again: its periodic checks and telemetry do not
// outlive the failed start. And when New fails before it started (a refused
// license), it is stopped without ever having been started.
func TestLicensingIsStoppedWhenNewFails(t *testing.T) {
	prev := newLicensingService
	t.Cleanup(func() { newLicensingService = prev })
	var validated, started, stopped atomic.Int32
	newLicensingService = func(licensing.Config, *gorm.DB) licensing.Service {
		return lifecycleLicense{validated: &validated, started: &started, stopped: &stopped}
	}

	opts := newTestOptions(t)
	opts.NodeLabel = strings.Repeat("x", 65)
	_, err := New(opts)
	require.ErrorContains(t, err, "label")
	assert.Equal(t, int32(1), validated.Load())
	assert.Equal(t, int32(1), started.Load(), "the failure came after licensing started")
	assert.Equal(t, int32(1), stopped.Load(), "and it was stopped")

	// A start that succeeds, then Stop: started once, stopped once.
	validated.Store(0)
	started.Store(0)
	stopped.Store(0)
	s, err := New(newTestOptions(t))
	require.NoError(t, err)
	stopStudio(t, s)
	assert.Equal(t, int32(1), started.Load())
	assert.Equal(t, int32(1), stopped.Load())
}
