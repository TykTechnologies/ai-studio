package services

import (
	"bytes"
	"fmt"
	"sync"
	"text/template"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/notifications"
	"gorm.io/gorm"
)

type usageKey struct {
	entityID   uint
	entityType string    // "App" or "LLM"
	startDate  time.Time // budget period start
}

type usageData struct {
	spent    float64
	cachedAt time.Time
}

type BudgetService struct {
	db            *gorm.DB
	usageCache    map[usageKey]usageData
	cacheMutex    sync.RWMutex
	cacheDuration time.Duration
	mailService   *notifications.MailService
}

// NewBudgetService returns our unified budget service
func NewBudgetService(db *gorm.DB, mailService *notifications.MailService) *BudgetService {
	s := &BudgetService{
		db:            db,
		usageCache:    make(map[usageKey]usageData),
		cacheMutex:    sync.RWMutex{},
		cacheDuration: 5 * time.Minute,
		mailService:   mailService,
	}
	go s.cleanupCache()
	return s
}

func (s *BudgetService) cleanupCache() {
	ticker := time.NewTicker(s.cacheDuration)
	for range ticker.C {
		s.cacheMutex.Lock()
		now := time.Now()
		for key, data := range s.usageCache {
			if now.Sub(data.cachedAt) > s.cacheDuration {
				delete(s.usageCache, key)
			}
		}
		s.cacheMutex.Unlock()
	}
}

// GetMonthlySpending calculates total spending for an app since its budget start date or the current month.
func (s *BudgetService) GetMonthlySpending(appID uint, start, end time.Time) (float64, error) {
	var app models.App
	if err := s.db.First(&app, appID).Error; err != nil {
		return 0, err
	}

	// Use passed-in start date, as it should already account for budget start date

	key := usageKey{
		entityID:   appID,
		entityType: "App",
		startDate:  start,
	}

	s.cacheMutex.RLock()
	if data, exists := s.usageCache[key]; exists {
		if time.Since(data.cachedAt) < s.cacheDuration {
			s.cacheMutex.RUnlock()
			return data.spent, nil
		}
	}
	s.cacheMutex.RUnlock()

	var totalSpent float64
	err := s.db.Model(&models.LLMChatRecord{}).
		Where("app_id = ? AND time_stamp >= ? AND time_stamp <= ?", appID, start, end).
		Select("COALESCE(SUM(cost), 0)").
		Scan(&totalSpent).Error

	if err != nil {
		return 0, err
	}

	s.cacheMutex.Lock()
	s.usageCache[key] = usageData{
		spent:    totalSpent,
		cachedAt: time.Now(),
	}
	s.cacheMutex.Unlock()

	return totalSpent, nil
}

// GetLLMMonthlySpending calculates total spending for a given LLM since its budget start date or the current month.
func (s *BudgetService) GetLLMMonthlySpending(llmID uint, start, end time.Time) (float64, error) {
	var llm models.LLM
	if err := s.db.First(&llm, llmID).Error; err != nil {
		return 0, err
	}

	// Use passed-in start date, as it should already account for budget start date

	key := usageKey{
		entityID:   llmID,
		entityType: "LLM",
		startDate:  start,
	}

	s.cacheMutex.RLock()
	if data, exists := s.usageCache[key]; exists {
		if time.Since(data.cachedAt) < s.cacheDuration {
			s.cacheMutex.RUnlock()
			return data.spent, nil
		}
	}
	s.cacheMutex.RUnlock()

	var totalSpent float64
	err := s.db.Model(&models.LLMChatRecord{}).
		Where("llm_id = ? AND time_stamp >= ? AND time_stamp <= ?", llmID, start, end).
		Select("COALESCE(SUM(cost), 0)").
		Scan(&totalSpent).Error

	if err != nil {
		return 0, err
	}

	s.cacheMutex.Lock()
	s.usageCache[key] = usageData{
		spent:    totalSpent,
		cachedAt: time.Now(),
	}
	s.cacheMutex.Unlock()

	return totalSpent, nil
}

// CheckBudget verifies if a request would exceed either App or LLM budget.
// Returns app usage percentage, llm usage percentage, and error if budget exceeded
func (s *BudgetService) CheckBudget(app *models.App, llm *models.LLM) (float64, float64, error) {
	now := time.Now()
	end := now
	var appUsage, llmUsage float64

	// Check app budget if set
	if app.MonthlyBudget != nil && *app.MonthlyBudget > 0 {
		var start time.Time
		if app.BudgetStartDate != nil {
			start = *app.BudgetStartDate
		} else {
			start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		}

		spent, err := s.GetMonthlySpending(app.ID, start, end)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to check app budget: %v", err)
		}
		appUsage = (spent / *app.MonthlyBudget) * 100

		// Send notifications at thresholds
		if appUsage >= 80 {
			budget := *app.MonthlyBudget
			usage := &models.BudgetUsage{
				EntityID:        app.ID,
				EntityType:      "App",
				Name:            app.Name,
				Usage:           appUsage,
				Budget:          &budget,
				Spent:           spent,
				BudgetStartDate: &start,
			}
			if err := s.NotifyBudgetUsage(usage, 80); err != nil {
				// Log error but continue processing
				fmt.Printf("Failed to send app budget notification: %v\n", err)
			}

			// Send 100% notification if we've crossed that threshold too
			if appUsage >= 100 {
				if err := s.NotifyBudgetUsage(usage, 100); err != nil {
					fmt.Printf("Failed to send app budget notification: %v\n", err)
				}
			}
		}

		if spent >= *app.MonthlyBudget {
			return appUsage, llmUsage, fmt.Errorf("app monthly budget exceeded: spent %.2f of %.2f", spent, *app.MonthlyBudget)
		}
	}

	// Check LLM budget if set
	if llm.MonthlyBudget != nil && *llm.MonthlyBudget > 0 {
		var start time.Time
		if llm.BudgetStartDate != nil {
			start = *llm.BudgetStartDate
		} else {
			start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		}

		spent, err := s.GetLLMMonthlySpending(llm.ID, start, end)
		if err != nil {
			return appUsage, 0, fmt.Errorf("failed to check LLM budget: %v", err)
		}
		llmUsage = (spent / *llm.MonthlyBudget) * 100

		// Send notifications at thresholds
		if llmUsage >= 80 {
			budget := *llm.MonthlyBudget
			usage := &models.BudgetUsage{
				EntityID:        llm.ID,
				EntityType:      "LLM",
				Name:            llm.Name,
				Usage:           llmUsage,
				Budget:          &budget,
				Spent:           spent,
				BudgetStartDate: &start,
			}
			if err := s.NotifyBudgetUsage(usage, 80); err != nil {
				// Log error but continue processing
				fmt.Printf("Failed to send llm budget notification: %v\n", err)
			}

			// Send 100% notification if we've crossed that threshold too
			if llmUsage >= 100 {
				if err := s.NotifyBudgetUsage(usage, 100); err != nil {
					fmt.Printf("Failed to send llm budget notification: %v\n", err)
				}
			}
		}

		if spent >= *llm.MonthlyBudget {
			return appUsage, llmUsage, fmt.Errorf("LLM monthly budget exceeded: spent %.2f of %.2f", spent, *llm.MonthlyBudget)
		}
	}

	return appUsage, llmUsage, nil
}

// ClearCache clears the spending cache, forcing next queries to hit the database
func (s *BudgetService) ClearCache() {
	s.cacheMutex.Lock()
	s.usageCache = make(map[usageKey]usageData)
	s.cacheMutex.Unlock()
}

// NotifyBudgetUsage sends notifications when budget thresholds are reached
func (s *BudgetService) NotifyBudgetUsage(usage *models.BudgetUsage, threshold int) error {
	if usage.EntityType == "App" {
		return s.sendAppBudgetNotification(usage.EntityID, usage.Spent, *usage.Budget, threshold)
	}
	return s.sendLLMBudgetNotification(usage.EntityID, usage.Spent, *usage.Budget, threshold)
}

func (s *BudgetService) sendAppBudgetNotification(appID uint, spent, budget float64, threshold int) error {
	var app models.App
	if err := s.db.First(&app, appID).Error; err != nil {
		return err
	}

	var owner models.User
	if err := s.db.First(&owner, app.UserID).Error; err != nil {
		return err
	}

	var admins []models.User
	if err := s.db.Where("is_admin = ?", true).Find(&admins).Error; err != nil {
		return err
	}

	// Get currency from app's first LLM
	var firstLLM models.LLM
	var currency string
	if err := s.db.Model(&app).Association("LLMs").Find(&firstLLM); err == nil && firstLLM.DefaultModel != "" {
		var modelPrice models.ModelPrice
		if err := modelPrice.GetByModelNameAndVendor(s.db, firstLLM.DefaultModel, string(firstLLM.Vendor)); err == nil {
			currency = modelPrice.Currency
		}
	}

	tmpl, err := template.ParseFiles("./templates/budget_alert.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse template: %v", err)
	}

	// Notify owner
	ownerData := map[string]interface{}{
		"IsLLM":        false,
		"IsAdmin":      false,
		"Name":         app.Name,
		"CurrentUsage": spent,
		"Budget":       budget,
		"Currency":     currency,
		"Threshold":    threshold,
	}

	var ownerBuf bytes.Buffer
	if err := tmpl.Execute(&ownerBuf, ownerData); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}

	if err := s.mailService.SendEmail(
		owner.Email,
		fmt.Sprintf("Budget Alert: App %s at %d%% Usage", app.Name, threshold),
		ownerBuf.String(),
	); err != nil {
		return err
	}

	// Notify admins
	adminData := map[string]interface{}{
		"IsLLM":        false,
		"IsAdmin":      true,
		"Name":         app.Name,
		"OwnerEmail":   owner.Email,
		"CurrentUsage": spent,
		"Budget":       budget,
		"Currency":     currency,
		"Threshold":    threshold,
	}

	var adminBuf bytes.Buffer
	if err := tmpl.Execute(&adminBuf, adminData); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}

	for _, admin := range admins {
		if err := s.mailService.SendEmail(
			admin.Email,
			fmt.Sprintf("Budget Alert: App %s at %d%% Usage", app.Name, threshold),
			adminBuf.String(),
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *BudgetService) sendLLMBudgetNotification(llmID uint, spent, budget float64, threshold int) error {
	var llm models.LLM
	if err := s.db.First(&llm, llmID).Error; err != nil {
		return err
	}

	var admins []models.User
	if err := s.db.Where("is_admin = ?", true).Find(&admins).Error; err != nil {
		return err
	}

	var currency string
	if llm.DefaultModel != "" {
		var modelPrice models.ModelPrice
		if err := modelPrice.GetByModelNameAndVendor(s.db, llm.DefaultModel, string(llm.Vendor)); err == nil {
			currency = modelPrice.Currency
		}
	}

	tmpl, err := template.ParseFiles("./templates/budget_alert.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse template: %v", err)
	}

	data := map[string]interface{}{
		"IsLLM":        true,
		"Name":         llm.Name,
		"CurrentUsage": spent,
		"Budget":       budget,
		"Currency":     currency,
		"Threshold":    threshold,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}

	// Notify admins only
	for _, admin := range admins {
		if err := s.mailService.SendEmail(
			admin.Email,
			fmt.Sprintf("Budget Alert: LLM %s at %d%% Usage", llm.Name, threshold),
			buf.String(),
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *BudgetService) GetBudgetUsage() ([]models.BudgetUsage, error) {
	var usages []models.BudgetUsage
	now := time.Now()
	defaultStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Get all Apps
	var apps []models.App
	if err := s.db.Find(&apps).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch apps: %v", err)
	}
	for _, app := range apps {
		// Use budget start date if set, otherwise use start of current month
		start := defaultStart
		if app.BudgetStartDate != nil {
			start = *app.BudgetStartDate
		}

		spent, err := s.GetMonthlySpending(app.ID, start, now)
		if err != nil {
			continue
		}
		usage := models.BudgetUsage{
			EntityID:        app.ID,
			Name:            app.Name,
			EntityType:      "App",
			Budget:          app.MonthlyBudget,
			Spent:           spent,
			BudgetStartDate: app.BudgetStartDate,
		}
		if app.MonthlyBudget != nil && *app.MonthlyBudget > 0 {
			usage.Usage = (spent / *app.MonthlyBudget) * 100
		}
		usages = append(usages, usage)
	}

	// Get all LLMs
	var llms []models.LLM
	if err := s.db.Find(&llms).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch LLMs: %v", err)
	}
	for _, llm := range llms {
		// Use budget start date if set, otherwise use start of current month
		start := defaultStart
		if llm.BudgetStartDate != nil {
			start = *llm.BudgetStartDate
		}

		spent, err := s.GetLLMMonthlySpending(llm.ID, start, now)
		if err != nil {
			continue
		}
		usage := models.BudgetUsage{
			EntityID:        llm.ID,
			Name:            llm.Name,
			EntityType:      "LLM",
			Budget:          llm.MonthlyBudget,
			Spent:           spent,
			BudgetStartDate: llm.BudgetStartDate,
		}
		if llm.MonthlyBudget != nil && *llm.MonthlyBudget > 0 {
			usage.Usage = (spent / *llm.MonthlyBudget) * 100
		}
		usages = append(usages, usage)
	}

	return usages, nil
}
