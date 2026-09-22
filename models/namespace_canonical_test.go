package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The global/default namespace has three spellings in the wild: "" (object
// tables, the microgateway's EDGE_NAMESPACE default), "global" (the admin
// API) and "default" (edge registration). Sync bookkeeping keys one row per
// logical namespace, so every spelling must land on the same row.

func TestCanonicalNamespace(t *testing.T) {
	for _, in := range []string{"", "global", "default", " default ", "GLOBAL"} {
		assert.Equal(t, DefaultNamespace, CanonicalNamespace(in), "%q", in)
	}
	assert.Equal(t, "eu-west", CanonicalNamespace("eu-west"))
	assert.Equal(t, "eu-west", CanonicalNamespace(" eu-west "))
	assert.ElementsMatch(t, []string{"default", "", "global"}, NamespaceAliases(""))
	assert.Equal(t, []string{"eu-west"}, NamespaceAliases("eu-west"))
}

func TestNamespaceSyncStatus_AllSpellingsShareOneRow(t *testing.T) {
	db := setupSyncStatusTestDB(t)
	now := time.Now()

	// A push recorded under "" and a snapshot recorded under "default" are
	// the same namespace: one row, both facts on it.
	require.NoError(t, MarkNamespacePushed(db, "", now))
	upsert := &NamespaceSyncStatus{Namespace: "global", ExpectedChecksum: "abc", ConfigVersion: "1", LastConfigChange: now}
	require.NoError(t, upsert.Upsert(db))

	var count int64
	require.NoError(t, db.Model(&NamespaceSyncStatus{}).Count(&count).Error)
	assert.Equal(t, int64(1), count, "one row per logical namespace")

	for _, spelling := range []string{"", "global", "default"} {
		var row NamespaceSyncStatus
		require.NoError(t, row.GetByNamespace(db, spelling), spelling)
		assert.Equal(t, DefaultNamespace, row.Namespace, spelling)
		assert.Equal(t, "abc", row.ExpectedChecksum, spelling)
		require.NotNil(t, row.LastPushAt, spelling)
	}
}

func TestNamespaceSyncStatus_GetByNamespace_FallsBackToLegacyRow(t *testing.T) {
	db := setupSyncStatusTestDB(t)
	now := time.Now()

	// A database from before canonicalisation: the global row is keyed "".
	legacy := &NamespaceSyncStatus{Namespace: "", ExpectedChecksum: "legacy", ConfigVersion: "1", LastConfigChange: now, LastPushAt: &now}
	require.NoError(t, db.Create(legacy).Error)

	var row NamespaceSyncStatus
	require.NoError(t, row.GetByNamespace(db, "default"))
	assert.Equal(t, "legacy", row.ExpectedChecksum)

	// A push against the canonical name updates the legacy row rather than
	// creating a second one.
	later := now.Add(time.Minute)
	require.NoError(t, MarkNamespacePushed(db, "default", later))
	var count int64
	require.NoError(t, db.Model(&NamespaceSyncStatus{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	require.NoError(t, row.GetByNamespace(db, ""))
	assert.WithinDuration(t, later, *row.LastPushAt, time.Second)
}

func TestMergeLegacyNamespaceSyncStatus(t *testing.T) {
	t.Run("legacy row alone is renamed", func(t *testing.T) {
		db := setupSyncStatusTestDB(t)
		now := time.Now()
		require.NoError(t, db.Create(&NamespaceSyncStatus{Namespace: "", ExpectedChecksum: "old", ConfigVersion: "1", LastConfigChange: now, LastPushAt: &now}).Error)

		require.NoError(t, MergeLegacyNamespaceSyncStatus(db))

		var rows []NamespaceSyncStatus
		require.NoError(t, db.Unscoped().Find(&rows).Error)
		require.Len(t, rows, 1, "no tombstone left behind the unique index")
		assert.Equal(t, DefaultNamespace, rows[0].Namespace)
		assert.Equal(t, "old", rows[0].ExpectedChecksum)
		require.NotNil(t, rows[0].LastPushAt)
	})

	t.Run("legacy and canonical rows are merged keeping the newest facts", func(t *testing.T) {
		db := setupSyncStatusTestDB(t)
		t0 := time.Now().Add(-3 * time.Hour)
		t1 := t0.Add(time.Hour)
		t2 := t1.Add(time.Hour)
		// Legacy "" row: older checksum, but the only push stamp.
		require.NoError(t, db.Create(&NamespaceSyncStatus{Namespace: "", ExpectedChecksum: "old", ConfigVersion: "1", LastConfigChange: t0, LastPushAt: &t1}).Error)
		// Canonical row: newer checksum, never pushed.
		require.NoError(t, db.Create(&NamespaceSyncStatus{Namespace: "default", ExpectedChecksum: "new", ConfigVersion: "2", LastConfigChange: t2}).Error)
		// A "global" row with an even newer push.
		t3 := t2.Add(time.Minute)
		require.NoError(t, db.Create(&NamespaceSyncStatus{Namespace: "global", LastConfigChange: t0, LastPushAt: &t3}).Error)

		require.NoError(t, MergeLegacyNamespaceSyncStatus(db))
		require.NoError(t, MergeLegacyNamespaceSyncStatus(db), "idempotent")

		var rows []NamespaceSyncStatus
		require.NoError(t, db.Unscoped().Find(&rows).Error)
		require.Len(t, rows, 1)
		assert.Equal(t, DefaultNamespace, rows[0].Namespace)
		assert.Equal(t, "new", rows[0].ExpectedChecksum)
		assert.Equal(t, "2", rows[0].ConfigVersion)
		assert.WithinDuration(t, t2, rows[0].LastConfigChange, time.Second)
		require.NotNil(t, rows[0].LastPushAt)
		assert.WithinDuration(t, t3, *rows[0].LastPushAt, time.Second)
	})

	t.Run("other namespaces are untouched", func(t *testing.T) {
		db := setupSyncStatusTestDB(t)
		now := time.Now()
		require.NoError(t, db.Create(&NamespaceSyncStatus{Namespace: "eu", ExpectedChecksum: "x", ConfigVersion: "1", LastConfigChange: now}).Error)
		require.NoError(t, MergeLegacyNamespaceSyncStatus(db))
		var row NamespaceSyncStatus
		require.NoError(t, row.GetByNamespace(db, "eu"))
		assert.Equal(t, "x", row.ExpectedChecksum)
	})
}

// Edge rows are stored under the registered ("default") spelling; sync
// queries must still see legacy "" rows as the same namespace.
func TestEdgeInstance_NamespaceQueriesAcceptEverySpelling(t *testing.T) {
	db := setupSyncStatusTestDB(t)
	now := time.Now()
	for _, e := range []EdgeInstance{
		{EdgeID: "stored-default", Namespace: "default", Status: EdgeStatusConnected, SyncStatus: EdgeSyncStatusInSync, LoadedChecksum: "abc", LastSyncAck: &now},
		{EdgeID: "legacy-empty", Namespace: "", Status: EdgeStatusConnected, SyncStatus: EdgeSyncStatusInSync, LoadedChecksum: "abc", LastSyncAck: &now},
		{EdgeID: "elsewhere", Namespace: "eu", Status: EdgeStatusConnected, SyncStatus: EdgeSyncStatusInSync, LoadedChecksum: "abc", LastSyncAck: &now},
	} {
		e := e
		require.NoError(t, db.Create(&e).Error)
	}

	var edges EdgeInstances
	require.NoError(t, edges.ListEdgesInNamespace(db, ""))
	assert.Len(t, edges, 2)
	require.NoError(t, edges.ListEdgesInNamespace(db, "global"))
	assert.Len(t, edges, 2)

	counts, err := (&EdgeInstance{}).CountEdgesBySyncStatus(db, "default")
	require.NoError(t, err)
	assert.Equal(t, int64(2), counts[EdgeSyncStatusInSync])

	acked, err := (&EdgeInstance{}).FirstInSyncWithChecksum(db, "global", "abc")
	require.NoError(t, err)
	require.NotNil(t, acked)
	assert.Contains(t, []string{"stored-default", "legacy-empty"}, acked.EdgeID)
	missing, err := (&EdgeInstance{}).FirstInSyncWithChecksum(db, "default", "zzz")
	require.NoError(t, err)
	assert.Nil(t, missing)

	require.NoError(t, (&EdgeInstance{}).MarkEdgesAsPendingInNamespace(db, ""))
	var pending int64
	require.NoError(t, db.Model(&EdgeInstance{}).Where("sync_status = ?", EdgeSyncStatusPending).Count(&pending).Error)
	assert.Equal(t, int64(2), pending, "both spellings of the default namespace, not eu")
}
