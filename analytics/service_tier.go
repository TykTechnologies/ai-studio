package analytics

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// ServiceTierNone selects the records without a service tier (list-price
// traffic, rows written before the tier was recorded, and rows an early
// build stored as "default"/"auto") in the ServiceTierUsage filter. In the
// report those rows have tier "".
const ServiceTierNone = "none"

// tierExpr reads a record's tier as the report shows it: NULL (rows older
// than the column) and the list-price names OpenAI reports ("default",
// "auto", stored by builds before they were normalised) are "".
const tierExpr = "CASE WHEN service_tier IS NULL OR service_tier IN ('default', 'auto') THEN '' ELSE service_tier END"

// ServiceTierModelUsage is one vendor/model's traffic at one service tier.
// Cost is in currency units, already at the tier's price.
type ServiceTierModelUsage struct {
	ServiceTier    string  `json:"service_tier"`
	Vendor         string  `json:"vendor"`
	Model          string  `json:"model"`
	Currency       string  `json:"currency"`
	Requests       int64   `json:"requests"`
	PromptTokens   int64   `json:"prompt_tokens"`
	ResponseTokens int64   `json:"response_tokens"`
	TotalTokens    int64   `json:"total_tokens"`
	Cost           float64 `json:"cost"`
}

// ServiceTierTotals sums one service tier over every model. Costs are keyed
// by currency.
type ServiceTierTotals struct {
	ServiceTier    string             `json:"service_tier"`
	Requests       int64              `json:"requests"`
	TotalTokens    int64              `json:"total_tokens"`
	CostByCurrency map[string]float64 `json:"cost_by_currency"`
}

// ServiceTierUsage breaks spend down by vendor service tier (batch,
// priority, flex...).
type ServiceTierUsage struct {
	ByTier  []ServiceTierTotals     `json:"by_tier"`
	ByModel []ServiceTierModelUsage `json:"by_model"`
}

// GetServiceTierUsage reports chat-record traffic between startDate and
// endDate per service tier and model, optionally for one LLM, one App and
// one tier (ServiceTierNone for records without one). Edge traffic is
// included: the pulse carries the tier.
func GetServiceTierUsage(db *gorm.DB, startDate, endDate time.Time, llmID, appID *uint, tier *string) (*ServiceTierUsage, error) {
	q := db.Model(&models.LLMChatRecord{}).
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate)
	if llmID != nil {
		q = q.Where("llm_id = ?", *llmID)
	}
	if appID != nil {
		q = q.Where("app_id = ?", *appID)
	}
	if tier != nil {
		if *tier == ServiceTierNone {
			q = q.Where("(service_tier IS NULL OR service_tier IN ('', 'default', 'auto'))")
		} else {
			q = q.Where("service_tier = ?", *tier)
		}
	}

	var rows []struct {
		ServiceTier    string
		Vendor         string
		Model          string
		Currency       string
		Requests       int64
		PromptTokens   int64
		ResponseTokens int64
		TotalTokens    int64
		Cost           float64
	}
	if err := q.
		Select(tierExpr + " AS service_tier, vendor, name AS model, currency, COUNT(*) AS requests, " +
			"COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens, COALESCE(SUM(response_tokens), 0) AS response_tokens, " +
			"COALESCE(SUM(total_tokens), 0) AS total_tokens, COALESCE(SUM(cost), 0) AS cost").
		Group(tierExpr + ", vendor, name, currency").
		Order("service_tier, cost DESC, vendor, model").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	out := &ServiceTierUsage{ByTier: []ServiceTierTotals{}, ByModel: make([]ServiceTierModelUsage, 0, len(rows))}
	tierIndex := map[string]int{}
	for _, r := range rows {
		currency := r.Currency
		if currency == "" {
			currency = "USD"
		}
		cost := r.Cost / recordedCostScale
		out.ByModel = append(out.ByModel, ServiceTierModelUsage{
			ServiceTier: r.ServiceTier, Vendor: r.Vendor, Model: r.Model, Currency: currency,
			Requests: r.Requests, PromptTokens: r.PromptTokens, ResponseTokens: r.ResponseTokens,
			TotalTokens: r.TotalTokens, Cost: cost,
		})
		i, ok := tierIndex[r.ServiceTier]
		if !ok {
			i = len(out.ByTier)
			tierIndex[r.ServiceTier] = i
			out.ByTier = append(out.ByTier, ServiceTierTotals{ServiceTier: r.ServiceTier, CostByCurrency: map[string]float64{}})
		}
		t := &out.ByTier[i]
		t.Requests += r.Requests
		t.TotalTokens += r.TotalTokens
		t.CostByCurrency[currency] += cost
	}
	return out, nil
}
