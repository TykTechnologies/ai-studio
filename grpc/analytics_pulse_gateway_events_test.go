package grpc

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// An edge's gateway events are bounded before staging: an unknown or
// 1 KiB type and 10k events never reach the assembler, which makes a span
// of each (jyx.171).
func TestSendAnalyticsPulse_CaptureGatewayEventsBounded(t *testing.T) {
	db, server := capturePulseDB(t)
	withHubTracing(t, true)

	ev := edgeCaptureEvent(5, 7)
	ev.GatewayEvents = []agenttrace.GatewayEvent{
		{Type: "evil", Outcome: agenttrace.OutcomeBlock},
		{Type: strings.Repeat("t", 1024), Outcome: agenttrace.OutcomeBlock},
	}
	for i := 0; i < 10000; i++ {
		ev.GatewayEvents = append(ev.GatewayEvents, agenttrace.GatewayEvent{
			Type: agenttrace.GatewayEventAttempt, Outcome: agenttrace.OutcomeError, Name: strings.Repeat("m", 1024),
		})
	}
	resp, err := server.SendAnalyticsPulse(context.Background(), capturePulse(1, ev))
	require.NoError(t, err)
	require.True(t, resp.Success)

	rows := stagedCapture(t, db)
	require.Len(t, rows, 1)
	got, err := agenttrace.DecodeStaged(rows[0].Event, rows[0].IdempotencyKey)
	require.NoError(t, err)
	require.Len(t, got.GatewayEvents, 64)
	for _, ge := range got.GatewayEvents {
		assert.Equal(t, agenttrace.GatewayEventAttempt, ge.Type)
		assert.Empty(t, ge.Name, "a 1 KiB name is dropped")
	}
}
