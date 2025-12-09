package budget

import (
	"errors"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

var (
	// ErrEnterpriseFeature is returned when attempting to use enterprise-only features in CE
	ErrEnterpriseFeature = errors.New("budget management is an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")
)

// communityService is a stub implementation of the budget service for Community Edition.
// It allows all requests to proceed (no budget enforcement) and returns errors for
// features that require Enterprise Edition.
type communityService struct{}

// newCommunityService creates a new community edition budget service stub.
func newCommunityService() Service {
	return &communityService{}
}

// CheckBudget always allows requests in Community Edition (no budget enforcement).
func (s *communityService) CheckBudget(app *models.App, llm *models.LLM) (float64, float64, error) {
	// CE: No budget enforcement - allow all requests
	return 0, 0, nil
}

// AnalyzeBudgetUsage is a no-op in Community Edition.
func (s *communityService) AnalyzeBudgetUsage(app *models.App, llm *models.LLM) {
	// CE: No budget analysis
}

// GetMonthlySpending returns 0 in Community Edition.
func (s *communityService) GetMonthlySpending(appID uint, start, end time.Time) (float64, error) {
	// CE: No spending tracking
	return 0, nil
}

// GetLLMMonthlySpending returns 0 in Community Edition.
func (s *communityService) GetLLMMonthlySpending(llmID uint, start, end time.Time) (float64, error) {
	// CE: No spending tracking
	return 0, nil
}

// GetBudgetUsage returns an enterprise feature error in Community Edition.
func (s *communityService) GetBudgetUsage() ([]models.BudgetUsage, error) {
	return nil, ErrEnterpriseFeature
}

// ClearCache is a no-op in Community Edition.
func (s *communityService) ClearCache() {
	// CE: No cache to clear
}

// NotifyBudgetUsage is a no-op in Community Edition.
func (s *communityService) NotifyBudgetUsage(usage *models.BudgetUsage, threshold int) error {
	// CE: No notifications
	return nil
}
