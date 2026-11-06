package services

import (
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// orphanSnapshot is what a hub sent while it still listed the attachments
// of a deleted LLM: plugin 1 on LLM 1 and on LLM 51, which the snapshot
// does not carry (midsommar-clean-26ut).
func orphanSnapshot(at time.Time) *pb.ConfigurationSnapshot {
	snap := snapshotAt(at, "", []*pb.LLMConfig{syncLLM(1, "live", at)}, nil)
	snap.Plugins = []*pb.PluginConfig{{
		Id: 1, Name: "audit", Command: "file:///plugins/audit", HookType: "post_auth", IsActive: true,
		LlmIds: []uint32{1, 51}, CreatedAt: timestamppb.New(at), UpdatedAt: timestamppb.New(at),
	}}
	return snap
}

// An edge restarting onto its persisted SQLite re-applies the hub's
// snapshot: the attachment to an LLM it never stored must not fail that.
func TestReapplyOntoPersistedDBWithOrphanAttachment(t *testing.T) {
	for _, ns := range []string{"", "team-a"} {
		t.Run("namespace "+ns, func(t *testing.T) {
			db := setupEdgeSyncTestDB(t)
			now := time.Now()
			require.NoError(t, NewEdgeSyncService(db, ns).SyncConfiguration(orphanSnapshot(now)), "first start")

			// Restart: a new process, a new snapshot (new version), the same
			// database.
			require.NoError(t, NewEdgeSyncService(db, ns).SyncConfiguration(orphanSnapshot(now.Add(time.Minute))), "restart")
			require.NoError(t, NewEdgeSyncService(db, ns).SyncConfiguration(orphanSnapshot(now.Add(2*time.Minute))), "and again")

			var rows []database.LLMPlugin
			require.NoError(t, db.Order("llm_id").Find(&rows).Error)
			require.Len(t, rows, 2, "one row per attachment, never duplicated")
			assert.Equal(t, uint(1), rows[0].LLMID)

			// The hub no longer sends the deleted LLM's attachment: the
			// edge drops it.
			fixed := orphanSnapshot(now.Add(3 * time.Minute))
			fixed.Plugins[0].LlmIds = []uint32{1}
			require.NoError(t, NewEdgeSyncService(db, ns).SyncConfiguration(fixed))
			require.NoError(t, db.Find(&rows).Error)
			require.Len(t, rows, 1)
			assert.Equal(t, uint(1), rows[0].LLMID)
		})
	}
}

// The same for filters: llm_filters rows of a filter whose LLM the edge
// never stored are cleared with the filter.
func TestReapplyFilterAttachmentToUnknownLLM(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	now := time.Now()
	snap := func(at time.Time) *pb.ConfigurationSnapshot {
		s := snapshotAt(at, "", []*pb.LLMConfig{syncLLM(1, "live", at)}, nil)
		s.Filters = []*pb.FilterConfig{{Id: 7, Name: "pii", Script: "x", IsActive: true, LlmIds: []uint32{51},
			CreatedAt: timestamppb.New(at), UpdatedAt: timestamppb.New(at)}}
		return s
	}
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(snap(now)))
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(snap(now.Add(time.Minute))))
	var n int64
	require.NoError(t, db.Model(&database.LLMFilter{}).Count(&n).Error)
	assert.Equal(t, int64(1), n)
}

// configClient hands out one snapshot, as the edge client does after a
// pull.
type configClient struct {
	mu     sync.Mutex
	config *pb.ConfigurationSnapshot
}

func (c *configClient) RequestFullSync() error { return nil }

func (c *configClient) GetCurrentConfiguration() *pb.ConfigurationSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.config
}

// A snapshot the edge received but could not apply must not count as up to
// date: the push applies it again and, failing again, reports FAILED, so the
// hub records a failed push instead of "succeeded".
func TestReloadReportsFailureForUnappliedSnapshot(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	syncer := NewEdgeSyncService(db, "team-a")
	snap := orphanSnapshot(time.Now())
	client := &configClient{config: snap}

	// Applying fails (here: the join table is gone).
	require.NoError(t, db.Exec("DROP TABLE llm_plugins").Error)
	require.Error(t, syncer.SyncConfiguration(snap), "the edge's own apply on receipt fails")

	var mu sync.Mutex
	var last *pb.ConfigurationReloadResponse
	h := NewEdgeReloadHandler(client, syncer, db, "edge-a", func(r *pb.ConfigurationReloadResponse) {
		mu.Lock()
		defer mu.Unlock()
		last = r
	}, nil)
	h.HandleReloadRequest(&pb.ConfigurationReloadRequest{OperationId: "push-1"})
	mu.Lock()
	require.NotNil(t, last)
	assert.Equal(t, pb.ReloadPhase_FAILED, last.Phase, "not READY: the snapshot was never applied")
	assert.Contains(t, last.Message, "Failed to update SQLite")
	mu.Unlock()

	// Once applying works, the same snapshot is applied and the push is READY.
	require.NoError(t, database.Migrate(db))
	h.HandleReloadRequest(&pb.ConfigurationReloadRequest{OperationId: "push-2"})
	mu.Lock()
	assert.Equal(t, pb.ReloadPhase_READY, last.Phase)
	mu.Unlock()
	_, applied, ok := syncer.ReportedConfig()
	assert.True(t, ok)
	assert.Equal(t, snap.Version, applied)
}
