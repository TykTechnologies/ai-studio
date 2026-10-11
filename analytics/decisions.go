package analytics

import (
	"sort"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// DecisionTotals summarises decision-model traffic over a period. Costs are
// in currency units (dollars), keyed by currency.
type DecisionTotals struct {
	Requests       int64              `json:"requests"`
	Questions      int64              `json:"questions"`
	Answered       int64              `json:"answered"`
	Refusals       int64              `json:"refusals"`
	InputTokens    int64              `json:"input_tokens"`
	CostByCurrency map[string]float64 `json:"cost_by_currency"`
}

// DecisionDaySeries is decision traffic per day, aligned on Labels (dates).
// Cost sums every currency; it is a trend line, not an invoice.
type DecisionDaySeries struct {
	Labels    []string  `json:"labels"`
	Requests  []int64   `json:"requests"`
	Questions []int64   `json:"questions"`
	Refusals  []int64   `json:"refusals"`
	Cost      []float64 `json:"cost"`
}

// DecisionModelUsage is decision traffic for one vendor/model.
type DecisionModelUsage struct {
	Vendor      string  `json:"vendor"`
	Model       string  `json:"model"`
	Currency    string  `json:"currency"`
	Requests    int64   `json:"requests"`
	Questions   int64   `json:"questions"`
	Refusals    int64   `json:"refusals"`
	InputTokens int64   `json:"input_tokens"`
	Cost        float64 `json:"cost"`
	// CostPerThousand is the cost of a thousand decision requests at this
	// period's average, the number decision pricing is usually compared on.
	CostPerThousand float64 `json:"cost_per_thousand"`
}

// DecisionUsage is the decision-model report for a period.
type DecisionUsage struct {
	Totals  DecisionTotals       `json:"totals"`
	ByDay   DecisionDaySeries    `json:"by_day"`
	ByModel []DecisionModelUsage `json:"by_model"`
}

// GetDecisionUsage reports decision-model traffic between startDate and
// endDate, optionally for one LLM and/or one App. Decision calls are the chat
// records with request_kind "decision"; their answered questions are the
// records' choices.
func GetDecisionUsage(db *gorm.DB, startDate, endDate time.Time, llmID, appID *uint) (*DecisionUsage, error) {
	base := func() *gorm.DB {
		q := db.Model(&models.LLMChatRecord{}).
			Where("request_kind = ?", models.DecisionRequest).
			Where("time_stamp BETWEEN ? AND ?", startDate, endDate)
		if llmID != nil {
			q = q.Where("llm_id = ?", *llmID)
		}
		if appID != nil {
			q = q.Where("app_id = ?", *appID)
		}
		return q
	}

	var byModel []struct {
		Vendor      string
		Model       string
		Currency    string
		Requests    int64
		Questions   int64
		Answered    int64
		Refusals    int64
		InputTokens int64
		Cost        float64
	}
	if err := base().
		Select("vendor, name AS model, currency, COUNT(*) AS requests, " +
			"COALESCE(SUM(decision_questions), 0) AS questions, COALESCE(SUM(choices), 0) AS answered, " +
			"COALESCE(SUM(decision_refusals), 0) AS refusals, COALESCE(SUM(prompt_tokens), 0) AS input_tokens, " +
			"COALESCE(SUM(cost), 0) AS cost").
		Group("vendor, name, currency").
		Find(&byModel).Error; err != nil {
		return nil, err
	}

	var byDay []struct {
		Date      string
		Requests  int64
		Questions int64
		Refusals  int64
		Cost      float64
	}
	if err := base().
		Select("DATE(time_stamp) AS date, COUNT(*) AS requests, COALESCE(SUM(decision_questions), 0) AS questions, " +
			"COALESCE(SUM(decision_refusals), 0) AS refusals, COALESCE(SUM(cost), 0) AS cost").
		Group("DATE(time_stamp)").
		Order("date").
		Find(&byDay).Error; err != nil {
		return nil, err
	}

	out := &DecisionUsage{
		Totals: DecisionTotals{CostByCurrency: map[string]float64{}},
		ByDay: DecisionDaySeries{
			Labels: []string{}, Requests: []int64{}, Questions: []int64{}, Refusals: []int64{}, Cost: []float64{},
		},
		ByModel: make([]DecisionModelUsage, 0, len(byModel)),
	}
	for _, m := range byModel {
		cost := m.Cost / 10000
		currency := m.Currency
		if currency == "" {
			currency = "USD"
		}
		usage := DecisionModelUsage{
			Vendor: m.Vendor, Model: m.Model, Currency: currency,
			Requests: m.Requests, Questions: m.Questions, Refusals: m.Refusals,
			InputTokens: m.InputTokens, Cost: cost,
		}
		if m.Requests > 0 {
			usage.CostPerThousand = cost / float64(m.Requests) * 1000
		}
		out.ByModel = append(out.ByModel, usage)

		out.Totals.Requests += m.Requests
		out.Totals.Questions += m.Questions
		out.Totals.Answered += m.Answered
		out.Totals.Refusals += m.Refusals
		out.Totals.InputTokens += m.InputTokens
		out.Totals.CostByCurrency[currency] += cost
	}
	sort.Slice(out.ByModel, func(i, j int) bool {
		if out.ByModel[i].Requests != out.ByModel[j].Requests {
			return out.ByModel[i].Requests > out.ByModel[j].Requests
		}
		return out.ByModel[i].Vendor+"/"+out.ByModel[i].Model < out.ByModel[j].Vendor+"/"+out.ByModel[j].Model
	})
	for _, d := range byDay {
		out.ByDay.Labels = append(out.ByDay.Labels, d.Date)
		out.ByDay.Requests = append(out.ByDay.Requests, d.Requests)
		out.ByDay.Questions = append(out.ByDay.Questions, d.Questions)
		out.ByDay.Refusals = append(out.ByDay.Refusals, d.Refusals)
		out.ByDay.Cost = append(out.ByDay.Cost, d.Cost/10000)
	}
	return out, nil
}
