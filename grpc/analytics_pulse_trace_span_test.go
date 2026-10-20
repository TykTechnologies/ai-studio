package grpc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The edge's request id and the agent tracing span id reach the hub's proxy
// log; the hub used to drop the request id at ingest.
func TestSendAnalyticsPulse_RequestIDAndTraceSpanPersisted(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	pulse := &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "test", SequenceNumber: 1, TotalRecords: 2,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{RequestId: "req_edge_1", TraceSpanId: "00f067aa0ba902b7", AppId: 1, LlmId: 1, UserId: 3,
				ModelName: "gpt-4o", Vendor: "openai", StatusCode: 200, Timestamp: timestamppb.New(time.Now())},
			{RequestId: "req_edge_2", AppId: 1, LlmId: 1, UserId: 3,
				ModelName: "gpt-4o", Vendor: "openai", StatusCode: 200, Timestamp: timestamppb.New(time.Now())},
		},
	}
	resp, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	require.True(t, resp.Success)

	var logs []models.ProxyLog
	require.Eventually(t, func() bool {
		logs = nil
		db.Order("request_id").Find(&logs)
		return len(logs) == 2
	}, 5*time.Second, 50*time.Millisecond)

	assert.Equal(t, "req_edge_1", logs[0].RequestID)
	assert.Equal(t, "00f067aa0ba902b7", logs[0].TraceSpanID)
	assert.Equal(t, "req_edge_2", logs[1].RequestID)
	assert.Empty(t, logs[1].TraceSpanID, "an edge with tracing off sends no span id")
}

// Edge values are untrusted: a malformed span id or request id is cleared at
// ingest instead of being stored (or, over the column size on Postgres,
// failing the row's whole batch) (jyx.88).
func TestSendAnalyticsPulse_MalformedTraceIdentityCleared(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	event := func(requestID, spanID, model string) *pb.AnalyticsEvent {
		return &pb.AnalyticsEvent{RequestId: requestID, TraceSpanId: spanID, AppId: 1, LlmId: 1, UserId: 3,
			ModelName: model, Vendor: "openai", StatusCode: 200, Timestamp: timestamppb.New(time.Now())}
	}
	pulse := &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "test", SequenceNumber: 1, TotalRecords: 3,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			event(strings.Repeat("r", 65), strings.Repeat("a", 40), "m1"),
			event("req\nforged", "zz67aa0ba902b7zz", "m2"),
			event("req_ok", "00F067AA0BA902B7", "m3"),
		},
	}
	resp, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	require.True(t, resp.Success)

	var logs []models.ProxyLog
	require.Eventually(t, func() bool {
		logs = nil
		db.Order("model_name").Find(&logs)
		return len(logs) == 3
	}, 5*time.Second, 50*time.Millisecond)

	assert.Empty(t, logs[0].RequestID, "over the column size")
	assert.Empty(t, logs[0].TraceSpanID, "not a span id")
	assert.Empty(t, logs[1].RequestID, "control characters")
	assert.Empty(t, logs[1].TraceSpanID, "not hex")
	assert.Equal(t, "req_ok", logs[2].RequestID)
	assert.Equal(t, "00f067aa0ba902b7", logs[2].TraceSpanID, "a valid span id is stored lower-case")
}

func TestEdgeTraceIdentityValidation(t *testing.T) {
	assert.Equal(t, "", edgeTraceSpanID(""))
	assert.Equal(t, "", edgeTraceSpanID("0000000000000000"), "the zero span id is no span")
	assert.Equal(t, "00f067aa0ba902b7", edgeTraceSpanID("00f067aa0ba902b7"))
	assert.Equal(t, strings.Repeat("r", 64), edgeRequestID(strings.Repeat("r", 64)))
	assert.Equal(t, "", edgeRequestID(strings.Repeat("r", 65)))
	assert.Equal(t, "", edgeRequestID("req\x00"))
}
