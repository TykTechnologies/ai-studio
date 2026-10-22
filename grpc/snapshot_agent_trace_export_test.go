package grpc

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// While the hub exports assembled agent-trace spans, the snapshot tells
// edges so, and they leave out their live proxy span too (jyx.90).
func TestSnapshot_AssembledExportActive(t *testing.T) {
	server, db := setupTestServer(t, nil)
	createTestLLMs(db, "")
	agenttrace.SetFingerprinter(exportingFingerprinter{agenttrace.StaticFingerprinter{ID: 42, Key: bytes.Repeat([]byte{7}, 32)}})
	agenttrace.SetEdgeContentGlobals(&agenttrace.ContentGlobals{DefaultMode: "metadata"})
	t.Cleanup(func() {
		agenttrace.SetFingerprinter(nil)
		agenttrace.SetEdgeContentGlobals(nil)
		agenttrace.SetExportActive(false)
	})

	agenttrace.SetExportActive(false)
	off, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	require.NotNil(t, off.AgentTracingContentPolicy)
	assert.False(t, off.AgentTracingContentPolicy.AssembledExportActive)

	agenttrace.SetExportActive(true)
	on, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	assert.True(t, on.AgentTracingContentPolicy.AssembledExportActive)
	assert.NotEqual(t, off.Checksum, on.Checksum, "starting export marks edges pending")
}
