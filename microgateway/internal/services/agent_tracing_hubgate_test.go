package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// The applied snapshot opens and closes edge capture: with no content
// policy (tracing off on the hub) nothing is captured, so nothing is queued
// for a hub that would discard it (jyx.128).
func TestInstallAgentTracingContent_GatesCaptureOnTheHub(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	agenttrace.SetEnabled(true)
	agenttrace.RequireHubTracing(true)
	t.Cleanup(func() {
		agenttrace.SetEnabled(false)
		agenttrace.RequireHubTracing(false)
		agenttrace.SetHubTracing(false)
		agenttrace.SetContentProcessor(nil)
		agenttrace.SetExportActive(false)
	})

	installAgentTracingContent(nil, db)
	_, rec := agenttrace.Begin(context.Background())
	assert.Nil(t, rec, "no policy in the snapshot: nothing captured")

	installAgentTracingContent(&pb.AgentTracingContentPolicy{DefaultMode: "metadata"}, db)
	_, rec = agenttrace.Begin(context.Background())
	assert.NotNil(t, rec, "the hub's tracing is on")

	installAgentTracingContent(nil, db)
	assert.False(t, agenttrace.Enabled(), "the hub turned tracing off")
}
