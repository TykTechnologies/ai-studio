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

// An edge's chat records are proxy traffic, except a guardrail's judge call,
// which keeps its type so it is charged to the judge LLM alone. Anything
// else an edge claims is ignored.
func TestEdgeInteractionType(t *testing.T) {
	for in, want := range map[string]models.InteractionType{
		"guardrail_judge": models.GuardrailJudgeInteraction,
		"":                models.ProxyInteraction,
		"proxy":           models.ProxyInteraction,
		"chat":            models.ProxyInteraction,
		"anything":        models.ProxyInteraction,
	} {
		if got := edgeInteractionType(in); got != want {
			t.Errorf("edgeInteractionType(%q) = %q, want %q", in, got, want)
		}
	}
}

// A judge call recorded on an edge (no App, interaction type
// guardrail_judge) reaches the hub as such: charged to the judge LLM, to no
// App.
func TestSendAnalyticsPulse_GuardrailJudgeRecord(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)
	now := time.Now()
	_, err := server.SendAnalyticsPulse(context.Background(), &pb.AnalyticsPulse{
		EdgeId: "edge-judge", EdgeNamespace: "test", SequenceNumber: 1, TotalRecords: 2,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{RequestId: "chat_0_1", AppId: 0, LlmId: 70, ModelName: "judge-mini", Vendor: "openai", InteractionType: "guardrail_judge",
				StatusCode: 200, RequestTokens: 120, ResponseTokens: 9, TotalTokens: 129, Cost: 0.001, Timestamp: timestamppb.New(now)},
			{RequestId: "req-2", AppId: 4, LlmId: 1, ModelName: "gpt", Vendor: "openai", InteractionType: "chat",
				StatusCode: 200, RequestTokens: 1, ResponseTokens: 1, TotalTokens: 2, Cost: 0.001, Timestamp: timestamppb.New(now)},
		},
	})
	require.NoError(t, err)

	var recs []models.LLMChatRecord
	require.Eventually(t, func() bool {
		recs = nil
		return db.Order("llm_id DESC").Find(&recs).Error == nil && len(recs) == 2
	}, 2*time.Second, 20*time.Millisecond)
	assert.Equal(t, models.GuardrailJudgeInteraction, recs[0].InteractionType)
	assert.Equal(t, uint(70), recs[0].LLMID)
	assert.Equal(t, uint(0), recs[0].AppID)
	// An edge cannot claim any other type.
	assert.Equal(t, models.ProxyInteraction, recs[1].InteractionType)
}
