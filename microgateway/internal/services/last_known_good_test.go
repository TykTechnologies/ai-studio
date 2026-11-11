package services

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lkgSnapshot is a snapshot with what an edge keeps only in memory: a
// prompt template and a guardrail exception (and so a filter and an App).
func lkgSnapshot(at time.Time, checksum string) *pb.ConfigurationSnapshot {
	snap := snapshotAt(at, checksum,
		[]*pb.LLMConfig{syncLLM(1, "fast", at.Add(-time.Hour))},
		[]*pb.AppConfig{syncApp(1, []uint32{1}, at.Add(-time.Hour))})
	snap.Filters = []*pb.FilterConfig{{Id: 5, Name: "guard", Script: "return true", IsActive: true}}
	snap.PromptTemplates = []*pb.PromptTemplateConfig{{
		Id: 7, AppId: 1, Name: "review", Version: 1, PlacementRole: "user",
		Parts: []*pb.PromptTemplatePart{
			{Constant: "Please review the following change for security problems and say what you find:"},
			{Slot: "diff"},
			{Constant: "Answer with a list of findings, most serious first, and nothing else."},
		},
	}}
	snap.GuardrailExceptions = []*pb.GuardrailExceptionConfig{{
		Id: 9, AppId: 1, FilterId: 5, Detector: "prompt_attack", Channel: "text_user", Action: "log", AppCapability: "read_only",
	}}
	return snap
}

func clearInstalled(t *testing.T) {
	t.Helper()
	installPromptTemplates(nil)
	installGuardrailExceptions(nil)
}

func setContact(t *testing.T, db *gorm.DB, at time.Time) {
	t.Helper()
	controlContactWrites.Delete(db.Config)
	require.NoError(t, RecordControlContact(db, at))
}

// The edge records its last contact with a control plane, at most once a
// minute.
func TestRecordControlContact(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		// Without the database's snapshot store there is no key to
		// authenticate a contact with: nothing is recorded.
		require.NoError(t, RecordControlContact(db, time.Now()))
		var unkeyed int64
		require.NoError(t, db.Model(&database.SyncState{}).Where("topic = ?", controlContactTopic).Count(&unkeyed).Error)
		require.Zero(t, unkeyed)
		controlContactWrites.Delete(db.Config)

		persistingSync(t, db, "")
		_, ok, err := LastControlContact(db)
		require.NoError(t, err)
		assert.False(t, ok, "an edge that never had a control plane has no contact time")

		t0 := time.Now().Add(-10 * time.Minute).UTC().Truncate(time.Second)
		require.NoError(t, RecordControlContact(db, t0))
		at, ok, err := LastControlContact(db)
		require.NoError(t, err)
		require.True(t, ok)
		assert.True(t, at.Equal(t0), "%s", at)

		// Heartbeats every 30 s do not write every time...
		require.NoError(t, RecordControlContact(db, t0.Add(30*time.Second)))
		at, _, _ = LastControlContact(db)
		assert.True(t, at.Equal(t0), "a second contact within the minute is not written")

		// ... but the next one after a minute does, into the same row.
		require.NoError(t, RecordControlContact(db, t0.Add(90*time.Second)))
		at, _, _ = LastControlContact(db)
		assert.True(t, at.Equal(t0.Add(90*time.Second)), "%s", at)
		var rows int64
		require.NoError(t, db.Model(&database.SyncState{}).Where("topic = ?", controlContactTopic).Count(&rows).Error)
		assert.EqualValues(t, 1, rows)
	})
}

func refusal(t *testing.T, err error) *LastKnownGoodRefusal {
	t.Helper()
	var r *LastKnownGoodRefusal
	require.Error(t, err)
	require.True(t, errors.As(err, &r), "not a refusal: %v", err)
	return r
}

// Each reason an edge may not start from its stored snapshot.
func TestLoadLastKnownGood_Refusals(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		t.Cleanup(func() { clearInstalled(t) })
		now := time.Now()

		// Nothing stored: a new database, or an edge that never applied a
		// snapshot.
		_, err := LoadLastKnownGood(db, snapshotTestKey, "emea", 72*time.Hour, now)
		assert.Contains(t, refusal(t, err).Reason, "no configuration snapshot is stored")

		s := persistingSync(t, db, "emea")
		require.NoError(t, s.SyncConfiguration(lkgSnapshot(now.Add(-time.Minute), "c1")))

		// Usable.
		lkg, err := LoadLastKnownGood(db, snapshotTestKey, "emea", 72*time.Hour, now)
		require.NoError(t, err)
		assert.Equal(t, "c1", lkg.Checksum)
		assert.WithinDuration(t, now, lkg.LastContact, 5*time.Second, "with no contact recorded, the time it was applied counts")

		// Stored for another namespace.
		_, err = LoadLastKnownGood(db, snapshotTestKey, "apac", 72*time.Hour, now)
		r := refusal(t, err)
		assert.Contains(t, r.Reason, `is for namespace "emea"`)
		assert.Contains(t, r.Reason, `EDGE_NAMESPACE is "apac"`)

		// Another ENCRYPTION_KEY.
		_, err = LoadLastKnownGood(db, snapshotOtherKey, "emea", 72*time.Hour, now)
		r = refusal(t, err)
		assert.Contains(t, r.Reason, "cannot be read")
		assert.Contains(t, r.Error(), "does not decrypt with this ENCRYPTION_KEY")

		// Without a control plane for longer than the limit.
		_, err = LoadLastKnownGood(db, snapshotTestKey, "emea", 72*time.Hour, now.Add(73*time.Hour))
		r = refusal(t, err)
		assert.Contains(t, r.Reason, "longer than EDGE_LAST_KNOWN_GOOD_MAX_AGE (72h0m0s)")
		assert.Contains(t, r.Reason, "73h")

		// 0 is no limit.
		_, err = LoadLastKnownGood(db, snapshotTestKey, "emea", 0, now.Add(10000*time.Hour))
		require.NoError(t, err)
	})
}

// The limit is on how long the edge has been without a control plane, not
// on how long ago its configuration last changed: an edge whose
// configuration has been the same for a month, and which had a control
// plane an hour ago, starts from it.
func TestLoadLastKnownGood_AgeIsTimeWithoutAControlPlane(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		t.Cleanup(func() { clearInstalled(t) })
		s := persistingSync(t, db, "")
		applied := time.Now()
		require.NoError(t, s.SyncConfiguration(lkgSnapshot(applied.Add(-time.Minute), "c1")))

		// A month later the snapshot is unchanged (so the row is as old),
		// and the edge last heard from a control plane an hour ago.
		now := applied.Add(30 * 24 * time.Hour)
		setContact(t, db, now.Add(-time.Hour))
		lkg, err := LoadLastKnownGood(db, snapshotTestKey, "", 72*time.Hour, now)
		require.NoError(t, err, "a stable configuration is not a stale one")
		assert.WithinDuration(t, now.Add(-time.Hour), lkg.LastContact, time.Second)
		assert.WithinDuration(t, applied, lkg.AppliedAt, 5*time.Second, "applied_at still says when it was applied")

		// Four days after that contact it is too long.
		_, err = LoadLastKnownGood(db, snapshotTestKey, "", 72*time.Hour, now.Add(95*time.Hour))
		assert.Contains(t, refusal(t, err).Reason, "longer than EDGE_LAST_KNOWN_GOOD_MAX_AGE")

		// A contact time older than the snapshot (the snapshot arrived
		// later) does not make it older than it is.
		setContact(t, db, applied.Add(-100*time.Hour))
		lkg, err = LoadLastKnownGood(db, snapshotTestKey, "", 72*time.Hour, applied.Add(time.Hour))
		require.NoError(t, err)
		assert.WithinDuration(t, applied, lkg.LastContact, 5*time.Second)
	})
}

// An edge restarted without a control plane applies the stored snapshot
// again: the tables and what it holds only in memory come back, it reports
// the same checksum, and the stored row is left as it was (applying it must
// not make it look newly received).
func TestLastKnownGood_RestoresConfigurationAndKeepsTheRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "edge.db")
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		require.NoError(t, database.Migrate(db))
		t.Cleanup(func() { database.Close(db) })
		return db
	}
	t.Cleanup(func() { clearInstalled(t) })

	// First run: connected, a snapshot with a left-out entry is applied.
	db := open()
	first := persistingSync(t, db, "")
	at := time.Now().Add(-time.Minute)
	snap := lkgSnapshot(at, "hub-checksum")
	snap.Llms = append(snap.Llms, syncLLM(9, "fast", at)) // repeats a slug: left out
	require.NoError(t, first.SyncConfiguration(snap))
	reported, _, _ := first.ReportedConfig()
	require.NotEqual(t, "hub-checksum", reported, "a partial apply reports a checksum of its own")
	before := storedRow(t, db)
	require.Equal(t, 1, InstalledPromptTemplates(1))
	require.Equal(t, 1, InstalledGuardrailExceptions(1))

	// The process ends: what it held in memory is gone, and the tables are
	// emptied here to show the restore fills them.
	clearInstalled(t)
	require.NoError(t, db.Exec("DELETE FROM llms").Error)
	require.NoError(t, db.Exec("DELETE FROM filters").Error)
	require.Zero(t, InstalledPromptTemplates(1))

	// Second run, no control plane: a new handle on the same database.
	db2 := open()
	require.NoError(t, PersistAppliedSnapshots(db2, snapshotTestKey))
	t.Cleanup(func() { snapshotSealers.Delete(db2.Config) })
	lkg, err := LoadLastKnownGood(db2, snapshotTestKey, "", 72*time.Hour, time.Now())
	require.NoError(t, err)
	KeepStoredSnapshot(db2, lkg)
	second := NewEdgeSyncService(db2, "")
	require.NoError(t, second.SyncConfiguration(lkg.Snapshot))

	var llms, filters int64
	require.NoError(t, db2.Model(&database.LLM{}).Count(&llms).Error)
	require.NoError(t, db2.Model(&database.Filter{}).Count(&filters).Error)
	assert.EqualValues(t, 1, llms, "the LLMs are back (the left-out one is left out again)")
	assert.EqualValues(t, 1, filters, "the filters are back")
	assert.Equal(t, 1, InstalledPromptTemplates(1), "the prompt template is back in memory")
	assert.Equal(t, 1, InstalledGuardrailExceptions(1), "the guardrail exception is back in memory")
	again, _, ok := second.ReportedConfig()
	require.True(t, ok)
	assert.Equal(t, reported, again, "the restored configuration reports the checksum it had")
	assert.Equal(t, lkg.Checksum, again)

	after := storedRow(t, db2)
	assert.Equal(t, before.Snapshot, after.Snapshot, "the stored row was rewritten by the restore")
	assert.True(t, before.AppliedAt.Equal(after.AppliedAt))

	// The live configuration, when a control plane sends one, replaces it.
	live := lkgSnapshot(time.Now(), "live")
	live.Llms = append(live.Llms, syncLLM(2, "smart", at))
	require.NoError(t, second.SyncConfiguration(live))
	stored, err := LoadAppliedSnapshot(db2, snapshotTestKey)
	require.NoError(t, err)
	assert.Equal(t, "live", stored.Checksum)
	assert.Len(t, stored.Snapshot.Llms, 2)
}

// The mark that keeps the stored row applies to that snapshot only: if the
// first thing applied is something else (a control plane answered after
// all), the row is replaced as usual.
func TestKeepStoredSnapshot_OnlyForTheStoredSnapshot(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	t.Cleanup(func() { clearInstalled(t) })
	s := persistingSync(t, db, "")
	at := time.Now().Add(-time.Minute)
	require.NoError(t, s.SyncConfiguration(lkgSnapshot(at, "c1")))
	lkg, err := LoadLastKnownGood(db, snapshotTestKey, "", 0, time.Now())
	require.NoError(t, err)
	KeepStoredSnapshot(db, lkg)

	other := lkgSnapshot(at.Add(10*time.Second), "c2")
	require.NoError(t, s.SyncConfiguration(other))
	stored, err := LoadAppliedSnapshot(db, snapshotTestKey)
	require.NoError(t, err)
	assert.Equal(t, "c2", stored.Checksum)

	// And the mark is spent: the first snapshot, applied later by a control
	// plane (a rollback), is stored again.
	appliedSnapshots.Delete(appliedKey{config: db.Config, namespace: ""})
	require.NoError(t, s.SyncConfiguration(lkg.Snapshot))
	stored, err = LoadAppliedSnapshot(db, snapshotTestKey)
	require.NoError(t, err)
	assert.Equal(t, "c1", stored.Checksum)
}

// The recorded contact time is authenticated: pushed forward in the
// database (or written under another ENCRYPTION_KEY) it is ignored, and the
// age is measured from when the snapshot was applied, which the snapshot's
// own row authenticates.
func TestLoadLastKnownGood_TamperedContactFallsBackToAppliedAt(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		t.Cleanup(func() { clearInstalled(t) })
		s := persistingSync(t, db, "")
		applied := time.Now()
		require.NoError(t, s.SyncConfiguration(lkgSnapshot(applied.Add(-time.Minute), "c1")))
		now := applied.Add(100 * time.Hour)

		// The edge last had a control plane 99 hours ago: too long.
		setContact(t, db, applied.Add(time.Hour))
		_, err := LoadLastKnownGood(db, snapshotTestKey, "", 72*time.Hour, now)
		require.Contains(t, refusal(t, err).Reason, "longer than EDGE_LAST_KNOWN_GOOD_MAX_AGE")

		// Someone with access to the database moves the time to an hour ago.
		require.NoError(t, db.Model(&database.SyncState{}).Where("topic = ?", controlContactTopic).
			Update("last_sync_at", now.Add(-time.Hour).UTC().Truncate(time.Second)).Error)
		at, ok, err := LastControlContact(db)
		require.NoError(t, err)
		assert.False(t, ok, "a changed time is not believed: %s", at)
		_, err = LoadLastKnownGood(db, snapshotTestKey, "", 72*time.Hour, now)
		r := refusal(t, err)
		assert.Contains(t, r.Reason, "longer than EDGE_LAST_KNOWN_GOOD_MAX_AGE")
		assert.Contains(t, r.Reason, "100h", "the age falls back to applied_at")

		// A row with no tag at all (or one written under another key).
		require.NoError(t, db.Model(&database.SyncState{}).Where("topic = ?", controlContactTopic).
			Update("sequence_number", 0).Error)
		_, ok, _ = LastControlContact(db)
		assert.False(t, ok)
		other, err := newSnapshotSealer(snapshotOtherKey)
		require.NoError(t, err)
		recent := now.Add(-time.Hour).UTC().Truncate(time.Second)
		require.NoError(t, db.Model(&database.SyncState{}).Where("topic = ?", controlContactTopic).
			Updates(map[string]interface{}{"last_sync_at": recent, "sequence_number": controlContactTag(other.contactKey, recent)}).Error)
		_, err = LoadLastKnownGood(db, snapshotTestKey, "", 72*time.Hour, now)
		assert.Contains(t, refusal(t, err).Reason, "longer than EDGE_LAST_KNOWN_GOOD_MAX_AGE")

		// An authentic recent contact is believed.
		setContact(t, db, recent)
		lkg, err := LoadLastKnownGood(db, snapshotTestKey, "", 72*time.Hour, now)
		require.NoError(t, err)
		assert.True(t, lkg.LastContact.Equal(recent))
	})
}

// The edge's clock was set back: a recorded contact that lies in the future
// cannot make the configuration look fresh. It is ignored, the age is
// measured from applied_at, and a snapshot applied "in the future" is
// refused; a time only slightly ahead gives an age of zero, never a negative
// one.
func TestLoadLastKnownGood_ClockSetBack(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	t.Cleanup(func() { clearInstalled(t) })
	s := persistingSync(t, db, "")
	applied := time.Now()
	require.NoError(t, s.SyncConfiguration(lkgSnapshot(applied.Add(-time.Minute), "c1")))

	// The last contact was 200 hours after the snapshot was applied. The
	// clock is then set back to 100 hours after it.
	setContact(t, db, applied.Add(200*time.Hour))
	now := applied.Add(100 * time.Hour)
	_, err := LoadLastKnownGood(db, snapshotTestKey, "", 72*time.Hour, now)
	r := refusal(t, err)
	assert.Contains(t, r.Reason, "longer than EDGE_LAST_KNOWN_GOOD_MAX_AGE", "a contact in the future does not pass as fresh")
	assert.Contains(t, r.Reason, "100h")

	// With applied_at recent enough the edge starts, measured from it.
	lkg, err := LoadLastKnownGood(db, snapshotTestKey, "", 72*time.Hour, applied.Add(10*time.Hour))
	require.NoError(t, err)
	assert.WithinDuration(t, applied, lkg.LastContact, 5*time.Second, "not the future contact")

	// A contact a minute ahead (ordinary clock drift) is an age of zero.
	setContact(t, db, applied.Add(10*time.Hour+time.Minute))
	lkg, err = LoadLastKnownGood(db, snapshotTestKey, "", 72*time.Hour, applied.Add(10*time.Hour))
	require.NoError(t, err)
	assert.False(t, lkg.LastContact.After(applied.Add(10*time.Hour)), "the age is never negative")

	// The clock set back to before the snapshot was applied: no age can be
	// worked out at all.
	_, err = LoadLastKnownGood(db, snapshotTestKey, "", 72*time.Hour, applied.Add(-24*time.Hour))
	assert.Contains(t, refusal(t, err).Reason, "in the future by this edge's clock")
}
