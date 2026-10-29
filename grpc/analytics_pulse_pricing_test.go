package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	protobuf "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// An edge event's requested model and price multiplier land on the hub's
// chat record, so a price recalculation reaches and reprices it like an
// embedded gateway row. An older edge sends neither; an out-of-range
// multiplier is dropped.
func TestSendAnalyticsPulse_RequestedModelAndMultiplierPersisted(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	now := time.Now()
	event := func(id string, at time.Duration) *pb.AnalyticsEvent {
		return &pb.AnalyticsEvent{RequestId: id, AppId: 1, LlmId: 1, ModelName: "gemini-2.0-flash-001", Vendor: "google_ai",
			StatusCode: 200, RequestTokens: 1000, TotalTokens: 1000, Cost: 5, Timestamp: timestamppb.New(now.Add(at))}
	}
	batch := event("r1", 0)
	batch.RequestedModel = "gemini-2.0-flash"
	batch.PriceMultiplier = protobuf.Float64(0.5)
	older := event("r2", time.Second)
	bogus := event("r3", 2*time.Second)
	bogus.PriceMultiplier = protobuf.Float64(1e6)

	resp, err := server.SendAnalyticsPulse(context.Background(), &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "test", SequenceNumber: 1, TotalRecords: 3,
		AnalyticsEvents: []*pb.AnalyticsEvent{batch, older, bogus},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	var recs []models.LLMChatRecord
	require.Eventually(t, func() bool {
		recs = nil
		return db.Order("time_stamp asc").Find(&recs).Error == nil && len(recs) == 3
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, "gemini-2.0-flash", recs[0].RequestedModel)
	require.NotNil(t, recs[0].PriceMultiplier)
	assert.Equal(t, 0.5, *recs[0].PriceMultiplier)
	assert.Empty(t, recs[1].RequestedModel)
	assert.Nil(t, recs[1].PriceMultiplier, "an older edge sends no multiplier")
	assert.Nil(t, recs[2].PriceMultiplier, "an out-of-range multiplier is dropped")
}
