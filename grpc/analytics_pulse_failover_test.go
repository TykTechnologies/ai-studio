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

// An edge that failed over sends one event per attempt. The hub's ProxyLog
// must keep the marker so the failed primary row and the fallback row can be
// read together; dropping it here is exactly the class of bug that made edge
// logs vanish from the LLM view before (llm_id = 0).
func TestSendAnalyticsPulse_FailoverMarkerPersisted(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	now := time.Now()
	pulse := &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "test", SequenceNumber: 1, TotalRecords: 2,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{RequestId: "r1", AppId: 1, LlmId: 1, ModelName: "gpt-4o", Vendor: "openai", StatusCode: 503,
				Timestamp: timestamppb.New(now)},
			{RequestId: "r2", AppId: 1, LlmId: 2, ModelName: "claude-sonnet-4", Vendor: "anthropic", StatusCode: 200,
				Timestamp: timestamppb.New(now.Add(time.Second)), FailoverFromLlmId: 1, FailoverAttempt: 1},
		},
	}

	resp, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	require.True(t, resp.Success)
	time.Sleep(200 * time.Millisecond) // async analytics

	var logs []models.ProxyLog
	require.NoError(t, db.Order("time_stamp asc").Find(&logs).Error)
	require.Len(t, logs, 2)

	assert.Equal(t, uint(1), logs[0].LLMID)
	assert.Nil(t, logs[0].FailoverFromLLMID)
	assert.Equal(t, 0, logs[0].FailoverAttempt)

	assert.Equal(t, uint(2), logs[1].LLMID)
	require.NotNil(t, logs[1].FailoverFromLLMID)
	assert.Equal(t, uint(1), *logs[1].FailoverFromLLMID)
	assert.Equal(t, 1, logs[1].FailoverAttempt)
}
