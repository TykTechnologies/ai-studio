package analytics

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The chat-session path (RecordContentMessage) must record what the proxy
// path (buildChatRecordFor) records for the same usage: OpenAI's and Google's
// prompt counts include their cache tokens, so those are carved out of the
// prompt count, and the cost is models.ChatRecordCost. The analytics cost
// breakdown then splits the recorded cost at a cost-to-list ratio of 1.
func TestRecordContentMessage_CostMatchesChatRecordCost(t *testing.T) {
	usage := breakdownTokens{prompt: 1000, response: 500, cacheWrite: 200, cacheRead: 400}
	cases := []struct {
		vendor models.Vendor
		// genInfo is shaped like the in-tree langchaingo provider's
		// GenerationInfo, including its value types.
		genInfo map[string]any
		want    breakdownTokens
	}{
		{
			vendor: models.OPENAI,
			genInfo: map[string]any{
				"PromptTokens":       usage.prompt + usage.cacheRead,
				"CompletionTokens":   usage.response,
				"PromptCachedTokens": usage.cacheRead,
			},
			want: breakdownTokens{prompt: usage.prompt, response: usage.response, cacheRead: usage.cacheRead},
		},
		{
			vendor: models.GOOGLEAI,
			genInfo: map[string]any{
				"input_tokens":         int32(usage.prompt + usage.cacheRead),
				"output_tokens":        int32(usage.response),
				"ThinkingTokens":       0,
				"CacheReadInputTokens": int32(usage.cacheRead),
			},
			want: breakdownTokens{prompt: usage.prompt, response: usage.response, cacheRead: usage.cacheRead},
		},
		{
			vendor: models.ANTHROPIC,
			genInfo: map[string]any{
				"InputTokens":              usage.prompt,
				"OutputTokens":             usage.response,
				"CacheCreationInputTokens": usage.cacheWrite,
				"CacheReadInputTokens":     usage.cacheRead,
			},
			want: usage,
		},
	}
	priceSets := map[string]models.ModelPrice{
		"unset-cache": {CPIT: 0.000010, CPT: 0.000030},
		"set-cache":   {CPIT: 0.000010, CPT: 0.000030, CacheWritePT: 0.000012, CacheReadPT: 0.000001},
	}

	for _, tc := range cases {
		for setName, p := range priceSets {
			vendor := string(tc.vendor)
			model := vendor + "-" + setName
			t.Run(model, func(t *testing.T) {
				db := setupStatsTest(t)
				p.ModelName, p.Vendor, p.Currency = model, vendor, "USD"
				require.NoError(t, db.Create(&p).Error)

				var rec *models.LLMChatRecord
				withHandler(t, &mockHandler{onRecordChatRecord: func(r *models.LLMChatRecord) { rec = r }})

				mc := llms.TextParts(llms.ChatMessageTypeHuman, "hi")
				cr := &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: "hello", GenerationInfo: tc.genInfo}}}
				now := time.Now().UTC().Truncate(time.Second)
				const llmID = 9
				RecordContentMessage(&mc, cr, tc.vendor, model, "chat-1", 10, 1, 1, llmID, now, &services.Service{DB: db}, true)
				require.NotNil(t, rec)

				w := tc.want
				assert.Equal(t, w.prompt, rec.PromptTokens, "prompt tokens")
				assert.Equal(t, w.response, rec.ResponseTokens, "response tokens")
				assert.Equal(t, w.cacheWrite, rec.CacheWritePromptTokens, "cache write tokens")
				assert.Equal(t, w.cacheRead, rec.CacheReadPromptTokens, "cache read tokens")
				assert.Equal(t, w.prompt+w.response+w.cacheWrite+w.cacheRead, rec.TotalTokens, "total tokens")
				wantCost := models.ChatRecordCost(vendor, p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT,
					w.prompt, w.response, w.cacheWrite, w.cacheRead)
				assert.InDelta(t, wantCost, rec.Cost, 1e-9, "cost")

				require.NoError(t, db.Create(rec).Error)
				row, ok := breakdownRows(t, db, llmID, now)[model]
				require.True(t, ok)
				assertBreakdown(t, row, listComponents(vendor, p, w), model)
			})
		}
	}
}

// Usage is read once per response, from the first choice that carries it, and
// the cache tokens come from that same choice: a choice with no usage (here a
// leading block without GenerationInfo) is skipped, and a second candidate
// repeating the response's usage is not added again.
func TestRecordContentMessage_UsageReadOnceFromFirstChoiceCarryingIt(t *testing.T) {
	db := setupStatsTest(t)
	const model = "gemini-two-candidates"
	p := models.ModelPrice{ModelName: model, Vendor: string(models.GOOGLEAI), Currency: "USD", CPIT: 0.000010, CPT: 0.000030}
	require.NoError(t, db.Create(&p).Error)

	var rec *models.LLMChatRecord
	withHandler(t, &mockHandler{onRecordChatRecord: func(r *models.LLMChatRecord) { rec = r }})

	usage := func() map[string]any {
		return map[string]any{"input_tokens": int32(1400), "output_tokens": int32(500), "CacheReadInputTokens": int32(400)}
	}
	mc := llms.TextParts(llms.ChatMessageTypeHuman, "hi")
	cr := &llms.ContentResponse{Choices: []*llms.ContentChoice{
		{Content: "a"},
		{Content: "b", GenerationInfo: usage()},
		{Content: "c", GenerationInfo: usage()},
	}}
	RecordContentMessage(&mc, cr, models.GOOGLEAI, model, "chat-1", 10, 1, 1, 9, time.Now(), &services.Service{DB: db}, true)
	require.NotNil(t, rec)

	assert.Equal(t, 1000, rec.PromptTokens, "prompt tokens")
	assert.Equal(t, 500, rec.ResponseTokens, "response tokens")
	assert.Equal(t, 400, rec.CacheReadPromptTokens, "cache read tokens")
	assert.Equal(t, 1900, rec.TotalTokens, "total tokens")
	assert.InDelta(t, models.ChatRecordCost(string(models.GOOGLEAI), p.CPT, p.CPIT, 0, 0, 1000, 500, 0, 400), rec.Cost, 1e-9, "cost")
}
