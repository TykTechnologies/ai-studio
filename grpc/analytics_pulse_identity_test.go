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

// Who an edge request was for and the agent that made it, when an edge auth
// plugin said, land on the hub's proxy log and chat record.
func TestSendAnalyticsPulse_IdentityPersisted(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	pulse := &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "test", SequenceNumber: 1, TotalRecords: 1,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{RequestId: "r1", AppId: 1, LlmId: 1, UserId: 3, ModelName: "gpt-4o", Vendor: "openai", StatusCode: 200,
				Timestamp: timestamppb.New(time.Now()), OnBehalfOf: "alice@example.com", ActingAgent: "agent-7"},
		},
	}
	resp, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	require.True(t, resp.Success)

	require.Eventually(t, func() bool {
		var n int64
		db.Model(&models.LLMChatRecord{}).Count(&n)
		return n == 1
	}, 5*time.Second, 50*time.Millisecond)

	var log models.ProxyLog
	require.NoError(t, db.First(&log).Error)
	assert.Equal(t, "alice@example.com", log.OnBehalfOf)
	assert.Equal(t, "agent-7", log.ActingAgent)
	assert.Equal(t, uint(3), log.UserID, "the user id stays the App owner")

	var rec models.LLMChatRecord
	require.NoError(t, db.First(&rec).Error)
	assert.Equal(t, "alice@example.com", rec.OnBehalfOf)
	assert.Equal(t, "agent-7", rec.ActingAgent)
}
