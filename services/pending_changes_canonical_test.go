package services_test

import (
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The UI asks for "default", the API doc says "global" (or nothing), and the
// edges compare against the row registration keyed; all three must read the
// same row, or the preview says "never pushed" while the edge reads Synced.

func TestPendingChanges_DefaultNamespaceSpellingsShareOneRow(t *testing.T) {
	db := apitest.SetupTestDB(t)
	svc := services.NewSyncStatusService(db)
	push := time.Now().Add(-time.Hour)

	require.NoError(t, db.Create(&models.LLM{Name: "Global LLM", Namespace: ""}).Error)
	require.NoError(t, db.Create(&models.App{Name: "Default App", Namespace: "default"}).Error)
	// Recorded the way TriggerEdgeReload records it: under the edge's stored namespace.
	require.NoError(t, models.MarkNamespacePushed(db, "default", push))

	var reference *services.PendingChanges
	for _, spelling := range []string{"default", "", "global"} {
		pc, err := svc.GetPendingChanges(spelling)
		require.NoError(t, err, spelling)
		assert.Equal(t, models.DefaultNamespace, pc.Namespace, spelling)
		require.NotNil(t, pc.LastPushAt, "%q must see the push", spelling)
		assert.WithinDuration(t, push, *pc.LastPushAt, time.Second, spelling)
		require.NotNil(t, pc.Since, spelling)
		assert.Equal(t, services.PendingBaselinePush, pc.Baseline, spelling)
		if reference == nil {
			reference = pc
			continue
		}
		assert.Equal(t, reference.Total, pc.Total, spelling)
		assert.Equal(t, changeKeys(reference), changeKeys(pc), "%q sees the same object set", spelling)
	}
}

// A push for an edge that registered with EDGE_NAMESPACE unset (stored as
// "default", or "" on a legacy row) is what the "default" preview reports.
func TestPendingChanges_EdgeReloadStampsThePreview(t *testing.T) {
	db := apitest.SetupTestDB(t)
	edgeService := services.NewEdgeService(db)
	nsService := services.NewNamespaceService(db, edgeService)
	svc := services.NewSyncStatusService(db)

	require.NoError(t, db.Create(&models.EdgeInstance{EdgeID: "edge-legacy", Namespace: "", Status: models.EdgeStatusConnected}).Error)
	require.NoError(t, db.Create(&models.EdgeInstance{EdgeID: "edge-default", Namespace: "default", Status: models.EdgeStatusConnected}).Error)

	_, err := nsService.TriggerEdgeReload("edge-legacy", "admin@test")
	require.NoError(t, err)
	pc, err := svc.GetPendingChanges("default")
	require.NoError(t, err)
	require.NotNil(t, pc.LastPushAt, "preview for default sees the push recorded for the legacy edge")
	first := *pc.LastPushAt

	time.Sleep(5 * time.Millisecond)
	_, err = nsService.TriggerNamespaceReload("global", "admin@test")
	require.NoError(t, err, "a global push finds the edges stored under default")
	pc, err = svc.GetPendingChanges("")
	require.NoError(t, err)
	require.NotNil(t, pc.LastPushAt)
	assert.True(t, pc.LastPushAt.After(first))

	var rows int64
	require.NoError(t, db.Model(&models.NamespaceSyncStatus{}).Count(&rows).Error)
	assert.Equal(t, int64(1), rows)

	summary, edges, err := svc.GetNamespaceSyncStatus("global")
	require.NoError(t, err)
	assert.Equal(t, models.DefaultNamespace, summary.Namespace)
	assert.NotNil(t, summary.LastPushAt)
	assert.Len(t, edges, 2)
}

// A database from before last_push_at existed: the namespace has a checksum
// and an edge that loaded it, but no push stamp. The preview measures from
// the edge's ack instead of calling everything "never pushed".
func TestPendingChanges_EdgeAckBaselineWhenNoPushRecorded(t *testing.T) {
	db := apitest.SetupTestDB(t)
	svc := services.NewSyncStatusService(db)

	ack := time.Now().Add(-time.Hour)
	before := ack.Add(-time.Hour)
	after := ack.Add(10 * time.Minute)
	old := &models.LLM{Name: "Old", Namespace: ""}
	fresh := &models.App{Name: "Fresh", Namespace: "default"}
	require.NoError(t, db.Create(old).Error)
	require.NoError(t, db.Create(fresh).Error)
	setTimes(t, db, &models.LLM{}, old.ID, before, before)
	setTimes(t, db, &models.App{}, fresh.ID, after, after)

	row := &models.NamespaceSyncStatus{Namespace: "default", ExpectedChecksum: "abc", ConfigVersion: "1", LastConfigChange: before}
	require.NoError(t, row.Upsert(db))

	t.Run("no edge holds the checksum: still never pushed", func(t *testing.T) {
		pc, err := svc.GetPendingChanges("default")
		require.NoError(t, err)
		assert.Nil(t, pc.Since)
		assert.Nil(t, pc.LastPushAt)
		assert.Equal(t, services.PendingBaselineNone, pc.Baseline)
		assert.Equal(t, 2, pc.Total)
	})

	require.NoError(t, db.Create(&models.EdgeInstance{EdgeID: "edge-1", Namespace: "default", Status: models.EdgeStatusConnected,
		SyncStatus: models.EdgeSyncStatusInSync, LoadedChecksum: "abc", LastSyncAck: &ack}).Error)

	t.Run("an in-sync edge's ack is the reference point", func(t *testing.T) {
		pc, err := svc.GetPendingChanges("default")
		require.NoError(t, err)
		require.NotNil(t, pc.Since)
		assert.WithinDuration(t, ack, *pc.Since, time.Second)
		assert.Nil(t, pc.LastPushAt, "no push is invented")
		assert.Equal(t, services.PendingBaselineEdgeAck, pc.Baseline)
		assert.Equal(t, map[string]string{"app:Fresh": services.PendingChangeCreated}, changeKeys(pc))
		assert.Equal(t, 1, pc.Total)
	})

	t.Run("an edge holding another checksum does not count", func(t *testing.T) {
		require.NoError(t, db.Model(&models.EdgeInstance{}).Where("edge_id = ?", "edge-1").Update("loaded_checksum", "zzz").Error)
		pc, err := svc.GetPendingChanges("default")
		require.NoError(t, err)
		assert.Nil(t, pc.Since)
		assert.Equal(t, services.PendingBaselineNone, pc.Baseline)
	})

	t.Run("a recorded push wins over the ack", func(t *testing.T) {
		push := time.Now()
		require.NoError(t, models.MarkNamespacePushed(db, "default", push))
		pc, err := svc.GetPendingChanges("default")
		require.NoError(t, err)
		assert.Equal(t, services.PendingBaselinePush, pc.Baseline)
		assert.WithinDuration(t, push, *pc.Since, time.Second)
		assert.Equal(t, 0, pc.Total)
	})
}
