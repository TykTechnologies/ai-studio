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
}
