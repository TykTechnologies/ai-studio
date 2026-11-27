package analytics

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// recordedCostScale is the unit LLMChatRecord.Cost is stored in: currency
// units x 10000, as the gateway writes it (proxy.buildChatRecord), the edge
// pulses it (AnalyticsEvent.cost) and every report in stats.go reads it. The
// decision report returns currency units, like the others.
const recordedCostScale = 10000

// A decision call was answered when the model returned answers: choices or
// refusals. The failed-call test is the one the decision-question metric
// uses (recordChatRecordMetrics).
const (
	answeredCall      = "(choices + decision_refusals) > 0"
	answeredCount     = "COALESCE(SUM(CASE WHEN " + answeredCall + " THEN 1 ELSE 0 END), 0)"
	failedCount       = "COALESCE(SUM(CASE WHEN " + answeredCall + " THEN 0 ELSE 1 END), 0)"
	answeredQuestions = "COALESCE(SUM(CASE WHEN " + answeredCall + " THEN decision_questions ELSE 0 END), 0)"
)

// DecisionTotals summarises decision-model traffic over a period. Costs are
// in currency units (dollars), keyed by currency. Requests are the calls the
// model answered; Failed are the calls that came back without answers (an
// upstream error such as model_not_found, a budget denial, a policy block).
// Questions, Answered and Refusals count answered calls only, so a failed
// call's questions are not read as unanswered ones.
type DecisionTotals struct {
	Requests       int64              `json:"requests"`
	Failed         int64              `json:"failed"`
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
	Failed    []int64   `json:"failed"`
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
	Failed      int64   `json:"failed"`
	Questions   int64   `json:"questions"`
	Refusals    int64   `json:"refusals"`
	InputTokens int64   `json:"input_tokens"`
	Cost        float64 `json:"cost"`
	// CostPerThousand is the cost of a thousand answered decision requests
	// at this period's average, the number decision pricing is usually
	// compared on.
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
// records' choices. A record with neither choices nor refusals is a failed
// call: the gateway records failed exchanges too, with the questions the
// request asked (responses.DecisionResponse), and the model answered none.
func GetDecisionUsage(db *gorm.DB, startDate, endDate time.Time, llmID, appID *uint) (*DecisionUsage, error) {
	base := func() *gorm.DB {
		q := db.Model(&models.LLMChatRecord{}).
			Where("request_kind = ?", models.DecisionRequest).
			Where("time_stamp >= ? AND time_stamp < ?", startDate, endDate)
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
		Failed      int64
		Questions   int64
		Answered    int64
		Refusals    int64
		InputTokens int64
		Cost        float64
	}
	if err := base().
		Select("vendor, name AS model, currency, " + answeredCount + " AS requests, " + failedCount + " AS failed, " +
			answeredQuestions + " AS questions, COALESCE(SUM(choices), 0) AS answered, " +
			"COALESCE(SUM(decision_refusals), 0) AS refusals, COALESCE(SUM(prompt_tokens), 0) AS input_tokens, " +
			"COALESCE(SUM(cost), 0) AS cost").
		Group("vendor, name, currency").
		Order("requests DESC, failed DESC, vendor, name").
		Find(&byModel).Error; err != nil {
		return nil, err
	}

	var byDay []struct {
		Date      string
		Requests  int64
		Failed    int64
		Questions int64
		Refusals  int64
		Cost      float64
	}
	if err := base().
		Select("DATE(time_stamp) AS date, " + answeredCount + " AS requests, " + failedCount + " AS failed, " +
			answeredQuestions + " AS questions, " +
			"COALESCE(SUM(decision_refusals), 0) AS refusals, COALESCE(SUM(cost), 0) AS cost").
		Group("DATE(time_stamp)").
		Order("date").
		Find(&byDay).Error; err != nil {
		return nil, err
	}

	out := &DecisionUsage{
		Totals: DecisionTotals{CostByCurrency: map[string]float64{}},
		ByDay: DecisionDaySeries{
			Labels: []string{}, Requests: []int64{}, Failed: []int64{}, Questions: []int64{}, Refusals: []int64{}, Cost: []float64{},
		},
		ByModel: make([]DecisionModelUsage, 0, len(byModel)),
	}
	for _, m := range byModel {
		cost := m.Cost / recordedCostScale
		currency := m.Currency
		if currency == "" {
			currency = "USD"
		}
		usage := DecisionModelUsage{
			Vendor: m.Vendor, Model: m.Model, Currency: currency,
			Requests: m.Requests, Failed: m.Failed, Questions: m.Questions, Refusals: m.Refusals,
			InputTokens: m.InputTokens, Cost: cost,
		}
		if m.Requests > 0 {
			usage.CostPerThousand = cost / float64(m.Requests) * 1000
		}
		out.ByModel = append(out.ByModel, usage)

		out.Totals.Requests += m.Requests
		out.Totals.Failed += m.Failed
		out.Totals.Questions += m.Questions
		out.Totals.Answered += m.Answered
		out.Totals.Refusals += m.Refusals
		out.Totals.InputTokens += m.InputTokens
		out.Totals.CostByCurrency[currency] += cost
	}
	for _, d := range byDay {
		out.ByDay.Labels = append(out.ByDay.Labels, d.Date)
		out.ByDay.Requests = append(out.ByDay.Requests, d.Requests)
		out.ByDay.Failed = append(out.ByDay.Failed, d.Failed)
		out.ByDay.Questions = append(out.ByDay.Questions, d.Questions)
		out.ByDay.Refusals = append(out.ByDay.Refusals, d.Refusals)
		out.ByDay.Cost = append(out.ByDay.Cost, d.Cost/recordedCostScale)
	}
	return out, nil
}
