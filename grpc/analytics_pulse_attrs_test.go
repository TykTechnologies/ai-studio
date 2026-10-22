package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// An edge cannot forge the hub's own statements: attribution, the offered
// tool set and content keys it sends never reach the staged event the
// assembler builds spans from; the attributes a capture writes do
// (jyx.167).
func TestSendAnalyticsPulse_CaptureForgedAttributesDropped(t *testing.T) {
	db, server := capturePulseDB(t)
	withHubTracing(t, true)

	ev := edgeCaptureEvent(5, 7)
	ev.Attributes = map[string]string{
		"tyk.attribution.rule":       "catalogue_unique",
		"tyk.attribution.confidence": "exact",
		"tyk.attribution.candidates": `["tool:1"]`,
		"tyk.tools.offered":          `["x"]`,
		"tyk.tools.available":        "1",
		"tyk.tool_calls.not_offered": `["y"]`,
		"tyk.tool.in_catalogue":      "true",
		"gen_ai.input.messages":      "secret prompt",
		"mcp.method.name":            "tools/call",
		"tyk.server_tool_calls":      "1",
	}
	resp, err := server.SendAnalyticsPulse(context.Background(), capturePulse(1, ev))
	require.NoError(t, err)
	require.True(t, resp.Success)

	rows := stagedCapture(t, db)
	require.Len(t, rows, 1)
	got, err := agenttrace.DecodeStaged(rows[0].Event, rows[0].IdempotencyKey)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"mcp.method.name": "tools/call", "tyk.server_tool_calls": "1"}, got.Attributes)
}
