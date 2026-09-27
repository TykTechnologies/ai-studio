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

// An edge event's latency is kept on the hub's chat record (total_time_ms);
// the hub dropped it, so edge traffic had none in analytics.
func TestSendAnalyticsPulse_LatencyPersisted(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	pulse := &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "test", SequenceNumber: 1, TotalRecords: 1,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{RequestId: "r1", AppId: 1, LlmId: 1, ModelName: "gpt-4o", Vendor: "openai", StatusCode: 200,
				TotalTokens: 9, LatencyMs: 432, Timestamp: timestamppb.New(time.Now())},
		},
	}

	resp, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	require.True(t, resp.Success)

	var recs []models.LLMChatRecord
	require.Eventually(t, func() bool {
		recs = nil
		return db.Find(&recs).Error == nil && len(recs) == 1
	}, 5*time.Second, 50*time.Millisecond)
	assert.Equal(t, 432, recs[0].TotalTimeMS)
}
