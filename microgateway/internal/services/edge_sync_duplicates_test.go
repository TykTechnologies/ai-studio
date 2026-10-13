// internal/services/edge_sync_duplicates_test.go
package services

import (
	"regexp"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func dupSnapshot(version, checksum string) *pb.ConfigurationSnapshot {
	now := timestamppb.Now()
	return &pb.ConfigurationSnapshot{
		Version:      version,
		Checksum:     checksum,
		SnapshotTime: now,
		Llms: []*pb.LLMConfig{
			{Id: 1, Name: "GPT", Slug: "gpt", Vendor: "openai", IsActive: true, FilterIds: []uint32{1}, PluginIds: []uint32{1}, CreatedAt: now, UpdatedAt: now},
			{Id: 2, Name: "New LLM", Slug: "new-llm", Vendor: "openai", IsActive: true, CreatedAt: now, UpdatedAt: now},
		},
		Apps: []*pb.AppConfig{
			{Id: 1, Name: "App", IsActive: true, LlmIds: []uint32{1, 2}, ToolIds: []uint32{2, 3, 4}, CreatedAt: now, UpdatedAt: now},
		},
		Filters: []*pb.FilterConfig{{Id: 1, Name: "guard", Script: "return {block: false}", IsActive: true, LlmIds: []uint32{1}, CreatedAt: now, UpdatedAt: now}},
		Plugins: []*pb.PluginConfig{{Id: 1, Name: "p", Command: "file:///bin/p", HookType: "post_auth", IsActive: true, LlmIds: []uint32{1}, CreatedAt: now, UpdatedAt: now}},
		Tools: []*pb.ToolConfig{
			{Id: 2, Name: "Page fetcher", Slug: "page-fetcher", ToolType: "REST", IsActive: true, CreatedAt: now, UpdatedAt: now},
			{Id: 3, Name: "page fetcher", Slug: "page-fetcher", ToolType: "REST", IsActive: true, CreatedAt: now, UpdatedAt: now},
			{Id: 4, Name: "Other", Slug: "other", ToolType: "REST", IsActive: true, CreatedAt: now, UpdatedAt: now},
		},
	}
}

func toolIDsIn(t *testing.T, svc *EdgeSyncService) []uint {
	t.Helper()
	var ids []uint
	require.NoError(t, svc.db.Model(&database.Tool{}).Order("id").Pluck("id", &ids).Error)
	return ids
}

// Two tools with one slug: the later tool is left out on its own and the
// rest of the snapshot is applied.
func TestEdgeSync_DuplicateToolSlugDoesNotBlockTheRest(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	svc := NewEdgeSyncService(db, "")

	report, err := svc.SyncConfigurationReport(dupSnapshot("1", "sum1"))
	require.NoError(t, err)
	assert.Equal(t, []string{`tool 3 left out: slug "page-fetcher" is already used by tool 2`}, report.Skipped)

	assert.Equal(t, []uint{2, 4}, toolIDsIn(t, svc))
	var llms []database.LLM
	require.NoError(t, db.Order("id").Find(&llms).Error)
	require.Len(t, llms, 2, "the new LLM landed")

	var grants []uint
	require.NoError(t, db.Model(&database.AppTool{}).Where("app_id = ?", 1).Order("tool_id").Pluck("tool_id", &grants).Error)
	assert.Equal(t, []uint{2, 4}, grants, "the left-out tool's grant goes with it")
}

// An LLM left out for its slug never takes a protection from the LLM that
// keeps it: its filters and plugins are all stored and attached.
func TestEdgeSync_DuplicateLLMSlugKeepsTheKeptLLMGuarded(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	svc := NewEdgeSyncService(db, "")
	snap := dupSnapshot("1", "sum1")
	now := timestamppb.Now()
	snap.Llms = append(snap.Llms, &pb.LLMConfig{Id: 9, Name: "gpt", Slug: "gpt", Vendor: "openai", IsActive: true, CreatedAt: now, UpdatedAt: now})
	snap.Apps = append(snap.Apps, &pb.AppConfig{Id: 2, Name: "Only nine", IsActive: true, LlmIds: []uint32{9}, CreatedAt: now, UpdatedAt: now})

	report, err := svc.SyncConfigurationReport(snap)
	require.NoError(t, err)
	assert.Contains(t, report.Skipped, `LLM 9 left out: slug "gpt" is already used by LLM 1`)

	var filterLinks, pluginLinks int64
	require.NoError(t, db.Model(&database.LLMFilter{}).Where("llm_id = ? AND filter_id = ?", 1, 1).Count(&filterLinks).Error)
	require.NoError(t, db.Model(&database.LLMPlugin{}).Where("llm_id = ? AND plugin_id = ?", 1, 1).Count(&pluginLinks).Error)
	assert.Equal(t, int64(1), filterLinks, "the kept LLM keeps its filter")
	assert.Equal(t, int64(1), pluginLinks, "the kept LLM keeps its plugin")

	var n int64
	require.NoError(t, db.Model(&database.LLM{}).Where("id = ?", 9).Count(&n).Error)
	assert.Zero(t, n, "the left-out LLM is not stored")
	require.NoError(t, db.Model(&database.AppLLM{}).Where("app_id = ?", 2).Count(&n).Error)
	assert.Zero(t, n, "an App granted only the left-out LLM gets no grant")
}

// An LLM attached to a filter the snapshot does not carry (a filter in
// another namespace, for one) is not stored: it does not serve unguarded.
func TestEdgeSync_LLMWithMissingFilterIsNotServed(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	svc := NewEdgeSyncService(db, "")
	snap := dupSnapshot("1", "sum1")
	snap.Llms[1].FilterIds = []uint32{99}

	report, err := svc.SyncConfigurationReport(snap)
	require.NoError(t, err)
	assert.Contains(t, report.Skipped, "LLM 2 left out: its filter 99 is not in the configuration, and it does not run without it")

	var n int64
	require.NoError(t, db.Model(&database.LLM{}).Where("id = ?", 2).Count(&n).Error)
	assert.Zero(t, n)
	require.NoError(t, db.Model(&database.AppLLM{}).Where("llm_id = ?", 2).Count(&n).Error)
	assert.Zero(t, n)
	require.NoError(t, db.Model(&database.LLM{}).Where("id = ?", 1).Count(&n).Error)
	assert.Equal(t, int64(1), n, "the guarded LLM is served")
}

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Heartbeats report what was applied: the hub's checksum for a whole
// snapshot, a checksum of its own (same length, the hub stores 64
// characters) for one applied without entries, and the previous one when a
// sync fails.
func TestEdgeSync_ReportedConfig(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	svc := NewEdgeSyncService(db, "")
	hubSum := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	_, _, ok := svc.ReportedConfig()
	assert.False(t, ok, "nothing to report before a sync")

	clean := dupSnapshot("1", hubSum)
	clean.Tools = clean.Tools[:1]
	_, err := svc.SyncConfigurationReport(clean)
	require.NoError(t, err)
	sum, version, ok := svc.ReportedConfig()
	require.True(t, ok)
	assert.Equal(t, hubSum, sum)
	assert.Equal(t, "1", version)

	_, err = svc.SyncConfigurationReport(dupSnapshot("2", hubSum))
	require.NoError(t, err)
	partial, version, _ := svc.ReportedConfig()
	assert.NotEqual(t, hubSum, partial, "a snapshot applied without entries is not in sync")
	assert.Regexp(t, hex64, partial)
	assert.Equal(t, "2", version)

	// The hub's own omitted entries count too.
	omitted := dupSnapshot("3", hubSum)
	omitted.Tools = omitted.Tools[:1]
	omitted.OmittedEntries = []string{"tool 7 left out: slug \"x\" is already used by tool 6"}
	report, err := svc.SyncConfigurationReport(omitted)
	require.NoError(t, err)
	assert.Equal(t, omitted.OmittedEntries, report.Skipped)
	sum, _, _ = svc.ReportedConfig()
	assert.NotEqual(t, hubSum, sum)

	// A failed sync leaves the last applied configuration reported.
	require.NoError(t, db.Exec("DROP TABLE tools").Error)
	_, err = svc.SyncConfigurationReport(dupSnapshot("4", "bbbb"))
	require.Error(t, err)
	after, version, _ := svc.ReportedConfig()
	assert.Equal(t, sum, after)
	assert.Equal(t, "3", version)
}

type fakeReloadClient struct {
	mu      sync.Mutex
	current *pb.ConfigurationSnapshot
	next    *pb.ConfigurationSnapshot
}

func (f *fakeReloadClient) RequestFullSync() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = f.next
	return nil
}

func (f *fakeReloadClient) GetCurrentConfiguration() *pb.ConfigurationSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current
}

// A push applied without entries reports READY with them, so the hub can
// record them on the push.
func TestEdgeReload_ReadyCarriesSkippedEntries(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the reload handler's pull delay")
	}
	db := setupToolsSyncTestDB(t)
	svc := NewEdgeSyncService(db, "")
	client := &fakeReloadClient{current: &pb.ConfigurationSnapshot{Version: "0"}, next: dupSnapshot("1", "sum")}
	var mu sync.Mutex
	var sent []*pb.ConfigurationReloadResponse
	handler := NewEdgeReloadHandler(client, svc, db, "edge-1", func(r *pb.ConfigurationReloadResponse) {
		mu.Lock()
		sent = append(sent, r)
		mu.Unlock()
	}, nil)

	handler.HandleReloadRequest(&pb.ConfigurationReloadRequest{OperationId: "op-1"})

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, sent)
	last := sent[len(sent)-1]
	assert.Equal(t, pb.ReloadPhase_READY, last.Phase)
	assert.True(t, last.Success)
	assert.Equal(t, []string{`tool 3 left out: slug "page-fetcher" is already used by tool 2`}, last.SkippedEntries)
	assert.Contains(t, last.Message, "except 1 entry it could not store")
}
