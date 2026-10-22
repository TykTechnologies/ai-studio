package services

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// The edge follows the hub's assembled-export bit: on, it leaves out its
// live proxy span for captured requests; a snapshot without it, or without
// tracing settings at all, turns that off again (jyx.90).
func TestInstallAgentTracingContent_ExportActive(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	t.Cleanup(func() {
		agenttrace.SetExportActive(false)
		agenttrace.SetContentProcessor(nil)
	})

	installAgentTracingContent(&pb.AgentTracingContentPolicy{DefaultMode: "metadata", AssembledExportActive: true}, db)
	assert.True(t, agenttrace.ExportActive())

	installAgentTracingContent(&pb.AgentTracingContentPolicy{DefaultMode: "metadata"}, db)
	assert.False(t, agenttrace.ExportActive())

	agenttrace.SetExportActive(true)
	installAgentTracingContent(nil, db)
	assert.False(t, agenttrace.ExportActive(), "tracing off on the hub")
}
