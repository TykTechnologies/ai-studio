package models

import "time"

// BudgetUsage represents budget usage information for an LLM or App
type BudgetUsage struct {
	EntityID        uint       `json:"entity_id"`
	Name            string     `json:"name"`
	EntityType      string     `json:"entity_type"` // "LLM" or "App"
	Budget          *float64   `json:"budget"`
	Spent           float64    `json:"spent"`
	Usage           float64    `json:"usage"` // percentage
	BudgetStartDate *time.Time `json:"budget_start_date"`
	TotalCost       float64    `json:"total_cost"`   // cost for the specified date range
	TotalTokens     int64      `json:"total_tokens"` // total tokens for the specified date range
	UserID          uint       `json:"user_id"`      // ID of the user who owns the app
	UserEmail       *string    `json:"user_email"`   // Email of the user who owns the app (pointer to handle null case)
	// PeriodStart and PeriodEnd are the current budget period Spent covers,
	// as enforcement computes it (budget.Period): PeriodEnd is the start of
	// the next period (exclusive).
	PeriodStart *time.Time `json:"period_start,omitempty"`
	PeriodEnd   *time.Time `json:"period_end,omitempty"`
	// Currency is the currency of Spent: the one currency of the period's
	// spend (a budget's currency is implied by its spend). Empty when there
	// is no spend, or when the period mixes currencies.
	Currency string `json:"currency,omitempty"`
	// MixedCurrencySpend is set when the period mixes currencies: the spend
	// by currency. Costs are not converted, so Spent and Usage are 0 and the
	// budget refuses requests.
	MixedCurrencySpend map[string]float64 `json:"mixed_currency_spend,omitempty"`
}
