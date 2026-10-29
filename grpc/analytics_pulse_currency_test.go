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

// Edge chat records keep the currency the edge priced them in; they were all
// recorded as USD. An edge that sends none (older edges) or an invalid one
// is recorded as USD.
func TestSendAnalyticsPulse_CurrencyPersisted(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	now := time.Now()
	event := func(id string, currency string, i int) *pb.AnalyticsEvent {
		return &pb.AnalyticsEvent{RequestId: id, AppId: 1, LlmId: 1, ModelName: "gpt-4o", Vendor: "openai", StatusCode: 200,
			Cost: 100, Currency: currency, Timestamp: timestamppb.New(now.Add(time.Duration(i) * time.Second))}
	}
	pulse := &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "test", SequenceNumber: 1, TotalRecords: 3,
		AnalyticsEvents: []*pb.AnalyticsEvent{event("r1", "EUR", 0), event("r2", "", 1), event("r3", "EURO", 2)},
	}

	resp, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	require.True(t, resp.Success)
	time.Sleep(200 * time.Millisecond) // async analytics

	var recs []models.LLMChatRecord
	require.NoError(t, db.Order("time_stamp asc").Find(&recs).Error)
	require.Len(t, recs, 3)
	assert.Equal(t, "EUR", recs[0].Currency)
	assert.Equal(t, "USD", recs[1].Currency)
	assert.Equal(t, "USD", recs[2].Currency)
}
