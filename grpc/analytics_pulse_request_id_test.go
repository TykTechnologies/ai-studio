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

// An edge event's gateway and client request ids land on the hub's proxy log;
// a request with no usage (a guardrail block) is a log row only, with no chat
// record to count as an interaction or spend.
func TestSendAnalyticsPulse_RequestIDsAndNoUsage(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	now := time.Now()
	pulse := &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "test", SequenceNumber: 1, TotalRecords: 2,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{RequestId: "proxy_1_1", AppId: 1, LlmId: 11, Vendor: "openai", StatusCode: 400, Timestamp: timestamppb.New(now),
				GatewayRequestId: "tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b", ClientRequestId: "lb-1\n<x>", NoUsage: true},
			{RequestId: "proxy_1_2", AppId: 1, LlmId: 11, Vendor: "openai", StatusCode: 200, TotalTokens: 10, Cost: 1,
				Timestamp: timestamppb.New(now.Add(time.Second)), GatewayRequestId: "tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1c"},
		},
	}
	resp, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	require.True(t, resp.Success)

	var logs []models.ProxyLog
	require.Eventually(t, func() bool {
		logs = nil
		return db.Order("time_stamp asc").Find(&logs).Error == nil && len(logs) == 2
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, "tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b", logs[0].RequestID)
	assert.Equal(t, "lb-1x", logs[0].ClientRequestID, "sanitised")
	assert.Equal(t, 400, logs[0].ResponseCode)
	assert.Equal(t, "tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1c", logs[1].RequestID)

	var recs []models.LLMChatRecord
	require.Eventually(t, func() bool {
		recs = nil
		return db.Find(&recs).Error == nil && len(recs) == 1
	}, 5*time.Second, 20*time.Millisecond)
	time.Sleep(200 * time.Millisecond) // a stray chat record for the block would land by now
	require.NoError(t, db.Find(&recs).Error)
	require.Len(t, recs, 1, "the blocked request got a chat record")
	assert.Equal(t, 10, recs[0].TotalTokens)
}
