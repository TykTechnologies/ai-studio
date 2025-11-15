//go:build !enterprise
// +build !enterprise

package services

import (
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"gorm.io/gorm"
)

// CommunityBudgetService is a stub implementation for Community Edition.
// It allows all requests to proceed without budget enforcement.
type CommunityBudgetService struct{}

// NewDatabaseBudgetService creates a community budget service stub.
// This is called in CE builds where budget enforcement is not available.
func NewDatabaseBudgetService(db *gorm.DB, repo *database.Repository, pluginManager *plugins.PluginManager) BudgetServiceInterface {
	return &CommunityBudgetService{}
}

// CheckBudget always allows requests in Community Edition (no enforcement).
func (s *CommunityBudgetService) CheckBudget(appID uint, llmID *uint, estimatedCost float64) error {
	// CE: No budget enforcement - allow all requests
	return nil
}

// RecordUsage is a no-op in Community Edition.
func (s *CommunityBudgetService) RecordUsage(appID uint, llmID *uint, tokens int64, cost float64, promptTokens, completionTokens int64) error {
	// CE: No usage recording
	return nil
}

// GetBudgetStatus returns nil in Community Edition.
func (s *CommunityBudgetService) GetBudgetStatus(appID uint, llmID *uint) (*BudgetStatus, error) {
	// CE: No budget status available
	return nil, nil
}

// GetBudgetHistory returns empty slice in Community Edition.
func (s *CommunityBudgetService) GetBudgetHistory(appID uint, llmID *uint, startTime, endTime time.Time) ([]BudgetUsage, error) {
	// CE: No budget history
	return []BudgetUsage{}, nil
}

// UpdateBudget is a no-op in Community Edition.
func (s *CommunityBudgetService) UpdateBudget(appID uint, monthlyBudget float64, resetDay int) error {
	// CE: No budget updates
	return nil
}
