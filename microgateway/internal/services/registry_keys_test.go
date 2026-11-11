package services

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// derivedHandles are the handles of db that code may hold instead of the
// one the edge was opened with: sessions, and a transaction.
func derivedHandles(t *testing.T, db *gorm.DB, use func(name string, h *gorm.DB)) {
	t.Helper()
	use("WithContext", db.WithContext(context.Background()))
	use("Session", db.Session(&gorm.Session{}))
	use("Debug session", db.Session(&gorm.Session{NewDB: true}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		use("Transaction", tx)
		return nil
	}))
}

// The reason for dbKey: gorm copies the Config for every session, so the
// *gorm.Config these registries were keyed by is not the same for a handle
// and a session derived from it, while the connection pool is, also inside
// a transaction.
func TestDBKeyIsSharedByEveryHandleOfADatabase(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		session := db.WithContext(context.Background())
		require.NotSame(t, db.Config, session.Config, "gorm copies the Config for a session: it cannot be the key")

		derivedHandles(t, db, func(name string, h *gorm.DB) {
			assert.Equal(t, dbKey(db), dbKey(h), name)
		})
	})
	t.Run("two databases have two keys", func(t *testing.T) {
		assert.NotEqual(t, dbKey(setupEdgeSyncTestDB(t)), dbKey(setupEdgeSyncTestDB(t)))
	})
}

// Whatever handle of the database a caller holds, the applied snapshot is
// stored, the control contact is recorded and read, the configuration the
// edge reports is found, and a snapshot being restored is left as it is.
// With the registries keyed by the handle's Config, each of these silently
// did nothing through a session-derived handle: the edge would then have
// had nothing to start from without a control plane, with no error.
func TestRegistriesAreFoundThroughEveryHandle(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		require.NoError(t, PersistAppliedSnapshots(db, snapshotTestKey))
		at := time.Now().UTC().Truncate(time.Second)

		n := 0
		derivedHandles(t, db, func(name string, h *gorm.DB) {
			n++
			// The applied snapshot, through a sync service built on h.
			checksum := strings.Repeat(string(rune('a'+n)), 64)
			if name == "Transaction" {
				// A sync runs its own transaction; inside another one it
				// is the registry lookups that are checked here.
				sealer, ok := sealerOf(h, "test")
				require.True(t, ok, name)
				require.NotNil(t, sealer)
			} else {
				svc := NewEdgeSyncService(h, "ns")
				_, err := svc.SyncConfigurationReport(richSnapshot(at.Add(time.Duration(n)*time.Second), checksum))
				require.NoError(t, err, name)
				row := storedRow(t, db)
				require.NotNil(t, row, "%s: the snapshot applied through this handle is stored", name)
				assert.Equal(t, checksum, row.Checksum, name)

				// What the edge reports is one state per database and
				// namespace, whichever handle asks.
				got, _, ok := NewEdgeSyncService(db, "ns").ReportedConfig()
				require.True(t, ok, name)
				assert.Equal(t, checksum, got, name)
				got, _, ok = svc.ReportedConfig()
				require.True(t, ok, name)
				assert.Equal(t, checksum, got, name)
			}

			// The control contact, written and read through h.
			controlContactWrites.Delete(dbKey(db)) // not held back by the write interval
			contact := at.Add(time.Duration(n) * time.Hour)
			require.NoError(t, RecordControlContact(h, contact), name)
			read, ok, err := LastControlContact(h)
			require.NoError(t, err, name)
			require.True(t, ok, "%s: the contact recorded through this handle is there", name)
			assert.True(t, read.Equal(contact), "%s: %s", name, read)
		})

		// One entry per database in the registries that are created on
		// first use, however many handles were used.
		entries := 0
		controlContactWrites.Range(func(k, _ any) bool {
			if k == dbKey(db) {
				entries++
			}
			return true
		})
		assert.Equal(t, 1, entries)

		// A snapshot being restored from the database, announced through one
		// handle and applied through another: the row is not rewritten.
		before := storedRow(t, db)
		lkg, err := LoadLastKnownGood(db, snapshotTestKey, "ns", 0, time.Now())
		require.NoError(t, err)
		time.Sleep(1100 * time.Millisecond) // applied_at has one-second resolution
		KeepStoredSnapshot(db.WithContext(context.Background()), lkg)
		appliedSnapshots.Delete(appliedKey{db: dbKey(db), namespace: "ns"}) // as after a restart
		_, err = NewEdgeSyncService(db.Session(&gorm.Session{}), "ns").SyncConfigurationReport(lkg.Snapshot)
		require.NoError(t, err)
		after := storedRow(t, db)
		assert.Equal(t, before.AppliedAt, after.AppliedAt, "the stored snapshot was restored, not received: its row keeps its time")
	})
}

// A database without a snapshot store keeps nothing, as before; that is now
// said, once per database, as an error: on an edge it means the edge could
// not start without a control plane.
func TestMissingSnapshotStoreIsReportedOnce(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = prev })

	for i := 0; i < 3; i++ {
		require.NoError(t, RecordControlContact(db.WithContext(context.Background()), time.Now()))
		_, ok, err := LastControlContact(db)
		require.NoError(t, err)
		require.False(t, ok)
		_, err = NewEdgeSyncService(db, "ns").SyncConfigurationReport(richSnapshot(time.Now(), strings.Repeat("c", 64)))
		require.NoError(t, err)
	}
	assert.Nil(t, storedRow(t, db))
	assert.Equal(t, 1, strings.Count(buf.String(), "has no snapshot store"), buf.String())
	assert.Contains(t, buf.String(), `"level":"error"`)

	// Once it has one, the next lack of it would be said again.
	require.NoError(t, PersistAppliedSnapshots(db, snapshotTestKey))
	_, logged := missingSealerLogged.Load(dbKey(db))
	assert.False(t, logged)
}
