//go:build enterprise

package services

import "testing"

// kq3b.4: the edge budget check's period, as the config sync's
// (TestPricingRegression_EdgeSyncBudgetPeriodAnchors).
func TestPricingRegression_EdgeBudgetCheckPeriodAnchors(t *testing.T) {
	assertBudgetPeriodAnchors(t, (&DatabaseBudgetService{}).calculateBudgetPeriod)
}
