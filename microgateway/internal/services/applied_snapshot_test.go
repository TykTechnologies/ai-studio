package services

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

const (
	snapshotTestKey  = "0123456789abcdef0123456789abcdef"
	snapshotOtherKey = "fedcba9876543210fedcba9876543210"
)

// forEachEdgeDB runs body on an edge database on SQLite and, when
// MGW_TEST_POSTGRES_DSN is set, on Postgres.
func forEachEdgeDB(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) { body(t, setupEdgeSyncTestDB(t)) })
	t.Run("postgres", func(t *testing.T) { body(t, openPostgresSchema(t)) })
}

func persistingSync(t *testing.T, db *gorm.DB, namespace string) *EdgeSyncService {
	t.Helper()
	require.NoError(t, PersistAppliedSnapshots(db, snapshotTestKey))
	t.Cleanup(func() {
		snapshotSealers.Delete(db.Config)
		// What the test snapshots installed in memory.
		installPromptTemplates(nil)
		installGuardrailExceptions(nil)
	})
	return NewEdgeSyncService(db, namespace)
}

func storedRow(t *testing.T, db *gorm.DB) *database.AppliedSnapshot {
	t.Helper()
	var rows []database.AppliedSnapshot
	require.NoError(t, db.Find(&rows).Error)
	require.LessOrEqual(t, len(rows), 1, "applied_snapshots holds one row")
	if len(rows) == 0 {
		return nil
	}
	return &rows[0]
}

func deterministic(t *testing.T, snap *pb.ConfigurationSnapshot) []byte {
	t.Helper()
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(snap)
	require.NoError(t, err)
	return b
}

// A snapshot with what an edge keeps only in memory, so the stored copy is
// shown to carry it: a prompt template and a guardrail exception.
func richSnapshot(at time.Time, checksum string) *pb.ConfigurationSnapshot {
	snap := snapshotAt(at, checksum,
		[]*pb.LLMConfig{syncLLM(1, "fast", at.Add(-time.Hour))},
		[]*pb.AppConfig{syncApp(1, []uint32{1}, at.Add(-time.Hour))})
	snap.PromptTemplates = []*pb.PromptTemplateConfig{{Id: 7}}
	snap.GuardrailExceptions = []*pb.GuardrailExceptionConfig{{Id: 9}}
	return snap
}

// After a sync the stored row decrypts to a snapshot byte-equal to the one
// applied, with the columns the edge reports to the hub.
func TestAppliedSnapshot_StoredWithTheApply(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		s := persistingSync(t, db, "emea")
		snap := richSnapshot(time.Now().Add(-time.Minute), "c1")
		before := time.Now().Add(-2 * time.Second)
		require.NoError(t, s.SyncConfiguration(snap))

		stored, err := LoadAppliedSnapshot(db, snapshotTestKey)
		require.NoError(t, err)
		assert.Equal(t, deterministic(t, snap), deterministic(t, stored.Snapshot), "the stored snapshot is the one applied, byte for byte")
		assert.Len(t, stored.Snapshot.PromptTemplates, 1)
		assert.Len(t, stored.Snapshot.GuardrailExceptions, 1)
		assert.Equal(t, "emea", stored.Namespace)
		assert.Equal(t, snap.Version, stored.Version)
		assert.WithinRange(t, stored.AppliedAt, before, time.Now().Add(2*time.Second))

		reported, _, ok := s.ReportedConfig()
		require.True(t, ok)
		assert.Equal(t, reported, stored.Checksum, "the stored checksum is the one heartbeats report")
		assert.Equal(t, "c1", stored.Checksum)

		// The column holds ciphertext: nothing of the snapshot is readable.
		row := storedRow(t, db)
		require.NotNil(t, row)
		assert.EqualValues(t, database.AppliedSnapshotID, row.ID)
		assert.False(t, bytes.Contains(row.Snapshot, []byte("LLM fast")), "the stored snapshot is not encrypted")
		assert.False(t, bytes.Contains(row.Snapshot, deterministic(t, snap)[:16]))
	})
}

// A newer snapshot replaces the row; there is only ever one.
func TestAppliedSnapshot_ReplacedByTheNextApply(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		s := persistingSync(t, db, "")
		at := time.Now().Add(-time.Minute)
		require.NoError(t, s.SyncConfiguration(richSnapshot(at, "c1")))
		second := richSnapshot(at.Add(10*time.Second), "c2")
		second.Llms = append(second.Llms, syncLLM(2, "smart", at))
		require.NoError(t, s.SyncConfiguration(second))

		stored, err := LoadAppliedSnapshot(db, snapshotTestKey)
		require.NoError(t, err)
		assert.Equal(t, "c2", stored.Checksum)
		assert.Len(t, stored.Snapshot.Llms, 2)
		require.NotNil(t, storedRow(t, db)) // asserts a single row
	})
}

// A snapshot that fails to apply does not replace the stored one: the row
// is written in the apply's transaction.
func TestAppliedSnapshot_FailedApplyKeepsThePreviousRow(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		s := persistingSync(t, db, "")
		at := time.Now().Add(-time.Minute)
		first := richSnapshot(at, "good")
		require.NoError(t, s.SyncConfiguration(first))
		before := storedRow(t, db)

		// The next snapshot cannot be applied: a table it writes is gone.
		require.NoError(t, db.Exec("DROP TABLE tool_filters").Error)
		require.NoError(t, db.Exec("DROP TABLE app_tools").Error)
		require.NoError(t, db.Exec("DROP TABLE tools").Error)
		bad := richSnapshot(at.Add(10*time.Second), "bad")
		bad.Tools = []*pb.ToolConfig{{Id: 1, Name: "t", Slug: "t"}}
		require.Error(t, s.SyncConfiguration(bad))

		after := storedRow(t, db)
		require.NotNil(t, after)
		assert.Equal(t, before.Snapshot, after.Snapshot, "the stored snapshot was replaced by one that was not applied")
		stored, err := LoadAppliedSnapshot(db, snapshotTestKey)
		require.NoError(t, err)
		assert.Equal(t, "good", stored.Checksum)
		assert.Equal(t, deterministic(t, first), deterministic(t, stored.Snapshot))
	})
}

// If the row itself cannot be written, the apply is rolled back with it.
func TestAppliedSnapshot_StoreFailureRollsTheApplyBack(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	s := persistingSync(t, db, "")
	at := time.Now().Add(-time.Minute)
	require.NoError(t, s.SyncConfiguration(snapshotAt(at, "c1",
		[]*pb.LLMConfig{syncLLM(1, "fast", at.Add(-time.Hour))}, nil)))

	require.NoError(t, db.Exec("DROP TABLE applied_snapshots").Error)
	err := s.SyncConfiguration(snapshotAt(at.Add(10*time.Second), "c2",
		[]*pb.LLMConfig{syncLLM(1, "fast", at.Add(-time.Hour)), syncLLM(2, "smart", at)}, nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applied snapshot")

	var n int64
	require.NoError(t, db.Model(&database.LLM{}).Count(&n).Error)
	assert.EqualValues(t, 1, n, "the tables still hold the configuration of the stored snapshot")
	sum, _, _ := s.ReportedConfig()
	assert.Equal(t, "c1", sum, "the edge still reports the configuration it runs")
}

// An unchanged snapshot (same fingerprint, apply skipped) does not rewrite
// the row.
func TestAppliedSnapshot_UnchangedSnapshotIsNotRewritten(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		s := persistingSync(t, db, "")
		snap := richSnapshot(time.Now().Add(-time.Minute), "c1")
		require.NoError(t, s.SyncConfiguration(snap))
		before := storedRow(t, db)

		// The same snapshot again, through another service on the same
		// database, as a reload hands it to the callback and the handler.
		require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(proto.Clone(snap).(*pb.ConfigurationSnapshot)))
		after := storedRow(t, db)
		// A rewrite would have drawn a new nonce: the bytes would differ.
		assert.Equal(t, before.Snapshot, after.Snapshot)
		assert.True(t, before.AppliedAt.Equal(after.AppliedAt))
	})
}

// The stored snapshot cannot be read without the ENCRYPTION_KEY it was
// stored with, and a row whose columns were changed no longer opens.
func TestAppliedSnapshot_NeedsTheKeyAndAnUntouchedRow(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		s := persistingSync(t, db, "emea")
		require.NoError(t, s.SyncConfiguration(richSnapshot(time.Now().Add(-time.Minute), "c1")))

		_, err := LoadAppliedSnapshot(db, snapshotOtherKey)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not decrypt")
		assert.NotErrorIs(t, err, ErrNoStoredSnapshot)

		_, err = LoadAppliedSnapshot(db, snapshotTestKey)
		require.NoError(t, err)

		for column, value := range map[string]interface{}{
			"namespace":  "apac",
			"checksum":   "c2",
			"version":    "999",
			"applied_at": time.Now().Add(48 * time.Hour),
		} {
			var saved database.AppliedSnapshot
			require.NoError(t, db.First(&saved).Error)
			require.NoError(t, db.Model(&database.AppliedSnapshot{}).Where("id = ?", database.AppliedSnapshotID).Update(column, value).Error)
			_, err := LoadAppliedSnapshot(db, snapshotTestKey)
			assert.Error(t, err, "a changed %s went unnoticed", column)
			require.NoError(t, db.Save(&saved).Error)
			_, err = LoadAppliedSnapshot(db, snapshotTestKey)
			require.NoError(t, err, "restoring %s", column)
		}
	})
}

// A snapshot applied with entries left out stores the checksum the edge
// reports for it (not the hub's), and the snapshot as the hub sent it, so
// applying it again leaves out the same entries.
func TestAppliedSnapshot_PartialApplyKeepsItsReportedChecksum(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		s := persistingSync(t, db, "")
		at := time.Now().Add(-time.Minute)
		snap := snapshotAt(at, "hub-checksum", []*pb.LLMConfig{
			syncLLM(1, "gpt", at.Add(-time.Hour)),
			syncLLM(9, "gpt", at.Add(-time.Hour)), // repeats a slug: left out
		}, nil)
		report, err := s.SyncConfigurationReport(snap)
		require.NoError(t, err)
		require.NotEmpty(t, report.Skipped)

		stored, err := LoadAppliedSnapshot(db, snapshotTestKey)
		require.NoError(t, err)
		reported, _, _ := s.ReportedConfig()
		assert.Equal(t, reported, stored.Checksum)
		assert.NotEqual(t, "hub-checksum", stored.Checksum)
		assert.Equal(t, appliedChecksum("hub-checksum", report.Skipped), stored.Checksum)
		assert.Len(t, stored.Snapshot.Llms, 2, "the snapshot is stored as received")

		// Applied again from the stored copy, on a fresh process state, it
		// reports the same checksum.
		appliedSnapshots.Delete(appliedKey{config: db.Config, namespace: ""})
		reportedConfigs.Delete(appliedKey{config: db.Config, namespace: ""})
		again := NewEdgeSyncService(db, "")
		require.NoError(t, again.SyncConfiguration(stored.Snapshot))
		sum, _, ok := again.ReportedConfig()
		require.True(t, ok)
		assert.Equal(t, stored.Checksum, sum)
	})
}

// A database that does not persist snapshots (a standalone gateway, a
// control instance) stores none.
func TestAppliedSnapshot_OffWithoutPersistence(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	at := time.Now().Add(-time.Minute)
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(richSnapshot(at, "c1")))
	assert.Nil(t, storedRow(t, db))
	_, err := LoadAppliedSnapshot(db, snapshotTestKey)
	assert.ErrorIs(t, err, ErrNoStoredSnapshot)
}

// A snapshot can be as large as GRPC_MAX_MESSAGE_SIZE (16 MB): the column
// takes it on both databases and it reads back whole.
func TestAppliedSnapshot_SixteenMegabytes(t *testing.T) {
	if testing.Short() {
		t.Skip("stores a 16 MB snapshot")
	}
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		s := persistingSync(t, db, "")
		at := time.Now().Add(-time.Minute)
		// Random, so nothing compresses it on the way: 8 MiB of bytes as 16
		// MiB of hexadecimal text in a filter's script.
		raw := make([]byte, 8<<20)
		_, err := rand.Read(raw)
		require.NoError(t, err)
		snap := snapshotAt(at, "big", []*pb.LLMConfig{syncLLM(1, "fast", at.Add(-time.Hour))}, nil)
		snap.Filters = []*pb.FilterConfig{{Id: 1, Name: "big filter", Script: hex.EncodeToString(raw), IsActive: true}}
		size := proto.Size(snap)
		require.GreaterOrEqual(t, size, 16<<20, "the snapshot is at least 16 MiB")

		require.NoError(t, s.SyncConfiguration(snap))
		row := storedRow(t, db)
		require.NotNil(t, row)
		assert.Greater(t, len(row.Snapshot), size, "ciphertext, nonce and tag")

		stored, err := LoadAppliedSnapshot(db, snapshotTestKey)
		require.NoError(t, err)
		assert.True(t, bytes.Equal(deterministic(t, snap), deterministic(t, stored.Snapshot)))
	})
}

// The columns are bound to the ciphertext unambiguously: values that run
// into each other (a field that contains what would separate two fields)
// give different bytes.
func TestAppliedSnapshotAAD_IsUnambiguous(t *testing.T) {
	at := time.Unix(1794354337, 0)
	rows := []database.AppliedSnapshot{
		{Checksum: "c", Version: "a\x00b", Namespace: "c", AppliedAt: at},
		{Checksum: "c", Version: "a", Namespace: "b\x00c", AppliedAt: at},
		{Checksum: "c", Version: "ab", Namespace: "c", AppliedAt: at},
		{Checksum: "c", Version: "a", Namespace: "bc", AppliedAt: at},
		{Checksum: "ca", Version: "", Namespace: "bc", AppliedAt: at},
		{Checksum: "c", Version: "a", Namespace: "bc", AppliedAt: at.Add(time.Second)},
	}
	seen := map[string]int{}
	for i := range rows {
		aad := string(appliedSnapshotAAD(&rows[i]))
		if j, dup := seen[aad]; dup {
			t.Errorf("rows %d and %d are bound with the same bytes", j, i)
		}
		seen[aad] = i
	}
	// Sub-second precision does not count: databases keep different ones.
	a := database.AppliedSnapshot{Checksum: "c", AppliedAt: at}
	b := database.AppliedSnapshot{Checksum: "c", AppliedAt: at.Add(300 * time.Millisecond)}
	assert.Equal(t, appliedSnapshotAAD(&a), appliedSnapshotAAD(&b))
}

func TestNewSnapshotSealer_NeedsAKey(t *testing.T) {
	_, err := newSnapshotSealer("")
	require.Error(t, err)
	require.Error(t, PersistAppliedSnapshots(setupEdgeSyncTestDB(t), ""))
}
