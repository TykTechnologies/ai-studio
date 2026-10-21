package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// recordingProcessor stands in for the Enterprise pipeline and records what
// it was built with.
type recordingProcessor struct {
	globals   agenttrace.ContentGlobals
	overrides agenttrace.ContentOverridesFunc
}

func (recordingProcessor) Process(*agenttrace.Event) {}

func registerRecordingFactory(t *testing.T) {
	t.Helper()
	agenttrace.RegisterContentProcessorFactory(func(g agenttrace.ContentGlobals, o agenttrace.ContentOverridesFunc) agenttrace.ContentProcessor {
		return &recordingProcessor{globals: g, overrides: o}
	})
	t.Cleanup(func() {
		agenttrace.RegisterContentProcessorFactory(nil)
		agenttrace.SetContentProcessor(nil)
	})
}

// The hub's content settings arrive in the snapshot and configure the edge
// pipeline, with App and resource overrides read from the synced rows. A
// snapshot without them (tracing off on the hub) removes the pipeline, so
// the edge captures no content.
func TestEdgeSync_InstallsContentPipeline(t *testing.T) {
	registerRecordingFactory(t)
	db := setupEdgeSyncTestDB(t)
	sync := NewEdgeSyncService(db, "ns")
	off := `{"enabled":false,"content_mode":"full"}`
	snap := &pb.ConfigurationSnapshot{
		Version: "1", EdgeNamespace: "ns", SnapshotTime: timestamppb.Now(),
		Apps: []*pb.AppConfig{
			{Id: 31, Name: "raised", IsActive: true, Namespace: "ns", Tracing: `{"content_mode":"redacted","max_request_override":"full"}`},
			{Id: 32, Name: "plain", IsActive: true, Namespace: "ns"},
			{Id: 33, Name: "disabled", IsActive: true, Namespace: "ns", Tracing: off},
		},
		Llms: []*pb.LLMConfig{{Id: 41, Name: "capped", Vendor: "openai", IsActive: true, Namespace: "ns", TracingContentModeMax: "metadata"}},
		AgentTracingContentPolicy: &pb.AgentTracingContentPolicy{
			Ceiling: "redacted", DefaultMode: "metadata", MaxRequestOverride: "redacted", StoreCap: "full",
			MaxPartBytes: 1000, MaxSpanBytes: 5000, RedactionDetectors: []string{"email"},
		},
	}
	require.NoError(t, sync.SyncConfiguration(snap))

	proc, ok := agenttrace.CurrentContentProcessor().(*recordingProcessor)
	require.True(t, ok, "the pipeline is installed")
	assert.Equal(t, agenttrace.ContentGlobals{Ceiling: "redacted", DefaultMode: "metadata", MaxRequestOverride: "redacted", StoreCap: "full",
		MaxPartBytes: 1000, MaxSpanBytes: 5000, Detectors: []string{"email"}}, proc.globals)

	llm := agenttrace.Resource{Type: agenttrace.ResourceLLM, ID: "41"}
	assert.Equal(t, agenttrace.ContentOverrides{AppMode: "redacted", AppMaxRequestOverride: "full", ResourceMax: "metadata"}, proc.overrides(31, llm))
	assert.Equal(t, agenttrace.ContentOverrides{}, proc.overrides(32, agenttrace.Resource{}))
	// Fail closed: tracing off for the App, an App the edge does not have,
	// no App, an unknown resource.
	assert.Equal(t, appOff, proc.overrides(33, agenttrace.Resource{}))
	assert.Equal(t, appOff, proc.overrides(999, agenttrace.Resource{}))
	assert.Equal(t, appOff, proc.overrides(0, agenttrace.Resource{}))
	assert.Equal(t, "off", proc.overrides(32, agenttrace.Resource{Type: agenttrace.ResourceTool, ID: "77"}).ResourceMax)

	require.NoError(t, sync.SyncConfiguration(&pb.ConfigurationSnapshot{Version: "2", EdgeNamespace: "ns", SnapshotTime: timestamppb.Now()}))
	assert.Nil(t, agenttrace.CurrentContentProcessor(), "tracing off on the hub removes the pipeline")
}

// Without a registered pipeline (Community build) the edge installs none,
// whatever the hub sends.
func TestInstallAgentTracingContent_CommunityStaysInert(t *testing.T) {
	agenttrace.RegisterContentProcessorFactory(nil)
	t.Cleanup(func() { agenttrace.SetContentProcessor(nil) })
	installAgentTracingContent(&pb.AgentTracingContentPolicy{DefaultMode: "full"}, setupEdgeSyncTestDB(t))
	assert.Nil(t, agenttrace.CurrentContentProcessor())
}

func TestEdgeContentOverrides_Cache(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	require.NoError(t, db.Create(&database.App{Name: "a", IsActive: true, Tracing: database.JSON(`{"content_mode":"full"}`)}).Error)
	var app database.App
	require.NoError(t, db.First(&app).Error)

	e := newEdgeContentOverrides(db)
	now := time.Now()
	e.now = func() time.Time { return now }
	assert.Equal(t, "full", e.Lookup(app.ID, agenttrace.Resource{}).AppMode)
	require.NoError(t, db.Model(&app).Update("tracing", database.JSON(`{"content_mode":"metadata"}`)).Error)
	assert.Equal(t, "full", e.Lookup(app.ID, agenttrace.Resource{}).AppMode, "cached")
	now = now.Add(edgeOverridesTTL + time.Second)
	assert.Equal(t, "metadata", e.Lookup(app.ID, agenttrace.Resource{}).AppMode)

	require.NoError(t, db.Model(&app).Update("tracing", database.JSON(`not json`)).Error)
	now = now.Add(edgeOverridesTTL + time.Second)
	assert.Equal(t, "off", e.Lookup(app.ID, agenttrace.Resource{}).AppMode, "unreadable settings fail closed")
}
