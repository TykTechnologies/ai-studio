package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// An edge event's gateway_node is the authenticated edge's id, whatever the
// event claims (jyx.150).
func TestSendAnalyticsPulse_CaptureGatewayNodeIsTheEdge(t *testing.T) {
	db, server := capturePulseDB(t)
	withHubTracing(t, true)

	spoofed := edgeCaptureEvent(3, 7)
	spoofed.GatewayNode = "hub-node-0"
	missing := edgeCaptureEvent(4, 7)
	missing.GatewayNode = ""
	resp, err := server.SendAnalyticsPulse(context.Background(), capturePulse(1, spoofed, missing))
	require.NoError(t, err)
	require.True(t, resp.Success)

	rows := stagedCapture(t, db)
	require.Len(t, rows, 2)
	for _, r := range rows {
		got, err := agenttrace.DecodeStaged(r.Event, r.IdempotencyKey)
		require.NoError(t, err)
		assert.Equal(t, "edge-1", got.GatewayNode)
	}
}
