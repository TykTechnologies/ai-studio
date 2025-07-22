package aigateway

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// DatabaseBudgetService implements GatewayBudgetServiceInterface using the existing budget service.
// This provides a bridge between the new interface-based architecture and the current
// database-backed budget implementation.
type DatabaseBudgetService struct {
	budgetService *services.BudgetService
}

// NewDatabaseBudgetService creates a new DatabaseBudgetService that wraps the existing budget service.
func NewDatabaseBudgetService(budgetService *services.BudgetService) GatewayBudgetServiceInterface {
	return &DatabaseBudgetService{
		budgetService: budgetService,
	}
}

// CheckBudget verifies if a request would exceed either App or LLM budget by delegating to the existing budget service.
func (d *DatabaseBudgetService) CheckBudget(app *models.App, llm *models.LLM) (float64, float64, error) {
	return d.budgetService.CheckBudget(app, llm)
}

// AnalyzeBudgetUsage analyzes current budget usage and sends notifications if thresholds are reached.
func (d *DatabaseBudgetService) AnalyzeBudgetUsage(app *models.App, llm *models.LLM) {
	d.budgetService.AnalyzeBudgetUsage(app, llm)
}
