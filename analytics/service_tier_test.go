package analytics

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetServiceTierUsage(t *testing.T) {
	db := setupStatsTest(t)

	at := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	recs := []models.LLMChatRecord{
		{LLMID: 1, AppID: 10, Vendor: "anthropic", Name: "claude-luna", PromptTokens: 1000, ResponseTokens: 200, TotalTokens: 1200, Cost: 20, Currency: "USD", TimeStamp: at, ServiceTier: "batch"},
		{LLMID: 1, AppID: 11, Vendor: "anthropic", Name: "claude-luna", PromptTokens: 500, ResponseTokens: 100, TotalTokens: 600, Cost: 10, Currency: "USD", TimeStamp: at, ServiceTier: "batch"},
		{LLMID: 2, AppID: 10, Vendor: "openai", Name: "gpt-6", PromptTokens: 100, ResponseTokens: 10, TotalTokens: 110, Cost: 3, Currency: "USD", TimeStamp: at, ServiceTier: "priority"},
		{LLMID: 1, AppID: 10, Vendor: "anthropic", Name: "claude-luna", PromptTokens: 50, ResponseTokens: 50, TotalTokens: 100, Cost: 4, Currency: "USD", TimeStamp: at},
		// Out of range.
		{LLMID: 1, AppID: 10, Vendor: "anthropic", Name: "claude-luna", TotalTokens: 1, Cost: 99, Currency: "USD", TimeStamp: at.Add(-48 * time.Hour), ServiceTier: "batch"},
	}
	require.NoError(t, db.Create(&recs).Error)
	// A row written before the column existed has NULL, not "".
	legacy := models.LLMChatRecord{LLMID: 1, AppID: 10, Vendor: "anthropic", Name: "claude-luna", TotalTokens: 10, Cost: 1, Currency: "USD", TimeStamp: at}
	require.NoError(t, db.Create(&legacy).Error)
	require.NoError(t, db.Exec("UPDATE llm_chat_records SET service_tier = NULL WHERE id = ?", legacy.ID).Error)
	// Builds before the normalisation stored OpenAI's list-price tier name.
	early := models.LLMChatRecord{LLMID: 2, AppID: 10, Vendor: "openai", Name: "gpt-6", TotalTokens: 10, Cost: 1, Currency: "USD", TimeStamp: at, ServiceTier: "default"}
	require.NoError(t, db.Create(&early).Error)

	from, to := at.Add(-time.Hour), at.Add(time.Hour)
	usage, err := GetServiceTierUsage(db, from, to, nil, nil, nil)
	require.NoError(t, err)

	tiers := map[string]ServiceTierTotals{}
	for _, tt := range usage.ByTier {
		tiers[tt.ServiceTier] = tt
	}
	require.Len(t, tiers, 3, "batch, priority and no tier (NULL, \"\" and \"default\" together)")
	assert.Equal(t, int64(2), tiers["batch"].Requests)
	assert.Equal(t, int64(1800), tiers["batch"].TotalTokens)
	assert.InDelta(t, 30.0/10000, tiers["batch"].CostByCurrency["USD"], 1e-12)
	assert.Equal(t, int64(1), tiers["priority"].Requests)
	assert.Equal(t, int64(3), tiers[""].Requests)
	assert.InDelta(t, 6.0/10000, tiers[""].CostByCurrency["USD"], 1e-12)

	require.Len(t, usage.ByModel, 4)
	for _, m := range usage.ByModel {
		if m.ServiceTier == "batch" {
			assert.Equal(t, "claude-luna", m.Model)
			assert.Equal(t, int64(1500), m.PromptTokens)
			assert.Equal(t, int64(300), m.ResponseTokens)
		}
	}

	batch, none := "batch", ServiceTierNone
	usage, err = GetServiceTierUsage(db, from, to, nil, nil, &batch)
	require.NoError(t, err)
	require.Len(t, usage.ByTier, 1)
	assert.Equal(t, "batch", usage.ByTier[0].ServiceTier)

	usage, err = GetServiceTierUsage(db, from, to, nil, nil, &none)
	require.NoError(t, err)
	require.Len(t, usage.ByTier, 1)
	assert.Equal(t, "", usage.ByTier[0].ServiceTier)
	assert.Equal(t, int64(3), usage.ByTier[0].Requests)

	app := uint(11)
	usage, err = GetServiceTierUsage(db, from, to, nil, &app, nil)
	require.NoError(t, err)
	require.Len(t, usage.ByTier, 1)
	assert.Equal(t, int64(1), usage.ByTier[0].Requests)
	llm := uint(2)
	usage, err = GetServiceTierUsage(db, from, to, &llm, nil, nil)
	require.NoError(t, err)
	require.Len(t, usage.ByTier, 2, "priority and the early \"default\" row as no tier")

	usage, err = GetServiceTierUsage(db, at.Add(time.Hour), at.Add(2*time.Hour), nil, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, usage.ByTier)
	assert.Empty(t, usage.ByTier)
	assert.Empty(t, usage.ByModel)
}

// The tier report's CASE grouping, alias ordering and SUM scans must work on
// Postgres as on SQLite.
func TestGetServiceTierUsage_Postgres(t *testing.T) {
	db := postgresSchemaDB(t)
	require.NoError(t, db.AutoMigrate(&models.LLMChatRecord{}))
	at := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&[]models.LLMChatRecord{
		{LLMID: 1, AppID: 10, Vendor: "anthropic", Name: "claude-luna", TotalTokens: 1200, PromptTokens: 1000, ResponseTokens: 200, Cost: 20, Currency: "USD", TimeStamp: at, ServiceTier: "batch"},
		{LLMID: 2, AppID: 10, Vendor: "openai", Name: "gpt-6", TotalTokens: 10, Cost: 1, Currency: "USD", TimeStamp: at, ServiceTier: "default"},
		{LLMID: 2, AppID: 10, Vendor: "openai", Name: "gpt-6", TotalTokens: 10, Cost: 1, Currency: "USD", TimeStamp: at},
	}).Error)
	usage, err := GetServiceTierUsage(db, at.Add(-time.Hour), at.Add(time.Hour), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, usage.ByTier, 2)
	assert.Equal(t, "", usage.ByTier[0].ServiceTier, "ordered by tier")
	assert.Equal(t, int64(2), usage.ByTier[0].Requests)
	assert.Equal(t, "batch", usage.ByTier[1].ServiceTier)
	assert.Equal(t, int64(1000), usage.ByModel[1].PromptTokens)
	none := ServiceTierNone
	usage, err = GetServiceTierUsage(db, at.Add(-time.Hour), at.Add(time.Hour), nil, nil, &none)
	require.NoError(t, err)
	require.Len(t, usage.ByTier, 1)
	assert.Equal(t, int64(2), usage.ByTier[0].Requests)
}
