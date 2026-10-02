package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recalculation runs Postgres-only SQL (::DECIMAL), so it is tested here.
// It must bill cache tokens the way the gateway does when it records them:
// for OpenAI an unset (zero) cache price falls back to the input price.
func TestUpdateModelPriceAndRecalculate_UnsetCachePriceBillsAtInputPrice(t *testing.T) {
	s := setupPostgresServiceTest(t)

	model := fmt.Sprintf("pg-cache-price-%d", time.Now().UnixNano())
	vendor := string(models.OPENAI)
	t.Cleanup(func() {
		s.DB.Exec("DELETE FROM llm_chat_records WHERE name = ?", model)
		s.DB.Unscoped().Exec("DELETE FROM model_prices WHERE model_name = ?", model)
	})

	mp := &models.ModelPrice{ModelName: model, Vendor: vendor, CPT: 0.000006, CPIT: 0.000002, Currency: "USD"}
	require.NoError(t, mp.Create(s.DB))
	rec := models.LLMChatRecord{
		Name: model, Vendor: vendor,
		PromptTokens: 2, ResponseTokens: 6, CacheWritePromptTokens: 14, CacheReadPromptTokens: 6004,
		TotalTokens: 6026, Currency: "USD", TimeStamp: time.Now(),
	}
	require.NoError(t, s.DB.Create(&rec).Error)

	_, err := s.UpdateModelPriceAndRecalculate(mp.ID, model, vendor, 0.000006, 0.000002, 0, 0, "USD")
	require.NoError(t, err)

	var got models.LLMChatRecord
	require.NoError(t, s.DB.First(&got, rec.ID).Error)
	// completion 6×0.000006 + all 6020 prompt tokens ×0.000002 = 0.012076, ×10000.
	assert.InDelta(t, 120.76, got.Cost, 1e-6)
}
