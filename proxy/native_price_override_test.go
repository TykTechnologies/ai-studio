package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

// An LLM's per-tier price multiplier override replaces the vendor's batch
// multiplier (0.5) in the recorded cost and the budget charge; a tier the
// override does not name keeps the vendor's.
func TestNative_PriceMultiplierOverride(t *testing.T) {
	for name, tc := range map[string]struct {
		overrides models.LLMPriceMultipliers
		want      float64
	}{
		"batch override":         {overrides: models.LLMPriceMultipliers{"batch": 0.25}, want: 0.25},
		"other tier only":        {overrides: models.LLMPriceMultipliers{"priority": 2}, want: 0.5},
		"no override":            {overrides: nil, want: 0.5},
		"zero override is free":  {overrides: models.LLMPriceMultipliers{"batch": 0}, want: 0},
		"above list is accepted": {overrides: models.LLMPriceMultipliers{"batch": 1.2}, want: 1.2},
	} {
		t.Run(name, func(t *testing.T) {
			spend := &spendCounter{}
			h := newNativeHarness(t, nativeHarnessOpts{
				mutateLLM:  func(l *models.LLM) { l.PriceMultipliers = tc.overrides },
				wrapBudget: func(s budget.Service) budget.Service { spend.Service = s; return spend },
			})
			h.addPrice("claude-luna", 0.00001, 0.000002)
			h.vendor.addBatch("msgbatch_P1", "ended")
			h.vendor.setResults("msgbatch_P1", succeededLine("a", "claude-luna", 1000, 200))
			row := h.seedBatchRow(h.llm, h.appA, "msgbatch_P1")

			out, err := h.proxy.accountNativeRow(context.Background(), row)
			require.NoError(t, err)
			require.Equal(t, nativeAccountAccounted, out)

			var recs []models.LLMChatRecord
			require.Eventually(t, func() bool { recs = h.chatRecords(h.appA); return len(recs) == 1 }, 5*time.Second, 20*time.Millisecond)
			list := 200*0.00001 + 1000*0.000002
			assert.InDelta(t, list*tc.want*10000, recs[0].Cost, 1e-9)
			total, _ := spend.totals()
			assert.InDelta(t, list*tc.want, total, 1e-12, "the budget is charged the overridden price")
		})
	}
}
