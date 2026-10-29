package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// kq3b.4 on the edge: the config sync's budget period (and the budget
// check's, pricing_regression_ent_test.go) gives, for every anchor day and
// every day of three years (one leap), a period that contains now, starts on
// the anchor day or the month's last day when the month is shorter, and
// follows on from the previous period with no gap, as the hub computes it.
func TestPricingRegression_EdgeSyncBudgetPeriodAnchors(t *testing.T) {
	assertBudgetPeriodAnchors(t, calculateBudgetPeriod)
}

func assertBudgetPeriodAnchors(t *testing.T, period func(*time.Time, time.Time) (time.Time, time.Time)) {
	t.Helper()
	for anchorDay := 1; anchorDay <= 31; anchorDay++ {
		anchor := time.Date(2025, time.January, anchorDay, 0, 0, 0, 0, time.UTC)
		var prevEnd time.Time
		bad := 0
		for now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC); now.Year() < 2029 && bad < 3; now = now.AddDate(0, 0, 1) {
			start, end := period(&anchor, now)
			last := time.Date(start.Year(), start.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
			day := now.Format("2006-01-02")
			ok := assert.False(t, start.After(now), "anchor %d, now %s: period starts %s, in the future", anchorDay, day, start) &&
				assert.False(t, end.Before(now), "anchor %d, now %s: period ended %s", anchorDay, day, end) &&
				assert.Equal(t, min(anchorDay, last), start.Day(), "anchor %d, now %s: period starts %s", anchorDay, day, start)
			if !prevEnd.IsZero() && !end.Equal(prevEnd) {
				ok = assert.Equal(t, prevEnd.Add(time.Second), start, "anchor %d, now %s: the period follows on from the one that ended %s", anchorDay, day, prevEnd) && ok
			}
			if !ok {
				bad++
			}
			prevEnd = end
		}
	}
}
