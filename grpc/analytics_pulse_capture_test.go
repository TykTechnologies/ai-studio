package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func capturePulseDB(t *testing.T) (*gorm.DB, *ControlServer) {
	t.Helper()
	db := setupPulseTestDB(t)
	require.NoError(t, db.AutoMigrate(models.AgentTraceModels()...))
	return db, setupControlServer(t, db)
}

func withHubTracing(t *testing.T, on bool) {
	t.Helper()
	old := agenttrace.Enabled()
	agenttrace.SetEnabled(on)
	t.Cleanup(func() { agenttrace.SetEnabled(old) })
}

func edgeCaptureEvent(span byte, appID uint) *agenttrace.Event {
	return &agenttrace.Event{
		Kind:         agenttrace.KindLLMInference,
		SpanID:       agenttrace.SpanID{0, 0, 0, 0, 0, 0, 0, span},
		TraceID:      agenttrace.TraceID{1, 2, 3},
		Identity:     agenttrace.Identity{AppID: appID, CredentialID: "cred-1"},
		Provider:     "openai",
		Operation:    "chat",
		RequestModel: "gpt-4o",
		StartTime:    time.Unix(1760000000, 0).UTC(),
		EndTime:      time.Unix(1760000001, 0).UTC(),
		Status:       agenttrace.StatusOK,
		GatewayNode:  "edge-1",
		Usage:        agenttrace.Usage{InputTokens: 10, OutputTokens: 5, CostMicros: 42},
		Extracts: agenttrace.Extracts{
			ToolCalls:     []agenttrace.ToolCallRequest{{ID: "call_1", Name: "search"}},
			FinishReasons: []string{"tool_calls"},
		},
		Parts: []agenttrace.PayloadPart{{
			Part: agenttrace.PartRequest, ContentState: agenttrace.StateFull, Data: []byte("secret prompt"),
		}},
	}
}

func capturePulse(seq uint64, events ...*agenttrace.Event) *pb.AnalyticsPulse {
	p := &pb.AnalyticsPulse{
		EdgeId:         "edge-1",
		EdgeNamespace:  "default",
		PulseTimestamp: timestamppb.Now(),
		SequenceNumber: seq,
	}
	for _, ev := range events {
		pe := agenttrace.ToProto(ev)
		pe.IdempotencyKey = "embedded:spoofed:0" // the hub must not trust this
		pe.Source = "embedded"
		p.CaptureEvents = append(p.CaptureEvents, pe)
	}
	p.TotalRecords = uint32(len(p.CaptureEvents))
	return p
}

func stagedCapture(t *testing.T, db *gorm.DB) []models.AgentTraceCaptureEvent {
	t.Helper()
	rows, err := models.ListAgentTraceCaptureEventsAfter(db, 0, 100)
	require.NoError(t, err)
	return rows
}

func TestSendAnalyticsPulse_CaptureEventsStaged(t *testing.T) {
	db, server := capturePulseDB(t)
	withHubTracing(t, true)

	a, b := edgeCaptureEvent(1, 7), edgeCaptureEvent(2, 8)
	resp, err := server.SendAnalyticsPulse(context.Background(), capturePulse(1, a, b))
	require.NoError(t, err)
	require.True(t, resp.Success)
	assert.Equal(t, uint64(2), resp.ProcessedRecords)

	rows := stagedCapture(t, db)
	require.Len(t, rows, 2)
	for i, want := range []*agenttrace.Event{a, b} {
		r := rows[i]
		assert.Equal(t, "edge-1:"+want.SpanID.String()+":0", r.IdempotencyKey, "key is rebuilt from the pulse's edge id")
		assert.Equal(t, "edge-1", r.Source)
		assert.Equal(t, string(agenttrace.KindLLMInference), r.Kind)
		assert.Equal(t, want.Identity.AppID, r.AppID)
		assert.Equal(t, want.SpanID.Bytes(), r.SpanID)
		assert.False(t, r.ReceivedAt.IsZero())
		assert.Equal(t, len(r.Event), r.Size)

		got, err := agenttrace.DecodeStaged(r.Event, r.IdempotencyKey)
		require.NoError(t, err)
		assert.Equal(t, want.SpanID, got.SpanID)
		assert.Equal(t, want.TraceID, got.TraceID)
		assert.Equal(t, want.Identity, got.Identity)
		assert.Equal(t, want.Provider, got.Provider)
		assert.Equal(t, want.RequestModel, got.RequestModel)
		assert.True(t, want.StartTime.Equal(got.StartTime))
		assert.True(t, want.EndTime.Equal(got.EndTime))
		assert.Equal(t, want.Usage, got.Usage)
		assert.Equal(t, want.Extracts.ToolCalls[0].ID, got.Extracts.ToolCalls[0].ID)
		assert.Equal(t, want.Extracts.FinishReasons, got.Extracts.FinishReasons)
		assert.Equal(t, "edge-1", got.Source)
		// No sealer on this hub: the payload never reaches the table.
		assert.Nil(t, got.Parts[0].Data)
		assert.Equal(t, agenttrace.StateDropped, got.Parts[0].ContentState)
	}

	// A retried pulse (the edge never saw the answer) adds nothing.
	resp, err = server.SendAnalyticsPulse(context.Background(), capturePulse(1, a, b))
	require.NoError(t, err)
	require.True(t, resp.Success)
	assert.Len(t, stagedCapture(t, db), 2)
}

func TestSendAnalyticsPulse_CaptureIgnoredWhenTracingOff(t *testing.T) {
	db, server := capturePulseDB(t)
	withHubTracing(t, false)

	resp, err := server.SendAnalyticsPulse(context.Background(), capturePulse(1, edgeCaptureEvent(1, 7)))
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Empty(t, stagedCapture(t, db))
}

func TestSendAnalyticsPulse_CaptureWriteFailure(t *testing.T) {
	db, server := capturePulseDB(t)
	withHubTracing(t, true)
	require.NoError(t, db.Exec("DROP TABLE agent_trace_capture_events").Error)

	// Capture only: fail the pulse so the edge retries it.
	_, err := server.SendAnalyticsPulse(context.Background(), capturePulse(1, edgeCaptureEvent(1, 7)))
	require.Error(t, err)

	// Mixed with other records: never fail (they would be counted twice).
	mixed := capturePulse(2, edgeCaptureEvent(2, 7))
	mixed.ToolCalls = []*pb.ToolCallProto{{ToolId: 1, OperationId: "op", ExecTimeMs: 3, Timestamp: timestamppb.Now()}}
	resp, err := server.SendAnalyticsPulse(context.Background(), mixed)
	require.NoError(t, err)
	assert.True(t, resp.Success)
}

func TestSendAnalyticsPulse_CaptureNeedsEdgeID(t *testing.T) {
	_, server := capturePulseDB(t)
	withHubTracing(t, true)
	p := capturePulse(1, edgeCaptureEvent(1, 7))
	p.EdgeId = ""
	_, err := server.SendAnalyticsPulse(context.Background(), p)
	require.Error(t, err)
}

func TestSendAnalyticsPulse_CaptureDroppedReportOnly(t *testing.T) {
	_, server := capturePulseDB(t)
	withHubTracing(t, true)
	p := capturePulse(1)
	p.CaptureDropped = map[string]uint64{agenttrace.DropSpan: 3}
	resp, err := server.SendAnalyticsPulse(context.Background(), p)
	require.NoError(t, err)
	assert.True(t, resp.Success)
}
