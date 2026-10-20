package grpc

import (
	"context"
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
