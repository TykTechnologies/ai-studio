package analytics

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDecisionUsage(t *testing.T) {
	db := setupStatsTest(t)

	day1 := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	day2 := day1.Add(24 * time.Hour)
	recs := []models.LLMChatRecord{
		// Two Jev decisions on day 1, one with a refusal.
		{LLMID: 1, AppID: 10, Vendor: "systemone", Name: "jev-1.13.0", PromptTokens: 200, Choices: 3, Cost: 0.084, Currency: "USD", TimeStamp: day1,
			RequestKind: models.DecisionRequest, DecisionQuestions: 3},
		{LLMID: 1, AppID: 11, Vendor: "systemone", Name: "jev-1.13.0", PromptTokens: 100, Choices: 1, Cost: 0.042, Currency: "USD", TimeStamp: day1,
			RequestKind: models.DecisionRequest, DecisionQuestions: 2, DecisionRefusals: 1},
		// One OpenAI decision on day 2.
		{LLMID: 2, AppID: 10, Vendor: "openai", Name: "gpt-6-luna", PromptTokens: 1000, Choices: 1, Cost: 1, Currency: "USD", TimeStamp: day2,
			RequestKind: models.DecisionRequest, DecisionQuestions: 1},
		// Chat traffic is not decision traffic.
		{LLMID: 2, AppID: 10, Vendor: "openai", Name: "gpt-4o", PromptTokens: 50, ResponseTokens: 50, Choices: 1, Cost: 5, Currency: "USD", TimeStamp: day2},
	}
	require.NoError(t, db.Create(&recs).Error)

	usage, err := GetDecisionUsage(db, day1.Add(-time.Hour), day2.Add(time.Hour), nil, nil)
	require.NoError(t, err)

	assert.Equal(t, int64(3), usage.Totals.Requests)
	assert.Equal(t, int64(6), usage.Totals.Questions)
	assert.Equal(t, int64(5), usage.Totals.Answered)
	assert.Equal(t, int64(1), usage.Totals.Refusals)
	assert.Equal(t, int64(1300), usage.Totals.InputTokens)
	assert.InDelta(t, (0.084+0.042+1)/10000, usage.Totals.CostByCurrency["USD"], 1e-12)

	require.Len(t, usage.ByDay.Labels, 2)
	assert.Equal(t, []int64{2, 1}, usage.ByDay.Requests)
	assert.Equal(t, []int64{5, 1}, usage.ByDay.Questions)
	assert.Equal(t, []int64{1, 0}, usage.ByDay.Refusals)

	require.Len(t, usage.ByModel, 2)
	assert.Equal(t, "jev-1.13.0", usage.ByModel[0].Model, "busiest model first")
	assert.Equal(t, int64(2), usage.ByModel[0].Requests)
	assert.InDelta(t, (0.126/10000)/2*1000, usage.ByModel[0].CostPerThousand, 1e-12)

	// Filters.
	llm := uint(2)
	usage, err = GetDecisionUsage(db, day1.Add(-time.Hour), day2.Add(time.Hour), &llm, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), usage.Totals.Requests)
	app := uint(11)
	usage, err = GetDecisionUsage(db, day1.Add(-time.Hour), day2.Add(time.Hour), nil, &app)
	require.NoError(t, err)
	assert.Equal(t, int64(1), usage.Totals.Requests)
	assert.Equal(t, int64(1), usage.Totals.Refusals)

	// An empty period is an empty report, not an error.
	usage, err = GetDecisionUsage(db, day2.Add(48*time.Hour), day2.Add(72*time.Hour), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), usage.Totals.Requests)
	assert.Empty(t, usage.ByModel)
	assert.Empty(t, usage.ByDay.Labels)
}

// A decision call the model did not answer (an upstream 404 model_not_found,
// a budget denial) is recorded with the questions it asked and no answers.
// The report counts it as a failed call and keeps its questions out of the
// answered ratio, rather than reading it as a decision call with every
// question unanswered.
func TestGetDecisionUsageSeparatesFailedCalls(t *testing.T) {
	db := setupStatsTest(t)

	day := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	recs := []models.LLMChatRecord{
		{LLMID: 1, Vendor: "openai", Name: "gpt-6-luna", PromptTokens: 100, Choices: 2, Cost: 0.5, Currency: "USD", TimeStamp: day,
			RequestKind: models.DecisionRequest, DecisionQuestions: 3, DecisionRefusals: 1},
		// Two failed calls on a model the vendor does not serve.
		{LLMID: 1, Vendor: "openai", Name: "gpt-5.6-luna", Currency: "USD", TimeStamp: day,
			RequestKind: models.DecisionRequest, DecisionQuestions: 3},
		{LLMID: 1, Vendor: "openai", Name: "gpt-5.6-luna", Currency: "USD", TimeStamp: day,
			RequestKind: models.DecisionRequest, DecisionQuestions: 3},
	}
	require.NoError(t, db.Create(&recs).Error)

	usage, err := GetDecisionUsage(db, day.Add(-time.Hour), day.Add(time.Hour), nil, nil)
	require.NoError(t, err)

	assert.Equal(t, int64(1), usage.Totals.Requests, "only the answered call is a decision call")
	assert.Equal(t, int64(2), usage.Totals.Failed)
	assert.Equal(t, int64(3), usage.Totals.Questions, "failed calls' questions are not counted as asked")
	assert.Equal(t, int64(2), usage.Totals.Answered)
	assert.Equal(t, int64(1), usage.Totals.Refusals)

	assert.Equal(t, []int64{1}, usage.ByDay.Requests)
	assert.Equal(t, []int64{2}, usage.ByDay.Failed)
	assert.Equal(t, []int64{3}, usage.ByDay.Questions)

	require.Len(t, usage.ByModel, 2)
	assert.Equal(t, "gpt-6-luna", usage.ByModel[0].Model)
	assert.Equal(t, int64(1), usage.ByModel[0].Requests)
	assert.Equal(t, int64(0), usage.ByModel[0].Failed)
	assert.InDelta(t, 0.5/10000*1000, usage.ByModel[0].CostPerThousand, 1e-12)
	assert.Equal(t, "gpt-5.6-luna", usage.ByModel[1].Model)
	assert.Equal(t, int64(0), usage.ByModel[1].Requests)
	assert.Equal(t, int64(2), usage.ByModel[1].Failed)
	assert.Equal(t, int64(0), usage.ByModel[1].Questions)
	assert.Equal(t, float64(0), usage.ByModel[1].CostPerThousand)
}
