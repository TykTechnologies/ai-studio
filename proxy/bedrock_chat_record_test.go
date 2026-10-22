package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// A Bedrock chat record is built like every other vendor's: for the same
// usage and prices it matches an Anthropic record in cost (cache tokens at
// their cache prices), total tokens and currency (jyx.78).
func TestBedrockChatRecord_MatchesOtherVendors(t *testing.T) {
	const model = "anthropic.claude-3"
	price := func(vendor string) *models.ModelPrice {
		return &models.ModelPrice{ModelName: model, Vendor: vendor, CPT: 0.000015, CPIT: 0.000003,
			CacheWritePT: 0.00000375, CacheReadPT: 0.0000003, Currency: "USD"}
	}
	svc := new(MockService)
	svc.On("GetModelPriceByModelNameAndVendor", model, string(models.BEDROCK)).Return(price(string(models.BEDROCK)), nil)
	svc.On("GetModelPriceByModelNameAndVendor", model, string(models.ANTHROPIC)).Return(price(string(models.ANTHROPIC)), nil)
	p := &Proxy{gatewayService: svc}
	app := &models.App{ID: 7, UserID: 9}
	now := time.Now()

	bedrock := bedrockChatRecord(p, &models.LLM{ID: 1, Vendor: models.BEDROCK}, app, model, 1000, 200, 400, 3000, now)
	anthropic := buildChatRecordFor(svc, &models.LLM{ID: 1, Vendor: models.ANTHROPIC}, app,
		&MockTokenResponse{model: model, promptTokens: 1000, responseTokens: 200, cacheWriteTokens: 400, cacheReadTokens: 3000, choiceCount: 1}, model, now)
	require.NotNil(t, bedrock)

	// 200×0.000015 + 1000×0.000003 + 400×0.00000375 + 3000×0.0000003 = 0.0084, ×10000.
	assert.InDelta(t, 84, bedrock.Cost, 1e-9)
	assert.InDelta(t, anthropic.Cost, bedrock.Cost, 1e-9)
	assert.Equal(t, anthropic.TotalTokens, bedrock.TotalTokens, "cache tokens count in the total")
	assert.Equal(t, 4600, bedrock.TotalTokens)
	assert.Equal(t, anthropic.Currency, bedrock.Currency)
	assert.Equal(t, 1000, bedrock.PromptTokens)
	assert.Equal(t, 400, bedrock.CacheWritePromptTokens)
	assert.Equal(t, 3000, bedrock.CacheReadPromptTokens)
	assert.Equal(t, 1, bedrock.Choices)
	assert.Equal(t, uint(7), bedrock.AppID)
	assert.Equal(t, uint(9), bedrock.UserID)
	assert.Equal(t, models.ProxyInteraction, bedrock.InteractionType)

	assert.Nil(t, bedrockChatRecord(p, &models.LLM{Vendor: models.BEDROCK}, app, model, 0, 0, 0, 0, now), "no usage, no record")
}

// An unpriced Bedrock model still records usage, in the default currency.
func TestBedrockChatRecord_UnpricedModel(t *testing.T) {
	svc := new(MockService)
	svc.On("GetModelPriceByModelNameAndVendor", "unpriced", string(models.BEDROCK)).Return(nil, assert.AnError)
	rec := bedrockChatRecord(&Proxy{gatewayService: svc}, &models.LLM{Vendor: models.BEDROCK}, &models.App{}, "unpriced", 10, 5, 0, 0, time.Now())
	require.NotNil(t, rec)
	assert.Zero(t, rec.Cost)
	assert.Equal(t, "USD", rec.Currency)
	assert.Equal(t, 15, rec.TotalTokens)
}
