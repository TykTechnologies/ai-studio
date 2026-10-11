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

// Edge decision traffic must land on the hub as decision traffic: the
// request kind on both rows, the question counts on the chat record, and the
// answered questions as its choices (chat events keep the default of 1).
func TestSendAnalyticsPulse_DecisionFieldsPersisted(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	now := time.Now()
	pulse := &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "test", SequenceNumber: 1, TotalRecords: 2,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{RequestId: "r1", AppId: 1, LlmId: 1, ModelName: "jev-1.13.0", Vendor: "systemone", StatusCode: 200,
				RequestTokens: 200, TotalTokens: 200, Choices: 2, Cost: 0.084,
				RequestKind: "decision", DecisionQuestions: 3, DecisionRefusals: 1,
				Timestamp: timestamppb.New(now)},
			{RequestId: "r2", AppId: 1, LlmId: 2, ModelName: "gpt-4o", Vendor: "openai", StatusCode: 200,
				Choices: 4, Timestamp: timestamppb.New(now.Add(time.Second))},
		},
	}

	resp, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	require.True(t, resp.Success)
	time.Sleep(200 * time.Millisecond) // async analytics

	var logs []models.ProxyLog
	require.NoError(t, db.Order("time_stamp asc").Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Equal(t, models.DecisionRequest, logs[0].RequestKind)
	assert.Equal(t, models.GenerativeRequest, logs[1].RequestKind)

	var recs []models.LLMChatRecord
	require.NoError(t, db.Order("time_stamp asc").Find(&recs).Error)
	require.Len(t, recs, 2)
	assert.Equal(t, models.DecisionRequest, recs[0].RequestKind)
	assert.Equal(t, 3, recs[0].DecisionQuestions)
	assert.Equal(t, 1, recs[0].DecisionRefusals)
	assert.Equal(t, 2, recs[0].Choices)
	assert.Equal(t, 200, recs[0].PromptTokens)
	assert.InDelta(t, 0.084, recs[0].Cost, 1e-9)

	assert.Equal(t, models.GenerativeRequest, recs[1].RequestKind)
	assert.Equal(t, 1, recs[1].Choices, "chat events keep the hub's default")
}
