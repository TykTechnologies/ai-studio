//go:build enterprise
// +build enterprise

package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/TykTechnologies/ai-studio-enterprise/v2/features/agenttracing/content"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// The edge resolves content modes with the hub's ceiling and default from
// the snapshot, App and resource settings from its synced rows, and the
// request header (jyx.172). A snapshot from a hub that sends no ceiling
// captures nothing.
func TestEdgeContentCeilingMatrix(t *testing.T) {
	agenttrace.RegisterContentProcessorFactory(func(g agenttrace.ContentGlobals, o agenttrace.ContentOverridesFunc) agenttrace.ContentProcessor {
		return content.NewProcessor(g, o)
	})
	t.Cleanup(func() {
		agenttrace.RegisterContentProcessorFactory(nil)
		agenttrace.SetContentProcessor(nil)
	})
	db := setupEdgeSyncTestDB(t)
	sync := NewEdgeSyncService(db, "ns")
	snap := func(policy *pb.AgentTracingContentPolicy) *pb.ConfigurationSnapshot {
		return &pb.ConfigurationSnapshot{
			Version: "1", EdgeNamespace: "ns", SnapshotTime: timestamppb.Now(),
			Apps: []*pb.AppConfig{
				{Id: 1, Name: "inherits", IsActive: true, Namespace: "ns"},
				{Id: 2, Name: "full", IsActive: true, Namespace: "ns", Tracing: `{"content_mode":"full"}`},
				{Id: 3, Name: "metadata", IsActive: true, Namespace: "ns", Tracing: `{"content_mode":"metadata","max_request_override":"full"}`},
			},
			Llms: []*pb.LLMConfig{
				{Id: 41, Name: "capped", Vendor: "openai", IsActive: true, Namespace: "ns", TracingContentModeMax: "metadata"},
			},
			AgentTracingContentPolicy: policy,
		}
	}
	// llm "" is a span with no resource maximum.
	mode := func(app uint, llm, header string) string {
		p, ok := agenttrace.CurrentContentProcessor().(*content.Processor)
		require.True(t, ok)
		ev := &agenttrace.Event{
			Identity: agenttrace.Identity{AppID: app},
			Declared: agenttrace.DeclaredContext{CaptureMode: header},
		}
		if llm != "" {
			ev.Resource = agenttrace.Resource{Type: agenttrace.ResourceLLM, ID: llm}
		}
		return p.Mode(ev)
	}

	require.NoError(t, sync.SyncConfiguration(snap(&pb.AgentTracingContentPolicy{
		Ceiling: "redacted", DefaultMode: "off", MaxRequestOverride: "metadata", StoreCap: "full",
	})))
	cases := []struct {
		app         uint
		llm, header string
		want        string
	}{
		{1, "", "", "off"},              // the default
		{1, "", "metadata", "metadata"}, // header raise up to the override
		{1, "", "full", "metadata"},     // ... and no further
		{2, "", "", "redacted"},         // App above the default, held at the ceiling
		{2, "", "off", "off"},           // the header lowers
		{2, "41", "", "metadata"},       // the resource maximum
		{3, "", "", "metadata"},         // the App's own mode
		{3, "", "full", "metadata"},     // the global override still bounds the App's (jyx.99)
		{3, "41", "full", "metadata"},   // the resource maximum bounds the header
		{999, "", "full", "off"},        // an App the edge does not have: no header raise
		{0, "", "metadata", "off"},      // no App
	}
	for _, c := range cases {
		assert.Equal(t, c.want, mode(c.app, c.llm, c.header), "app %d llm %s header %q", c.app, c.llm, c.header)
	}

	// An older hub sends no ceiling: nothing is captured.
	require.NoError(t, sync.SyncConfiguration(snap(&pb.AgentTracingContentPolicy{
		DefaultMode: "full", MaxRequestOverride: "full", StoreCap: "full",
	})))
	assert.Equal(t, "off", mode(2, "", "full"))
	assert.Equal(t, "off", mode(1, "", ""))

	// A ceiling the edge does not know (a typo, a newer mode) is off
	// (jyx.180).
	require.NoError(t, sync.SyncConfiguration(snap(&pb.AgentTracingContentPolicy{
		Ceiling: "metdata", DefaultMode: "metadata", MaxRequestOverride: "full", StoreCap: "full",
	})))
	assert.Equal(t, "off", mode(2, "", "full"))
	assert.Equal(t, "off", mode(1, "", "full"))
}
