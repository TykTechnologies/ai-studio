package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

type recordingBudget struct {
	BudgetServiceInterface
	costs []float64
}

func (b *recordingBudget) RecordUsage(_ uint, _ *uint, _ int64, cost float64, _, _ int64) error {
	b.costs = append(b.costs, cost)
	return nil
}

// Edge budgets count USD only (and rows with no currency); costs are not
// converted, so EUR spend is not added to the App's dollars. The control
// plane refuses a budget whose period has any, through the budget sync.
func TestRecordBudgetUsage_BudgetCurrencyOnly(t *testing.T) {
	b := &recordingBudget{}
	h := &MicrogatewaAnalyticsHandler{budgetService: b}
	for _, c := range []string{"USD", "", "usd", "EUR"} {
		h.recordBudgetUsage(&models.LLMChatRecord{AppID: 1, LLMID: 1, Cost: 100, Currency: c})
	}
	assert.Equal(t, []float64{100, 100, 100}, b.costs)
}
