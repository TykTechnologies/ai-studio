//go:build !enterprise
// +build !enterprise

package services

import (
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// CommunityBudgetService implements budget recording but not enforcement for Community Edition.
// Budget enforcement (CheckBudget) is enterprise-only.
// Budget recording (RecordUsage) works in both CE and ENT for analytics purposes.
type CommunityBudgetService struct {
	db            *gorm.DB
	repo          *database.Repository
	pluginManager *plugins.PluginManager
}

// NewDatabaseBudgetService creates a community budget service.
// This records usage for analytics but does not enforce budget limits.
func NewDatabaseBudgetService(db *gorm.DB, repo *database.Repository, pluginManager *plugins.PluginManager) BudgetServiceInterface {
	return &CommunityBudgetService{
		db:            db,
		repo:          repo,
		pluginManager: pluginManager,
	}
}

// CheckBudget always allows requests in Community Edition (no enforcement).
// Enterprise Edition enforces budget limits here.
func (s *CommunityBudgetService) CheckBudget(appID uint, llmID *uint, estimatedCost float64) error {
	// CE: No budget enforcement - allow all requests
	log.Debug().
		Uint("app_id", appID).
		Float64("estimated_cost", estimatedCost).
		Msg("Budget enforcement disabled in Community Edition - allowing request")
	return nil
}

// RecordUsage records usage for budget tracking in Community Edition.
// This provides analytics data even though enforcement is disabled.
func (s *CommunityBudgetService) RecordUsage(appID uint, llmID *uint, tokens int64, cost float64, promptTokens, completionTokens int64) error {
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Execute budget usage data collection plugins
	if s.pluginManager != nil {
		llmIDValue := uint(0)
		if llmID != nil {
			llmIDValue = *llmID
		}

		budgetData := &interfaces.BudgetUsageData{
			AppID:            appID,
			LLMID:            llmIDValue,
			TokensUsed:       tokens,
			Cost:             cost,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			PeriodStart:      periodStart,
			PeriodEnd:        periodEnd,
			Timestamp:        now,
			RequestID:        fmt.Sprintf("budget_%d_%d", appID, now.UnixNano()),
			RequestsCount:    1,
		}

		// Execute budget plugins
		if err := s.pluginManager.ExecuteDataCollectionPlugins("budget", budgetData); err != nil {
			log.Error().Err(err).Msg("Failed to execute budget data collection plugins")
		}

		// Check if any plugins are configured to replace database storage for budget
		if s.pluginManager.ShouldReplaceDatabaseStorage("budget") {
			log.Debug().Msg("Budget database storage replaced by plugin - skipping database write")
			return nil
		}
	}

	// Get or create usage record
	usage, err := s.repo.GetOrCreateBudgetUsage(appID, llmID, periodStart, periodEnd)
	if err != nil {
		return fmt.Errorf("failed to get/create budget usage: %w", err)
	}

	// Update usage statistics
	err = s.repo.UpdateBudgetUsage(usage.ID, tokens, 1, cost, promptTokens, completionTokens)
	if err != nil {
		return fmt.Errorf("failed to update budget usage: %w", err)
	}

	log.Debug().
		Uint("app_id", appID).
		Float64("cost", cost).
		Int64("tokens", tokens).
		Msg("Budget usage recorded in Community Edition (enforcement disabled)")

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
