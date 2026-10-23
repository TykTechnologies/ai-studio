package analytics

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/require"
)

// The analytics date range is half-open, [start, end): the API's end is the
// midnight after end_date, so a call in the last sub-second of end_date
// counts and one at the next midnight does not (midsommar-clean-5h8).
func TestStatsEndBoundIsExclusive(t *testing.T) {
	start := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC) // end_date 2026-01-12, exclusive
	inside := []time.Time{start, end.Add(-500 * time.Millisecond)}
	outside := end

	seed := func(t *testing.T, at []time.Time) *gorm.DB {
		db := setupStatsTest(t)
		for _, ts := range at {
			require.NoError(t, db.Create(&models.LLMChatRecord{Name: "gpt", Vendor: "openai", LLMID: 1, AppID: 1, UserID: 1,
				ChatID: "chat-1", PromptTokens: 4, ResponseTokens: 6, TotalTokens: 10, Cost: 10000, Currency: "USD",
				Choices: 1, InteractionType: models.ProxyInteraction, RequestKind: models.DecisionRequest, TimeStamp: ts}).Error)
			require.NoError(t, db.Create(&models.ToolCallRecord{ToolID: 1, Name: "op", TimeStamp: ts}).Error)
			require.NoError(t, db.Create(&models.ProxyLog{AppID: 1, LLMID: 1, Vendor: "openai", TimeStamp: ts}).Error)
		}
		return db
	}
	sum := func(xs []float64) float64 {
		var s float64
		for _, x := range xs {
			s += x
		}
		return s
	}
	chart := func(c *ChartData, err error) float64 { require.NoError(t, err); return sum(c.Data) }
	multi := func(m *models.MultiAxisChartData, err error) float64 {
		require.NoError(t, err)
		var s float64
		for _, d := range m.Datasets {
			s += sum(d.Data)
		}
		return s
	}
	id := uint(1)

	cases := map[string]func(db *gorm.DB, s, e time.Time) float64{
		"GetChatRecordsPerDay":  func(db *gorm.DB, s, e time.Time) float64 { return chart(GetChatRecordsPerDay(db, &s, &e)) },
		"GetToolCallsPerDay":    func(db *gorm.DB, s, e time.Time) float64 { return chart(GetToolCallsPerDay(db, s, e)) },
		"GetChatRecordsPerUser": func(db *gorm.DB, s, e time.Time) float64 { return chart(GetChatRecordsPerUser(db, s, e)) },
		"GetCostAnalysis": func(db *gorm.DB, s, e time.Time) float64 {
			m, err := GetCostAnalysis(db, s, e, nil)
			require.NoError(t, err)
			var total float64
			for _, c := range m {
				total += sum(c.Data)
			}
			return total
		},
		"GetMostUsedLLMModels":   func(db *gorm.DB, s, e time.Time) float64 { return chart(GetMostUsedLLMModels(db, s, e, nil)) },
		"GetToolUsageStatistics": func(db *gorm.DB, s, e time.Time) float64 { return chart(GetToolUsageStatistics(db, s, e)) },
		"GetToolOperationsUsageOverTime": func(db *gorm.DB, s, e time.Time) float64 {
			return multi(GetToolOperationsUsageOverTime(db, 1, s, e))
		},
		"GetUniqueUsersPerDay":       func(db *gorm.DB, s, e time.Time) float64 { return chart(GetUniqueUsersPerDay(db, s, e)) },
		"GetTokenUsagePerUser":       func(db *gorm.DB, s, e time.Time) float64 { return chart(GetTokenUsagePerUser(db, s, e, nil)) },
		"GetTokenUsagePerApp":        func(db *gorm.DB, s, e time.Time) float64 { return chart(GetTokenUsagePerApp(db, s, e, nil)) },
		"GetTokenUsageForApp":        func(db *gorm.DB, s, e time.Time) float64 { return chart(GetTokenUsageForApp(db, s, e, 1)) },
		"GetChatInteractionsForChat": func(db *gorm.DB, s, e time.Time) float64 { return chart(GetChatInteractionsForChat(db, s, e, "chat-1")) },
		"GetAppInteractionsOverTime": func(db *gorm.DB, s, e time.Time) float64 { return chart(GetAppInteractionsOverTime(db, s, e, 1)) },
		"GetModelUsage":              func(db *gorm.DB, s, e time.Time) float64 { return chart(GetModelUsage(db, s, e, "gpt")) },
		"GetVendorUsage":             func(db *gorm.DB, s, e time.Time) float64 { return chart(GetVendorUsage(db, s, e, "openai", nil)) },
		"GetUsage": func(db *gorm.DB, s, e time.Time) float64 {
			return multi(GetUsage(db, s, e, "", nil, nil, nil, ""))
		},
		"GetTokenUsageAndCostForApp": func(db *gorm.DB, s, e time.Time) float64 { return multi(GetTokenUsageAndCostForApp(db, s, e, 1)) },
		"GetTotalCostPerVendorAndModel": func(db *gorm.DB, s, e time.Time) float64 {
			costs, err := GetTotalCostPerVendorAndModel(db, s, e, nil, nil)
			require.NoError(t, err)
			var total float64
			for _, c := range costs {
				total += float64(c.TotalTokens)
			}
			return total
		},
		"GetAppsForModel": func(db *gorm.DB, s, e time.Time) float64 {
			apps, err := GetAppsForModel(db, s, e, 1, "gpt", nil)
			require.NoError(t, err)
			var total float64
			for _, a := range apps {
				total += float64(a.RequestCount)
			}
			return total
		},
		"GetProxyLogsForAppID": func(db *gorm.DB, s, e time.Time) float64 {
			_, n, err := GetProxyLogsForAppID(db, s, e, 1, 1, 10, "")
			require.NoError(t, err)
			return float64(n)
		},
		"GetProxyLogsForLLM": func(db *gorm.DB, s, e time.Time) float64 {
			_, n, err := GetProxyLogsForLLM(db, s, e, 1, 1, 10, "")
			require.NoError(t, err)
			return float64(n)
		},
		"GetDecisionUsage": func(db *gorm.DB, s, e time.Time) float64 {
			u, err := GetDecisionUsage(db, s, e, &id, nil)
			require.NoError(t, err)
			return float64(u.Totals.Requests)
		},
		"GetServiceTierUsage": func(db *gorm.DB, s, e time.Time) float64 {
			u, err := GetServiceTierUsage(db, s, e, &id, nil, nil)
			require.NoError(t, err)
			var total float64
			for _, r := range u.ByTier {
				total += float64(r.Requests)
			}
			return total
		},
		// GetBudgetUsage takes the inclusive end day, as /analytics/budget-usage does.
		"GetBudgetUsage": func(db *gorm.DB, s, e time.Time) float64 {
			lastDay := e.AddDate(0, 0, -1)
			usage, err := GetBudgetUsage(db, &s, &lastDay, nil)
			require.NoError(t, err)
			var total float64
			for _, u := range usage {
				if u.EntityType == "LLM" {
					total += float64(u.TotalTokens)
				}
			}
			return total
		},
	}

	for name, measure := range cases {
		t.Run(name, func(t *testing.T) {
			// What the two calls inside the range amount to, read with a range
			// wide enough to hold them whatever its bound.
			want := measure(seed(t, inside), start.AddDate(0, 0, -1), end.AddDate(0, 0, 2))
			require.NotZero(t, want)
			got := measure(seed(t, append(append([]time.Time{}, inside...), outside)), start, end)
			require.Equal(t, want, got, "the call at 23:59:59.5 counts, the one at the next midnight does not")
		})
	}
}
