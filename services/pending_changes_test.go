package services_test

import (
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Pending-changes preview: created / updated / deleted since the last push,
// global objects visible from every namespace, and no reference point
// before the first push. Timestamps are written explicitly so the test
// does not depend on wall-clock ordering.

func setTimes(t *testing.T, db *gorm.DB, model interface{}, id uint, created, updated time.Time) {
	t.Helper()
	require.NoError(t, db.Model(model).Where("id = ?", id).
		Updates(map[string]interface{}{"created_at": created, "updated_at": updated}).Error)
}

func changeKeys(pc *services.PendingChanges) map[string]string {
	out := map[string]string{}
	for _, c := range pc.Changes {
		out[c.Type+":"+c.Name] = c.Change
	}
	return out
}

func TestPendingChanges(t *testing.T) {
	db := apitest.SetupTestDB(t)
	svc := services.NewSyncStatusService(db)

	push := time.Now().Add(-time.Hour)
	before := push.Add(-time.Hour)
	after := push.Add(10 * time.Minute)

	// Namespaced objects in "edge-1", one in another namespace, one global.
	untouched := &models.LLM{Name: "Untouched", Namespace: "edge-1"}
	updated := &models.LLM{Name: "Updated", Namespace: "edge-1"}
	created := &models.App{Name: "Created", Namespace: "edge-1"}
	deleted := &models.Tool{Name: "Deleted", Namespace: "edge-1"}
	elsewhere := &models.Filter{Name: "Elsewhere", Namespace: "edge-2"}
	global := &models.Datasource{Name: "Global"}
	price := &models.ModelPrice{ModelName: "gpt-x", Vendor: "openai"}
	for _, m := range []interface{}{untouched, updated, created, deleted, elsewhere, global, price} {
		require.NoError(t, db.Create(m).Error)
	}
	setTimes(t, db, &models.LLM{}, untouched.ID, before, before)
	setTimes(t, db, &models.LLM{}, updated.ID, before, after)
	setTimes(t, db, &models.App{}, created.ID, after, after)
	setTimes(t, db, &models.Tool{}, deleted.ID, before, before)
	setTimes(t, db, &models.Filter{}, elsewhere.ID, after, after)
	setTimes(t, db, &models.Datasource{}, global.ID, after, after)
	setTimes(t, db, &models.ModelPrice{}, price.ID, before, before)
	require.NoError(t, db.Delete(deleted).Error)

	t.Run("no push yet: everything alive counts as created", func(t *testing.T) {
		pc, err := svc.GetPendingChanges("edge-1")
		require.NoError(t, err)
		assert.Nil(t, pc.Since)
		assert.Nil(t, pc.LastPushAt)
		assert.Equal(t, "edge-1", pc.Namespace)
		keys := changeKeys(pc)
		assert.Equal(t, map[string]string{
			"llm:Untouched":            services.PendingChangeCreated,
			"llm:Updated":              services.PendingChangeCreated,
			"app:Created":              services.PendingChangeCreated,
			"datasource:Global":        services.PendingChangeCreated,
			"model_price:openai/gpt-x": services.PendingChangeCreated,
		}, keys, "deleted rows and other namespaces are absent")
		assert.Equal(t, len(keys), pc.Total)
	})

	require.NoError(t, models.MarkNamespacePushed(db, "edge-1", push))

	t.Run("since last push", func(t *testing.T) {
		pc, err := svc.GetPendingChanges("edge-1")
		require.NoError(t, err)
		require.NotNil(t, pc.Since)
		assert.WithinDuration(t, push, *pc.Since, time.Second)
		require.NotNil(t, pc.LastPushAt)
		assert.Equal(t, map[string]string{
			"llm:Updated":       services.PendingChangeUpdated,
			"app:Created":       services.PendingChangeCreated,
			"tool:Deleted":      services.PendingChangeDeleted,
			"datasource:Global": services.PendingChangeCreated,
		}, changeKeys(pc))
		assert.Equal(t, 4, pc.Total)
		// Newest first.
		for i := 1; i < len(pc.Changes); i++ {
			assert.False(t, pc.Changes[i].At.After(pc.Changes[i-1].At), "ordered by at desc")
		}
		var del services.PendingChange
		for _, c := range pc.Changes {
			if c.Type == "tool" {
				del = c
			}
		}
		assert.Equal(t, deleted.ID, del.ID)
		assert.True(t, del.At.After(push))
	})

	t.Run("global namespace sees only global objects", func(t *testing.T) {
		require.NoError(t, models.MarkNamespacePushed(db, "", push))
		pc, err := svc.GetPendingChanges("global")
		require.NoError(t, err)
		assert.Equal(t, "", pc.Namespace)
		assert.Equal(t, map[string]string{"datasource:Global": services.PendingChangeCreated}, changeKeys(pc))
	})

	t.Run("a fresh push empties the list", func(t *testing.T) {
		require.NoError(t, models.MarkNamespacePushed(db, "edge-1", time.Now()))
		pc, err := svc.GetPendingChanges("edge-1")
		require.NoError(t, err)
		assert.Equal(t, 0, pc.Total)
		assert.Equal(t, []services.PendingChange{}, pc.Changes, "[] not null")
	})
}

func TestMarkNamespacePushed_UpdatesOrCreatesRow(t *testing.T) {
	db := apitest.SetupTestDB(t)
	now := time.Now()

	// Existing row keeps its checksum and gains the stamp.
	existing := &models.NamespaceSyncStatus{Namespace: "ns", ExpectedChecksum: "abc", ConfigVersion: "1", LastConfigChange: now}
	require.NoError(t, existing.Upsert(db))
	require.NoError(t, models.MarkNamespacePushed(db, "ns", now))
	var row models.NamespaceSyncStatus
	require.NoError(t, row.GetByNamespace(db, "ns"))
	assert.Equal(t, "abc", row.ExpectedChecksum)
	require.NotNil(t, row.LastPushAt)
	assert.WithinDuration(t, now, *row.LastPushAt, time.Second)

	// The snapshot upsert does not clear the stamp.
	existing.ExpectedChecksum = "def"
	require.NoError(t, existing.Upsert(db))
	require.NoError(t, row.GetByNamespace(db, "ns"))
	assert.Equal(t, "def", row.ExpectedChecksum)
	assert.NotNil(t, row.LastPushAt)

	// No row yet: one is created.
	require.NoError(t, models.MarkNamespacePushed(db, "new-ns", now))
	var created models.NamespaceSyncStatus
	require.NoError(t, created.GetByNamespace(db, "new-ns"))
	require.NotNil(t, created.LastPushAt)

	// And the sync summary carries it.
	summary, err := services.NewSyncStatusService(db).GetNamespaceSyncSummary()
	require.NoError(t, err)
	for _, s := range summary {
		assert.NotNil(t, s.LastPushAt, s.Namespace)
	}
}

func TestReload_StampsLastPush(t *testing.T) {
	db := apitest.SetupTestDB(t)
	edgeService := services.NewEdgeService(db)
	nsService := services.NewNamespaceService(db, edgeService)

	edge := &models.EdgeInstance{EdgeID: "edge-a", Namespace: "default", Status: models.EdgeStatusConnected}
	require.NoError(t, db.Create(edge).Error)

	_, err := nsService.TriggerNamespaceReload("nowhere", "admin@test")
	assert.Error(t, err, "no edges, no push")
	var row models.NamespaceSyncStatus
	assert.ErrorIs(t, row.GetByNamespace(db, "nowhere"), gorm.ErrRecordNotFound)

	_, err = nsService.TriggerNamespaceReload("default", "admin@test")
	require.NoError(t, err)
	var afterNamespace models.NamespaceSyncStatus
	require.NoError(t, afterNamespace.GetByNamespace(db, "default"))
	require.NotNil(t, afterNamespace.LastPushAt)
	first := *afterNamespace.LastPushAt

	time.Sleep(5 * time.Millisecond)
	_, err = nsService.TriggerEdgeReload("edge-a", "admin@test")
	require.NoError(t, err)
	var afterEdge models.NamespaceSyncStatus
	require.NoError(t, afterEdge.GetByNamespace(db, "default"))
	assert.True(t, afterEdge.LastPushAt.After(first), "a single-edge reload also counts as a push")
}

func TestEnsurePendingChangeIndexes_IsIdempotent(t *testing.T) {
	db := apitest.SetupTestDB(t)
	require.NoError(t, services.EnsurePendingChangeIndexes(db))
	require.NoError(t, services.EnsurePendingChangeIndexes(db), "second run is a no-op")
	assert.True(t, db.Migrator().HasIndex(&models.LLM{}, "idx_llms_ns_updated_at"), "namespaced tables index (namespace, updated_at)")
	assert.True(t, db.Migrator().HasIndex(&models.AccessToken{}, "idx_access_tokens_updated_at"), "global tables index updated_at alone")
}

// A table with more changes than the cap returns the newest ones and still
// reports the true total, without loading every row.
func TestPendingChanges_CapsPerSourceAndKeepsTotal(t *testing.T) {
	db := apitest.SetupTestDB(t)
	svc := services.NewSyncStatusService(db)

	push := time.Now().Add(-time.Hour)
	require.NoError(t, models.MarkNamespacePushed(db, "edge-1", push))

	const extra = 5
	for i := 0; i < 200+extra; i++ {
		llm := &models.LLM{Name: "llm", Namespace: "edge-1"}
		require.NoError(t, db.Create(llm).Error)
		// Older ids changed earlier; the newest id is the newest change.
		at := push.Add(time.Duration(i+1) * time.Second)
		setTimes(t, db, &models.LLM{}, llm.ID, at, at)
	}
	// One deleted tool, newer than every LLM, must survive the cap.
	tool := &models.Tool{Name: "Gone", Namespace: "edge-1"}
	require.NoError(t, db.Create(tool).Error)
	setTimes(t, db, &models.Tool{}, tool.ID, push.Add(-time.Hour), push.Add(-time.Hour))
	require.NoError(t, db.Delete(tool).Error)

	pc, err := svc.GetPendingChanges("edge-1")
	require.NoError(t, err)
	assert.Equal(t, 200+extra+1, pc.Total)
	require.Len(t, pc.Changes, 200)
	assert.Equal(t, "tool", pc.Changes[0].Type, "the newest change leads")
	assert.Equal(t, services.PendingChangeDeleted, pc.Changes[0].Change)
	for i := 1; i < len(pc.Changes); i++ {
		assert.False(t, pc.Changes[i].At.After(pc.Changes[i-1].At), "ordered by at desc")
	}
	// The oldest `extra` LLM changes are the ones dropped.
	oldest := pc.Changes[len(pc.Changes)-1]
	assert.True(t, oldest.At.After(push.Add(extra*time.Second)), "the newest 199 LLM changes are kept")
}
