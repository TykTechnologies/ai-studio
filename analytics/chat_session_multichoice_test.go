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

// The in-tree langchaingo Anthropic provider returns one ContentChoice per
// content block (thinking, text, tool_use), and every choice carries the
// response's full usage. A chat-session turn that answers with text and a tool
// call must still record the usage once.
func TestRecordContentMessage_AnthropicMultiBlockUsageCountedOnce(t *testing.T) {
	db := setupStatsTest(t)
	const model = "claude-multiblock"
	p := models.ModelPrice{ModelName: model, Vendor: string(models.ANTHROPIC), Currency: "USD", CPIT: 0.000010, CPT: 0.000030}
	require.NoError(t, db.Create(&p).Error)

	var rec *models.LLMChatRecord
	withHandler(t, &mockHandler{onRecordChatRecord: func(r *models.LLMChatRecord) { rec = r }})

	usage := func() map[string]any {
		return map[string]any{"InputTokens": 1000, "OutputTokens": 500, "CacheCreationInputTokens": 0, "CacheReadInputTokens": 0}
	}
	mc := llms.TextParts(llms.ChatMessageTypeHuman, "hi")
	cr := &llms.ContentResponse{Choices: []*llms.ContentChoice{
		{Content: "let me look that up", GenerationInfo: usage()},
		{ToolCalls: []llms.ToolCall{{ID: "toolu_1", FunctionCall: &llms.FunctionCall{Name: "lookup", Arguments: "{}"}}}, GenerationInfo: usage()},
	}}
	RecordContentMessage(&mc, cr, models.ANTHROPIC, model, "chat-1", 10, 1, 1, 9, time.Now(), &services.Service{DB: db}, true)
	require.NotNil(t, rec)

	assert.Equal(t, 1000, rec.PromptTokens, "prompt tokens")
	assert.Equal(t, 500, rec.ResponseTokens, "response tokens")
	assert.InDelta(t, models.ChatRecordCost(string(models.ANTHROPIC), p.CPT, p.CPIT, 0, 0, 1000, 500, 0, 0), rec.Cost, 1e-9, "cost")
}
