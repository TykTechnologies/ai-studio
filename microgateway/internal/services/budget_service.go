//go:build enterprise
// +build enterprise

// internal/services/budget_service.go
package services

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// DatabaseBudgetService implements BudgetServiceInterface using database storage
type DatabaseBudgetService struct {
	db            *gorm.DB
	repo          *database.Repository
	pluginManager *plugins.PluginManager // For global data collection plugins
}

// NewDatabaseBudgetService creates a new database-backed budget service
func NewDatabaseBudgetService(db *gorm.DB, repo *database.Repository, pluginManager *plugins.PluginManager) BudgetServiceInterface {
	return &DatabaseBudgetService{
		db:            db,
		repo:          repo,
		pluginManager: pluginManager,
	}
}


// CheckBudget validates if the request is within budget limits
func (s *DatabaseBudgetService) CheckBudget(appID uint, llmID *uint, estimatedCost float64) error {
	// Get current budget period
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Get app's monthly budget
	var app database.App
	err := s.db.Where("id = ? AND is_active = ?", appID, true).First(&app).Error
	if err != nil {
		return fmt.Errorf("app not found or inactive: %w", err)
	}

	monthlyBudget := app.MonthlyBudget
	if monthlyBudget <= 0 {
		return nil // No budget limit set
	}

	// Get current usage for this period
	usage, err := s.repo.GetBudgetUsage(appID, llmID, periodStart, periodEnd)
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to get budget usage: %w", err)
	}

	currentCost := 0.0
	if usage != nil {
		// Convert from stored format (dollars * 10000) to dollars for comparison
		currentCost = usage.TotalCost / 10000.0
	}

	// Check if request would exceed budget
	if currentCost+estimatedCost > monthlyBudget {
		return fmt.Errorf("budget exceeded: current=%.2f, estimated=%.2f, limit=%.2f",
			currentCost, estimatedCost, monthlyBudget)
	}

	return nil
}

// RecordUsage records usage for budget tracking
func (s *DatabaseBudgetService) RecordUsage(appID uint, llmID *uint, tokens int64, cost float64, promptTokens, completionTokens int64) error {
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Execute budget usage data collection plugins
	if s.pluginManager != nil {
		llmIDVal := uint(0)
		if llmID != nil {
			llmIDVal = *llmID
		}
		
		// Convert to plugin format
		budgetData := &interfaces.BudgetUsageData{
			AppID:            appID,
			LLMID:            llmIDVal,
			TokensUsed:       tokens,
			Cost:             cost,
			RequestsCount:    1,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			PeriodStart:      periodStart,
			PeriodEnd:        periodEnd,
			Timestamp:        now,
			RequestID:        fmt.Sprintf("budget_%d_%d", appID, now.UnixNano()),
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

	// Update usage statistics - cost is already in stored format (dollars * 10000) from proxy layer
	err = s.repo.UpdateBudgetUsage(usage.ID, tokens, 1, cost, promptTokens, completionTokens)
	if err != nil {
		return fmt.Errorf("failed to update budget usage: %w", err)
	}

	return nil
}

// GetBudgetStatus returns current budget status for an app
func (s *DatabaseBudgetService) GetBudgetStatus(appID uint, llmID *uint) (*BudgetStatus, error) {
	// Get app details
	app, err := s.repo.GetApp(appID)
	if err != nil {
		return nil, fmt.Errorf("app not found: %w", err)
	}

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Get current usage
	usage, err := s.repo.GetBudgetUsage(appID, llmID, periodStart, periodEnd)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("failed to get budget usage: %w", err)
	}

	monthlyBudget := app.MonthlyBudget
	currentUsage := 0.0
	tokensUsed := int64(0)
	requestsCount := 0

	if usage != nil {
		// Convert from stored format (dollars * 10000) to dollars for display
		currentUsage = usage.TotalCost / 10000.0
		tokensUsed = usage.TokensUsed
		requestsCount = usage.RequestsCount
	}

	remainingBudget := monthlyBudget - currentUsage
	if remainingBudget < 0 {
		remainingBudget = 0
	}

	percentageUsed := 0.0
	if monthlyBudget > 0 {
		percentageUsed = (currentUsage / monthlyBudget) * 100
	}

	return &BudgetStatus{
		AppID:           appID,
		LLMID:           llmID,
		MonthlyBudget:   monthlyBudget,
		CurrentUsage:    currentUsage,
		RemainingBudget: remainingBudget,
		TokensUsed:      tokensUsed,
		RequestsCount:   requestsCount,
		PeriodStart:     periodStart,
		PeriodEnd:       periodEnd,
		IsOverBudget:    currentUsage > monthlyBudget,
		PercentageUsed:  percentageUsed,
	}, nil
}

// GetBudgetHistory returns budget usage history for a time period
func (s *DatabaseBudgetService) GetBudgetHistory(appID uint, llmID *uint, startTime, endTime time.Time) ([]BudgetUsage, error) {
	var usageRecords []database.BudgetUsage
	
	query := s.db.Where("app_id = ? AND period_start >= ? AND period_end <= ?", appID, startTime, endTime)
	if llmID != nil {
		query = query.Where("llm_id = ?", *llmID)
	}

	err := query.Preload("App").Preload("LLM").Order("period_start DESC").Find(&usageRecords).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get budget history: %w", err)
	}

	// Convert to service model
	result := make([]BudgetUsage, len(usageRecords))
	for i, record := range usageRecords {
		result[i] = BudgetUsage{
			ID:               record.ID,
			AppID:            record.AppID,
			LLMID:            record.LLMID,
			PeriodStart:      record.PeriodStart,
			PeriodEnd:        record.PeriodEnd,
			TokensUsed:       record.TokensUsed,
			RequestsCount:    record.RequestsCount,
			TotalCost:        record.TotalCost,
			PromptTokens:     record.PromptTokens,
			CompletionTokens: record.CompletionTokens,
			CreatedAt:        record.CreatedAt,
			UpdatedAt:        record.UpdatedAt,
		}
	}

	return result, nil
}

// UpdateBudget updates budget limits for an app
func (s *DatabaseBudgetService) UpdateBudget(appID uint, monthlyBudget float64, resetDay int) error {
	updates := map[string]interface{}{
		"monthly_budget":   monthlyBudget,
		"budget_reset_day": resetDay,
		"updated_at":       time.Now(),
	}

	result := s.db.Model(&database.App{}).Where("id = ?", appID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update budget: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("app not found: %d", appID)
	}

	return nil
}

// StartMonitoring starts budget monitoring background task
func (s *DatabaseBudgetService) StartMonitoring(ctx context.Context) {
	// This would typically run budget alerts, notifications, etc.
	// For now, it's a placeholder
}

// GetBudgetSummary returns a summary of budget usage across all apps
func (s *DatabaseBudgetService) GetBudgetSummary() (map[string]interface{}, error) {
	// Get total apps with budgets
	var appsWithBudgets int64
	err := s.db.Model(&database.App{}).
		Where("monthly_budget > 0 AND is_active = ?", true).
		Count(&appsWithBudgets).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count apps with budgets: %w", err)
	}

	// Get total budget allocated
	var totalBudgetAllocated float64
	err = s.db.Model(&database.App{}).
		Where("is_active = ?", true).
		Select("COALESCE(SUM(monthly_budget), 0)").
		Scan(&totalBudgetAllocated).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get total budget allocated: %w", err)
	}

	// Get total spent this month
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	var totalSpentStored float64
	err = s.db.Model(&database.BudgetUsage{}).
		Where("period_start >= ? AND period_end <= ?", periodStart, periodEnd).
		Select("COALESCE(SUM(total_cost), 0)").
		Scan(&totalSpentStored).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get total spent: %w", err)
	}
	// Convert from stored format (dollars * 10000) to dollars for display
	totalSpent := totalSpentStored / 10000.0

	// Get apps over budget
	// Note: monthly_budget is in dollars, total_cost is in dollars * 10000
	// We need to compare (total_cost / 10000) > monthly_budget, which is equivalent to total_cost > monthly_budget * 10000
	var appsOverBudget []struct {
		AppID         uint    `json:"app_id"`
		AppName       string  `json:"app_name"`
		MonthlyBudget float64 `json:"monthly_budget"`
		CurrentSpent  float64 `json:"current_spent"`
		OverByAmount  float64 `json:"over_by_amount"`
	}

	err = s.db.Raw(`
		SELECT
			a.id as app_id,
			a.name as app_name,
			a.monthly_budget,
			COALESCE(bu.total_cost, 0) / 10000.0 as current_spent,
			(COALESCE(bu.total_cost, 0) / 10000.0) - a.monthly_budget as over_by_amount
		FROM apps a
		LEFT JOIN budget_usage bu ON a.id = bu.app_id
			AND bu.period_start >= ? AND bu.period_end <= ?
		WHERE a.is_active = true
			AND a.monthly_budget > 0
			AND COALESCE(bu.total_cost, 0) > a.monthly_budget * 10000
		ORDER BY over_by_amount DESC
	`, periodStart, periodEnd).Scan(&appsOverBudget).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get apps over budget: %w", err)
	}

	return map[string]interface{}{
		"apps_with_budgets":     appsWithBudgets,
		"total_budget_allocated": totalBudgetAllocated,
		"total_spent_this_month": totalSpent,
		"apps_over_budget":      appsOverBudget,
		"period_start":          periodStart,
		"period_end":            periodEnd,
	}, nil
}