// internal/services/budget_adapter.go
package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// BudgetServiceAdapter adapts our DatabaseBudgetService to implement services.BudgetServiceInterface
type BudgetServiceAdapter struct {
	budgetService BudgetServiceInterface
	gatewayService GatewayServiceInterface
}

// NewBudgetServiceAdapter creates a new adapter that implements services.BudgetServiceInterface
func NewBudgetServiceAdapter(
	budgetService BudgetServiceInterface,
	gatewayService GatewayServiceInterface,
) services.BudgetServiceInterface {
	return &BudgetServiceAdapter{
		budgetService:  budgetService,
		gatewayService: gatewayService,
	}
}

// CheckBudget validates if the request is within budget limits
func (a *BudgetServiceAdapter) CheckBudget(app *models.App, llm *models.LLM) (float64, float64, error) {
	// Convert models to internal format and check budget
	var llmID *uint
	if llm != nil {
		llmID = &llm.ID
	}

	err := a.budgetService.CheckBudget(app.ID, llmID, 0.0) // We'll estimate cost as 0 for pre-check
	if err != nil {
		return 0, 0, err
	}

	// Get budget status to return current usage and limits
	status, err := a.budgetService.GetBudgetStatus(app.ID, llmID)
	if err != nil {
		return 0, 0, err
	}

	// Return current usage and budget limit
	currentUsage := status.CurrentUsage
	budgetLimit := status.MonthlyBudget

	return currentUsage, budgetLimit, nil
}

// AnalyzeBudgetUsage analyzes budget usage for an app and LLM combination
func (a *BudgetServiceAdapter) AnalyzeBudgetUsage(app *models.App, llm *models.LLM) {
	// This is typically called asynchronously to update budget metrics
	// For now, we'll just log that analysis was requested
	
	var llmID *uint
	if llm != nil {
		llmID = &llm.ID
	}

	// We could trigger a background analysis here, but for simplicity
	// we'll just ensure the budget status is up to date
	_, err := a.budgetService.GetBudgetStatus(app.ID, llmID)
	if err != nil {
		// Log error but don't fail since this is analysis, not blocking validation
		fmt.Printf("Budget analysis failed for app %d, llm %v: %v\n", app.ID, llmID, err)
	}
}