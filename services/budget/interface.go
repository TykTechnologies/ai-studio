package budget

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Service defines the interface for budget management across both CE and ENT editions.
// Community Edition provides stub implementations that allow all requests.
// Enterprise Edition provides full budget enforcement, caching, and notifications.
type Service interface {
	// CheckBudget validates if a request can proceed based on budget limits.
	// Returns: spent amount, budget limit, error if budget exceeded or on failure.
	// CE: Always returns (0, 0, nil) - allows all requests
	// ENT: Enforces actual budget limits with caching
	CheckBudget(app *models.App, llm *models.LLM) (spent float64, limit float64, err error)

	// AnalyzeBudgetUsage checks current spending against thresholds and triggers notifications.
	// Should be called after recording usage to check if alerts need to be sent.
	// CE: No-op
	// ENT: Checks 80% and 100% thresholds, sends email notifications
	AnalyzeBudgetUsage(app *models.App, llm *models.LLM)

	// GetMonthlySpending returns total spending for an app in the given time period.
	// CE: Returns 0, nil
	// ENT: Queries database with caching
	GetMonthlySpending(appID uint, start, end time.Time) (float64, error)

	// GetLLMMonthlySpending returns total spending for an LLM in the given time period.
	// CE: Returns 0, nil
	// ENT: Queries database with caching
	GetLLMMonthlySpending(llmID uint, start, end time.Time) (float64, error)

	// GetBudgetUsage returns budget usage data for all apps and LLMs.
	// CE: Returns empty slice or "Enterprise feature" error
	// ENT: Returns aggregated budget data from database
	GetBudgetUsage() ([]models.BudgetUsage, error)

	// ClearCache clears any in-memory caches.
	// CE: No-op
	// ENT: Clears usage cache
	ClearCache()

	// NotifyBudgetUsage sends budget notification for the given usage and threshold.
	// CE: No-op, returns nil
	// ENT: Sends email notification
	NotifyBudgetUsage(usage *models.BudgetUsage, threshold int) error
}
