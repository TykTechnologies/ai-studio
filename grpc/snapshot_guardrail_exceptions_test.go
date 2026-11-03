package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

func createGuardrailException(t *testing.T, db *gorm.DB, appID, filterID uint, channel, action, status string) *models.GuardrailException {
	t.Helper()
	now := time.Now()
	e := &models.GuardrailException{
		AppID: appID, FilterID: filterID, Detector: "prompt_attack", Channel: channel, Action: action,
		Status: status, Justification: "test vectors", ExpiresAfterDays: 30,
	}
	if status == models.GuardrailExceptionApproved {
		e.ApprovedAt = &now
	}
	require.NoError(t, db.Create(e).Error)
	return e
}

func snapshotExceptionIDs(snap *pb.ConfigurationSnapshot) []uint32 {
	var out []uint32
	for _, e := range snap.GuardrailExceptions {
		out = append(out, e.Id)
	}
	return out
}

func guardrailFilter(t *testing.T, db *gorm.DB, namespace string) *models.Filter {
	t.Helper()
	f := &models.Filter{Name: "Wolf " + namespace, Kind: models.FilterKindGuardrail, Namespace: namespace, Config: models.JSONMap{"provider": "http", "on_detect": "block"}}
	require.NoError(t, db.Create(f).Error)
	return f
}

// The snapshot carries the approved, unlapsed exceptions within their App's
// floor, each with its App's capability; proposed, retired, lapsed and
// floor-violating ones stay on the hub.
func TestSnapshot_GuardrailExceptionsIncludedAndExcluded(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	apps := createTestApps(db, "", llms)
	filter := guardrailFilter(t, db, "")
	require.NoError(t, db.Model(&models.App{}).Where("id = ?", apps[0].ID).Update("guardrail_capability", "read_only").Error)

	approved := createGuardrailException(t, db, apps[0].ID, filter.ID, "text_user", "log", models.GuardrailExceptionApproved)
	createGuardrailException(t, db, apps[0].ID, filter.ID, "text_system", "log", models.GuardrailExceptionProposed)
	createGuardrailException(t, db, apps[0].ID, filter.ID, "tool_call", "log", models.GuardrailExceptionRetired)
	lapsed := createGuardrailException(t, db, apps[0].ID, filter.ID, "tool_definition", "log", models.GuardrailExceptionApproved)
	require.NoError(t, db.Model(lapsed).Update("approved_at", time.Now().Add(-40*24*time.Hour)).Error)
	// apps[1] is unattested (acting): a log exception on text is below its floor.
	createGuardrailException(t, db, apps[1].ID, filter.ID, "text_user", "log", models.GuardrailExceptionApproved)

	snap, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	require.Equal(t, []uint32{uint32(approved.ID)}, snapshotExceptionIDs(snap))
	got := snap.GuardrailExceptions[0]
	assert.Equal(t, uint32(apps[0].ID), got.AppId)
	assert.Equal(t, uint32(filter.ID), got.FilterId)
	assert.Equal(t, "prompt_attack", got.Detector)
	assert.Equal(t, "text_user", got.Channel)
	assert.Equal(t, "log", got.Action)
	assert.Equal(t, "read_only", got.AppCapability)
	for _, a := range snap.Apps {
		if a.Id == uint32(apps[0].ID) {
			assert.Equal(t, "read_only", a.GuardrailCapability)
		}
	}
}

// Approving or retiring an exception, and attesting its App's capability,
// change the checksum, so edges become pending; a hit does not. With none
// the checksum is what it was before exceptions existed.
func TestSnapshot_GuardrailExceptionsChangeTheChecksum(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	apps := createTestApps(db, "", llms)
	filter := guardrailFilter(t, db, "")
	setCapability := func(c string) {
		require.NoError(t, db.Model(&models.App{}).Where("id = ?", apps[0].ID).Update("guardrail_capability", c).Error)
	}
	setCapability("read_only")

	checksum := func() string {
		snap, err := server.getConfigurationSnapshot("default")
		require.NoError(t, err)
		return snap.Checksum
	}
	none := checksum()

	e := createGuardrailException(t, db, apps[0].ID, filter.ID, "text_user", "quarantine", models.GuardrailExceptionProposed)
	assert.Equal(t, none, checksum(), "a proposed exception is not in the snapshot")
	require.NoError(t, db.Model(e).Updates(map[string]any{"status": models.GuardrailExceptionApproved, "approved_at": time.Now()}).Error)
	approved := checksum()
	assert.NotEqual(t, none, approved, "approving marks edges pending")

	require.NoError(t, db.Model(e).Updates(map[string]any{"last_hit_at": time.Now(), "hit_count": 5}).Error)
	assert.Equal(t, approved, checksum(), "a hit does not change the snapshot")

	setCapability("read_tools")
	readTools := checksum()
	assert.NotEqual(t, approved, readTools, "attesting the capability marks edges pending")
	setCapability("acting")
	assert.NotEqual(t, readTools, checksum(), "still within the floor, with the new capability")
	setCapability("")
	assert.NotEqual(t, readTools, checksum())

	setCapability("read_only")
	assert.Equal(t, approved, checksum())
	require.NoError(t, db.Model(e).Update("status", models.GuardrailExceptionRetired).Error)
	assert.Equal(t, none, checksum(), "retiring marks edges pending")
}

// Edges report the exceptions that applied in their heartbeats; the hub
// counts those its snapshot for the edge's namespace carries, and nothing
// else an edge claims.
func TestEdgeGuardrailExceptionHitsRecorded(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	apps := createTestApps(db, "", llms)
	global := guardrailFilter(t, db, "")
	for _, a := range apps {
		require.NoError(t, db.Model(&models.App{}).Where("id = ?", a.ID).Update("guardrail_capability", "read_only").Error)
	}
	eu := &models.App{Name: "EU app", IsActive: true, Namespace: "eu", GuardrailCapability: "read_only"}
	require.NoError(t, db.Create(eu).Error)
	euFilter := guardrailFilter(t, db, "eu")

	ok := createGuardrailException(t, db, apps[0].ID, global.ID, "text_user", "log", models.GuardrailExceptionApproved)
	euOnly := createGuardrailException(t, db, eu.ID, euFilter.ID, "text_user", "log", models.GuardrailExceptionApproved)
	proposed := createGuardrailException(t, db, apps[0].ID, global.ID, "text_system", "log", models.GuardrailExceptionProposed)
	lapsed := createGuardrailException(t, db, apps[0].ID, global.ID, "tool_call", "log", models.GuardrailExceptionApproved)
	require.NoError(t, db.Model(lapsed).Update("approved_at", time.Now().Add(-40*24*time.Hour)).Error)
	ids := []uint{ok.ID, euOnly.ID, proposed.ID, lapsed.ID}
	for _, id := range ids {
		edgeExceptionHitWritten.Delete(id)
	}
	t.Cleanup(func() {
		for _, id := range ids {
			edgeExceptionHitWritten.Delete(id)
		}
	})
	hits := func(id uint) int64 {
		var e models.GuardrailException
		require.NoError(t, db.First(&e, id).Error)
		return e.HitCount
	}

	server.recordEdgeGuardrailExceptionHits("edge-default", "default", []uint32{uint32(ok.ID), uint32(euOnly.ID), uint32(proposed.ID), uint32(lapsed.ID), 0, 99999})
	assert.EqualValues(t, 1, hits(ok.ID))
	assert.EqualValues(t, 0, hits(euOnly.ID), "another namespace's exception is not counted")
	assert.EqualValues(t, 0, hits(proposed.ID))
	assert.EqualValues(t, 0, hits(lapsed.ID), "a lapsed exception is not revived by an edge")

	server.recordEdgeGuardrailExceptionHits("edge-eu", "eu", []uint32{uint32(euOnly.ID)})
	assert.EqualValues(t, 1, hits(euOnly.ID))
	server.recordEdgeGuardrailExceptionHits("edge-default", "default", []uint32{uint32(ok.ID)})
	assert.EqualValues(t, 1, hits(ok.ID), "throttled")
}

// .52: an exception that lapses with no hub traffic is lapsed by the
// sweep, the change event recomputes the checksum, and the snapshot no
// longer carries it, so edges become pending and drop it at the push.
func TestSnapshot_SweptExceptionLeavesTheSnapshot(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	apps := createTestApps(db, "", llms)
	filter := guardrailFilter(t, db, "")
	require.NoError(t, db.Model(&models.App{}).Where("id = ?", apps[0].ID).Update("guardrail_capability", "read_only").Error)
	e := createGuardrailException(t, db, apps[0].ID, filter.ID, "text_user", "log", models.GuardrailExceptionApproved)

	snap, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	require.Equal(t, []uint32{uint32(e.ID)}, snapshotExceptionIDs(snap))
	carried := snap.Checksum

	require.NoError(t, db.Model(e).Update("approved_at", time.Now().Add(-31*24*time.Hour)).Error)
	n, err := services.NewService(db).SweepLapsedGuardrailExceptions(time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	snap, err = server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	assert.Empty(t, snapshotExceptionIDs(snap))
	assert.NotEqual(t, carried, snap.Checksum, "edges become pending")
	var got models.GuardrailException
	require.NoError(t, db.First(&got, e.ID).Error)
	assert.Equal(t, models.GuardrailExceptionProposed, got.Status)
}

// .58: the snapshot carries each attested app's capability in a list that
// counts in the checksum, so attesting, lowering or resetting it marks
// edges pending even for an app with no exceptions.
func TestSnapshot_GuardrailCapabilitiesChangeTheChecksum(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	apps := createTestApps(db, "", llms)
	setCapability := func(c string) {
		require.NoError(t, db.Model(&models.App{}).Where("id = ?", apps[0].ID).Update("guardrail_capability", c).Error)
	}
	snapshot := func() *pb.ConfigurationSnapshot {
		snap, err := server.getConfigurationSnapshot("default")
		require.NoError(t, err)
		return snap
	}

	unattested := snapshot()
	assert.Empty(t, unattested.GuardrailCapabilities, "unattested apps are not listed")
	assert.Empty(t, unattested.GuardrailExceptions)

	setCapability("read_only")
	readOnly := snapshot()
	require.Len(t, readOnly.GuardrailCapabilities, 1)
	assert.Equal(t, uint32(apps[0].ID), readOnly.GuardrailCapabilities[0].AppId)
	assert.Equal(t, "read_only", readOnly.GuardrailCapabilities[0].Capability)
	assert.NotEqual(t, unattested.Checksum, readOnly.Checksum, "attesting marks edges pending")

	setCapability("acting")
	assert.NotEqual(t, readOnly.Checksum, snapshot().Checksum, "lowering marks edges pending")
	setCapability("")
	assert.Equal(t, unattested.Checksum, snapshot().Checksum, "a reset marks edges pending")
}
