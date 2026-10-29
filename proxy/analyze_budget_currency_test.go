package proxy

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

type spendRecordingBudget struct{ recorded []float64 }

func (b *spendRecordingBudget) CheckBudget(*models.App, *models.LLM) (float64, float64, error) {
	return 0, 0, nil
}
func (b *spendRecordingBudget) AnalyzeBudgetUsage(*models.App, *models.LLM) {}
func (b *spendRecordingBudget) RecordSpend(_ *models.App, _ *models.LLM, cost float64) {
	b.recorded = append(b.recorded, cost)
}

// Budgets count USD only; a request priced in another currency is not added
// to the running total (the budget's next read finds it and refuses).
func TestAnalyzeBudget_RecordsBudgetCurrencyOnly(t *testing.T) {
	b := &spendRecordingBudget{}
	for _, c := range []string{"USD", "", "EUR"} {
		analyzeBudget(b, &models.LLM{}, &models.App{}, &models.LLMChatRecord{Cost: 10000, Currency: c})
	}
	assert.Equal(t, []float64{1, 1}, b.recorded)
}
