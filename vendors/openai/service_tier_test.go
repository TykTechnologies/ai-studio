package openaiVendor

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

func tierOf(t *testing.T, r models.ITokenResponse) (string, bool) {
	t.Helper()
	pa, ok := r.(models.IPriceAdjustedResponse)
	if !ok {
		return "", false
	}
	assert.Equal(t, 1.0, pa.PriceMultiplier(), "OpenAI tiers bill at list price unless the LLM overrides them")
	return pa.ServiceTier(), true
}

// Synchronous OpenAI replies report their service tier, so the tier is
// recorded and the LLM's price multiplier overrides apply as they do for
// background responses. "default" and "auto" are list price: no tier.
func TestOpenAISyncServiceTier(t *testing.T) {
	v := &OpenAI{}
	llm, app := &models.LLM{Vendor: models.OPENAI}, &models.App{}
	chat := func(tier string) string {
		tf := ""
		if tier != "" {
			tf = `,"service_tier":"` + tier + `"`
		}
		return `{"id":"c1","object":"chat.completion","model":"gpt-6","choices":[{"index":0,"message":{"role":"assistant","content":"hi"}}],` +
			`"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}` + tf + `}`
	}
	resp := func(tier string) string {
		return `{"id":"resp_1","object":"response","status":"completed","model":"gpt-6","service_tier":"` + tier +
			`","output":[],"usage":{"input_tokens":10,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`
	}

	for raw, want := range map[string]string{"flex": "flex", "priority": "priority", "Priority ": "priority", "default": "", "auto": "", "": ""} {
		t.Run("chat completions "+raw, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
			_, _, got, err := v.AnalyzeResponse(llm, app, 200, []byte(chat(raw)), r)
			require.NoError(t, err)
			require.Equal(t, 10, got.GetPromptTokens())
			tier, tiered := tierOf(t, got)
			assert.Equal(t, want != "", tiered)
			assert.Equal(t, want, tier)

			stream := "data: " + `{"id":"c1","object":"chat.completion.chunk","model":"gpt-6","choices":[{"index":0,"delta":{"content":"hi"}}]` +
				map[bool]string{true: `,"service_tier":"` + raw + `"`, false: ""}[raw != ""] + "}\n\n" +
				"data: " + `{"id":"c1","object":"chat.completion.chunk","model":"gpt-6","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}` + "\n\ndata: [DONE]\n"
			_, _, got, err = v.AnalyzeStreamingResponse(llm, app, 200, []byte(stream), r, nil)
			require.NoError(t, err)
			require.Equal(t, 10, got.GetPromptTokens())
			tier, _ = tierOf(t, got)
			assert.Equal(t, want, tier)
		})
		t.Run("responses "+raw, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/v1/responses", nil)
			_, _, got, err := v.AnalyzeResponse(llm, app, 200, []byte(resp(raw)), r)
			require.NoError(t, err)
			tier, tiered := tierOf(t, got)
			require.True(t, tiered)
			assert.Equal(t, want, tier)

			stream := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + resp(raw) + "}\n\n"
			_, _, got, err = v.AnalyzeStreamingResponse(llm, app, 200, []byte(stream), r, nil)
			require.NoError(t, err)
			require.NotNil(t, got)
			tier, _ = tierOf(t, got)
			assert.Equal(t, want, tier)
		})
	}
}

func TestOpenAITierOverrideAppliesToSyncCost(t *testing.T) {
	llm := &models.LLM{PriceMultipliers: models.LLMPriceMultipliers{"flex": 0.5}}
	r := httptest.NewRequest("POST", "/v1/responses", nil)
	_, _, got, err := (&OpenAI{}).AnalyzeResponse(llm, &models.App{}, 200,
		[]byte(`{"id":"resp_1","object":"response","status":"completed","model":"gpt-6","service_tier":"flex","output":[],"usage":{"input_tokens":10,"output_tokens":2}}`), r)
	require.NoError(t, err)
	pa := got.(models.IPriceAdjustedResponse)
	assert.Equal(t, 0.5, llm.PriceMultipliers.Multiplier(pa.ServiceTier(), pa.PriceMultiplier()))
}
