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

// An edge event's service tier lands on both hub rows; the cost is stored as
// sent (the edge already applied the tier's multiplier).
func TestSendAnalyticsPulse_ServiceTierPersisted(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	now := time.Now()
	pulse := &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "test", SequenceNumber: 1, TotalRecords: 2,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{RequestId: "r1", AppId: 1, LlmId: 1, ModelName: "claude-luna", Vendor: "anthropic", StatusCode: 200,
				RequestTokens: 1000, TotalTokens: 1000, Cost: 10, ServiceTier: "batch", Timestamp: timestamppb.New(now)},
			{RequestId: "r2", AppId: 1, LlmId: 1, ModelName: "claude-luna", Vendor: "anthropic", StatusCode: 200,
				Timestamp: timestamppb.New(now.Add(time.Second))},
		},
	}
	resp, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	require.True(t, resp.Success)

	var recs []models.LLMChatRecord
	require.Eventually(t, func() bool {
		recs = nil
		return db.Order("time_stamp asc").Find(&recs).Error == nil && len(recs) == 2
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, "batch", recs[0].ServiceTier)
	assert.InDelta(t, 10, recs[0].Cost, 1e-9)
	assert.Equal(t, "", recs[1].ServiceTier)

	var logs []models.ProxyLog
	require.Eventually(t, func() bool {
		logs = nil
		return db.Order("time_stamp asc").Find(&logs).Error == nil && len(logs) == 2
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, "batch", logs[0].ServiceTier)
	assert.Equal(t, "", logs[1].ServiceTier)
}
