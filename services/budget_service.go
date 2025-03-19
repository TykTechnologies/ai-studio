package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
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
	db              *gorm.DB
	usageCache      map[usageKey]usageData
	cacheMutex      sync.RWMutex
	cacheDuration   time.Duration
	notificationSvc *NotificationService
	templatePath    string
}

// calculateBudgetPeriodStart determines the start of the current budget period
// based on a reference budget start date. It uses the day of the month from
// the reference date to calculate the current period's start.
func (s *BudgetService) calculateBudgetPeriodStart(referenceDate *time.Time, now time.Time) time.Time {
	if referenceDate == nil {
		// If no reference date, use 1st of current month
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}

	budgetDay := referenceDate.Day()
	currentYear := now.Year()
	currentMonth := now.Month()

	// If we haven't reached the budget day in current month,
	// the period started on the budget day of previous month
	if now.Day() < budgetDay {
		// Go back one month
		if currentMonth == time.January {
			currentMonth = time.December
			currentYear--
		} else {
			currentMonth--
		}
	}

	result := time.Date(currentYear, currentMonth, budgetDay, 0, 0, 0, 0, now.Location())
	log.Printf("Calculated budget period start: %v for reference %v, now %v", result, referenceDate, now)
	return result
}

// NewBudgetService returns our unified budget service
func NewBudgetService(db *gorm.DB, notificationSvc *NotificationService) *BudgetService {
	// Get the absolute path to the template
	wd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("failed to get working directory: %v", err))
	}

	// Walk up the directory tree until we find the templates directory
	templatePath := "templates/budget_alert.tmpl"
	for {
		if _, err := os.Stat(filepath.Join(wd, templatePath)); err == nil {
			templatePath = filepath.Join(wd, templatePath)
			break
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			// We've reached the root directory
			panic("could not find templates directory")
		}
		wd = parent
	}

	s := &BudgetService{
		db:              db,
		usageCache:      make(map[usageKey]usageData),
		cacheMutex:      sync.RWMutex{},
		cacheDuration:   5 * time.Minute,
		notificationSvc: notificationSvc,
		templatePath:    templatePath,
	}
	go s.cleanupCache()
	return s
}

func (s *BudgetService) cleanupCache() {
	ticker := time.NewTicker(1 * time.Minute)  // Run cleanup every minute instead of every millisecond
	defer ticker.Stop()  // Ensure the ticker is cleaned up if the function exits
	
	for range ticker.C {
		s.cacheMutex.Lock()
		now := time.Now()
		for key, data := range s.usageCache {
			if now.Sub(data.cachedAt) > s.cacheDuration {
				delete(s.usageCache, key)
				log.Printf("Cache entry expired for %s ID %d", key.entityType, key.entityID)
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
			log.Printf("App %d spending from cache: %.2f (cached %v ago)", appID, data.spent, time.Since(data.cachedAt))
			return data.spent, nil
		}
		log.Printf("App %d cache expired (cached %v ago)", appID, time.Since(data.cachedAt))
	}
	s.cacheMutex.RUnlock()
	log.Printf("App %d spending cache miss, querying database", appID)

	// Adjust end time to include full day
	end = time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, end.Location())

	var totalSpent float64
	var recordCount int64

	// First get count of records for debugging
	countQuery := s.db.Model(&models.LLMChatRecord{}).
		Where("app_id = ? AND time_stamp >= ? AND time_stamp <= ?", appID, start, end)
	if err := countQuery.Count(&recordCount).Error; err != nil {
		log.Printf("Error counting records for app %d: %v", appID, err)
	}

	// Get raw sum before division
	var rawSum float64
	rawQuery := s.db.Model(&models.LLMChatRecord{}).
		Where("app_id = ? AND time_stamp >= ? AND time_stamp <= ?", appID, start, end).
		Select("COALESCE(CAST(SUM(cost) AS DECIMAL(20,4)), 0)")
	if err := rawQuery.Scan(&rawSum).Error; err != nil {
		log.Printf("Error getting raw sum for app %d: %v", appID, err)
	}

	// Get raw sum and perform division in application layer
	query := s.db.Model(&models.LLMChatRecord{}).
		Where("app_id = ? AND time_stamp >= ? AND time_stamp <= ?", appID, start, end).
		Select("COALESCE(SUM(cost), 0)")

	// Enhanced logging for query debugging
	sql := query.Statement.SQL.String()
	vars := query.Statement.Vars
	log.Printf("App %d spending calculation:", appID)
	log.Printf("- Records found: %d", recordCount)
	log.Printf("- Raw sum before division: %.4f", rawSum)
	log.Printf("- Query: %s with values: %v", sql, vars)

	var rawTotal float64
	err := query.Scan(&rawTotal).Error
	if err != nil {
		log.Printf("Error calculating app %d spending: %v", appID, err)
		return 0, err
	}

	totalSpent = rawTotal / 10000.0

	if err != nil {
		log.Printf("Error calculating app %d spending: %v", appID, err)
		return 0, err
	}
	log.Printf("App %d spending calculated: %.2f (start: %v, end: %v)", appID, totalSpent, start, end)

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
			log.Printf("LLM %d spending from cache: %.2f (cached %v ago)", llmID, data.spent, time.Since(data.cachedAt))
			return data.spent, nil
		}
		log.Printf("LLM %d cache expired (cached %v ago)", llmID, time.Since(data.cachedAt))
	}
	s.cacheMutex.RUnlock()
	log.Printf("LLM %d spending cache miss, querying database", llmID)

	// Adjust end time to include full day
	end = time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, end.Location())

	var totalSpent float64
	var recordCount int64

	// First get count of records for debugging
	countQuery := s.db.Model(&models.LLMChatRecord{}).
		Where("llm_id = ? AND time_stamp >= ? AND time_stamp <= ?", llmID, start, end)
	if err := countQuery.Count(&recordCount).Error; err != nil {
		log.Printf("Error counting records for LLM %d: %v", llmID, err)
	}

	// Get raw sum before division
	var rawSum float64
	rawQuery := s.db.Model(&models.LLMChatRecord{}).
		Where("llm_id = ? AND time_stamp >= ? AND time_stamp <= ?", llmID, start, end).
		Select("COALESCE(CAST(SUM(cost) AS DECIMAL(20,4)), 0)")
	if err := rawQuery.Scan(&rawSum).Error; err != nil {
		log.Printf("Error getting raw sum for LLM %d: %v", llmID, err)
	}

	// Get raw sum and perform division in application layer
	query := s.db.Model(&models.LLMChatRecord{}).
		Where("llm_id = ? AND time_stamp >= ? AND time_stamp <= ?", llmID, start, end).
		Select("COALESCE(SUM(cost), 0)")

	// Enhanced logging for query debugging
	sql := query.Statement.SQL.String()
	vars := query.Statement.Vars
	log.Printf("LLM %d spending calculation:", llmID)
	log.Printf("- Records found: %d", recordCount)
	log.Printf("- Raw sum before division: %.4f", rawSum)
	log.Printf("- Query: %s with values: %v", sql, vars)

	var rawTotal float64
	err := query.Scan(&rawTotal).Error
	if err != nil {
		log.Printf("Error calculating LLM %d spending: %v", llmID, err)
		return 0, err
	}

	totalSpent = rawTotal / 10000.0

	if err != nil {
		log.Printf("Error calculating LLM %d spending: %v", llmID, err)
		return 0, err
	}
	log.Printf("LLM %d spending calculated: %.2f (start: %v, end: %v)", llmID, totalSpent, start, end)

	s.cacheMutex.Lock()
	s.usageCache[key] = usageData{
		spent:    totalSpent,
		cachedAt: time.Now(),
	}
	s.cacheMutex.Unlock()

	return totalSpent, nil
}

// CheckBudget verifies if a request would exceed either App or LLM budget by checking for 100% threshold notifications.
// Returns app usage percentage, llm usage percentage, and error if budget exceeded
func (s *BudgetService) CheckBudget(app *models.App, llm *models.LLM) (float64, float64, error) {
	now := time.Now()
	var appUsage, llmUsage float64

	// Quick check for app budget
	if app.MonthlyBudget != nil && *app.MonthlyBudget > 0 {
		start := s.calculateBudgetPeriodStart(app.BudgetStartDate, now)
		monthOffset := int(start.Sub(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24 / 30)
		// Check for 100% threshold notification
		baseNotificationID := fmt.Sprintf("budget_app_%d_%d_%d_%d",
			app.ID,
			monthOffset,
			int(*app.MonthlyBudget),
			100, // 100% threshold
		)

		log.Printf("Checking app notification: %s", baseNotificationID)

		// Check for either owner or admin notifications
		var notification models.Notification
		err := s.db.Where("(notification_id = ? OR notification_id LIKE ?) AND sent_at >= ?",
			fmt.Sprintf("%s_owner", baseNotificationID),
			fmt.Sprintf("%s_admin_%%", baseNotificationID),
			start).First(&notification).Error
		if err == nil {
			// Found 100% threshold notification for this period
			return 100, llmUsage, fmt.Errorf("app monthly budget exceeded")
		}
	}

	// Quick check for LLM budget
	if llm.MonthlyBudget != nil && *llm.MonthlyBudget > 0 {
		start := s.calculateBudgetPeriodStart(llm.BudgetStartDate, now)
		monthOffset := int(start.Sub(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24 / 30)
		// Check for 100% threshold notification
		baseNotificationID := fmt.Sprintf("budget_llm_%d_%d_%d_%d",
			llm.ID,
			monthOffset,
			int(*llm.MonthlyBudget),
			100, // 100% threshold
		)

		log.Printf("Checking LLM notification: %s", baseNotificationID)

		// Check for admin notifications
		var notification models.Notification
		err := s.db.Where("notification_id LIKE ? AND sent_at >= ?",
			fmt.Sprintf("%s_admin_%%", baseNotificationID),
			start).First(&notification).Error
		if err == nil {
			// Found 100% threshold notification for this period
			return appUsage, 100, fmt.Errorf("LLM monthly budget exceeded")
		} else if err != gorm.ErrRecordNotFound {
			// Log unexpected errors but don't block the request
			log.Printf("Error checking for budget notifications: %v", err)
		}
	}

	return appUsage, llmUsage, nil
}

// AnalyzeBudgetUsage analyzes current budget usage and sends notifications if thresholds are reached
func (s *BudgetService) AnalyzeBudgetUsage(app *models.App, llm *models.LLM) {
	now := time.Now()
	end := now

	// Check app budget
	if app.MonthlyBudget != nil && *app.MonthlyBudget > 0 {
		start := s.calculateBudgetPeriodStart(app.BudgetStartDate, now)
		spent, err := s.GetMonthlySpending(app.ID, start, end)
		if err == nil {
			appUsage := (spent / *app.MonthlyBudget) * 100
			log.Printf("App %d usage calculated: %.2f%% (spent: %.2f, budget: %.2f)", 
				app.ID, appUsage, spent, *app.MonthlyBudget)
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

			// Check for existing notifications in this period
			monthOffset := int(start.Sub(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24 / 30)
			baseNotificationID80 := fmt.Sprintf("budget_app_%d_%d_%d_%d",
				app.ID,
				monthOffset,
				int(*app.MonthlyBudget),
				80,
			)
			baseNotificationID100 := fmt.Sprintf("budget_app_%d_%d_%d_%d",
				app.ID,
				monthOffset,
				int(*app.MonthlyBudget),
				100,
			)

			var existing80, existing100 models.Notification
			err80 := s.db.Where("(notification_id = ? OR notification_id LIKE ?) AND sent_at >= ?",
				fmt.Sprintf("%s_owner", baseNotificationID80),
				fmt.Sprintf("%s_admin_%%", baseNotificationID80),
				start).First(&existing80).Error
			err100 := s.db.Where("(notification_id = ? OR notification_id LIKE ?) AND sent_at >= ?",
				fmt.Sprintf("%s_owner", baseNotificationID100),
				fmt.Sprintf("%s_admin_%%", baseNotificationID100),
				start).First(&existing100).Error

			// Simplified threshold checks for immediate notification
			if appUsage >= 80 && err80 != nil {
				if err := s.NotifyBudgetUsage(usage, 80); err != nil {
					log.Printf("Failed to send app budget notification (80%%): %v", err)
				}
			}
			if appUsage >= 100 && err100 != nil {
				if err := s.NotifyBudgetUsage(usage, 100); err != nil {
					log.Printf("Failed to send app budget notification (100%%): %v", err)
				}
			}
		}
	}

	// Check LLM budget
	if llm.MonthlyBudget != nil && *llm.MonthlyBudget > 0 {
		start := s.calculateBudgetPeriodStart(llm.BudgetStartDate, now)
		spent, err := s.GetLLMMonthlySpending(llm.ID, start, end)
		if err == nil {
			llmUsage := (spent / *llm.MonthlyBudget) * 100
			log.Printf("LLM %d usage calculated: %.2f%% (spent: %.2f, budget: %.2f)", 
				llm.ID, llmUsage, spent, *llm.MonthlyBudget)
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

			// Check for existing notifications in this period
			monthOffset := int(start.Sub(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24 / 30)
			baseNotificationID80 := fmt.Sprintf("budget_llm_%d_%d_%d_%d",
				llm.ID,
				monthOffset,
				int(*llm.MonthlyBudget),
				80,
			)
			baseNotificationID100 := fmt.Sprintf("budget_llm_%d_%d_%d_%d",
				llm.ID,
				monthOffset,
				int(*llm.MonthlyBudget),
				100,
			)

			var existing80, existing100 models.Notification
			err80 := s.db.Where("notification_id LIKE ? AND sent_at >= ?",
				fmt.Sprintf("%s_admin_%%", baseNotificationID80),
				start).First(&existing80).Error
			err100 := s.db.Where("notification_id LIKE ? AND sent_at >= ?",
				fmt.Sprintf("%s_admin_%%", baseNotificationID100),
				start).First(&existing100).Error

			// Simplified threshold checks for immediate notification
			if llmUsage >= 80 && err80 != nil {
				if err := s.NotifyBudgetUsage(usage, 80); err != nil {
					log.Printf("Failed to send LLM budget notification (80%%): %v", err)
				}
			}
			if llmUsage >= 100 && err100 != nil {
				if err := s.NotifyBudgetUsage(usage, 100); err != nil {
					log.Printf("Failed to send LLM budget notification (100%%): %v", err)
				}
			}
		}
	}
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
		fmt.Printf("Failed to find app: %v\n", err)
		return err
	}

	var owner models.User
	if err := s.db.First(&owner, app.UserID).Error; err != nil {
		fmt.Printf("Failed to find owner: %v\n", err)
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

	// Get start time for notification ID
	now := time.Now()
	start := s.calculateBudgetPeriodStart(app.BudgetStartDate, now)

	// Create base notification ID (used for budget check)
	monthOffset := int(start.Sub(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24 / 30)
	baseNotificationID := fmt.Sprintf("budget_app_%d_%d_%d_%d",
		app.ID,
		monthOffset,
		int(budget),
		threshold,
	)

	// Prepare data for notifications
	data := map[string]interface{}{
		"IsLLM":        false,
		"IsAdmin":      false,
		"Name":         app.Name,
		"CurrentUsage": spent,
		"Budget":       budget,
		"Currency":     currency,
		"Threshold":    threshold,
	}

	// Send notification to both owner and admins
	data["IsAdmin"] = true
	data["OwnerEmail"] = owner.Email

	title := fmt.Sprintf("Budget Alert: App %s at %d%% Usage", app.Name, threshold)
	if err := s.notificationSvc.Notify(baseNotificationID, title, "budget_alert.tmpl", data, owner.ID|models.NotifyAdmins); err != nil {
		return fmt.Errorf("failed to send notifications: %v", err)
	}

	return nil
}

func (s *BudgetService) sendLLMBudgetNotification(llmID uint, spent, budget float64, threshold int) error {
	var llm models.LLM
	if err := s.db.First(&llm, llmID).Error; err != nil {
		return err
	}

	// Get start time for notification ID
	now := time.Now()
	start := s.calculateBudgetPeriodStart(llm.BudgetStartDate, now)

	// Create base notification ID (used for budget check)
	monthOffset := int(start.Sub(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24 / 30)
	baseNotificationID := fmt.Sprintf("budget_llm_%d_%d_%d_%d",
		llm.ID,
		monthOffset,
		int(budget),
		threshold,
	)

	var currency string
	if llm.DefaultModel != "" {
		var modelPrice models.ModelPrice
		if err := modelPrice.GetByModelNameAndVendor(s.db, llm.DefaultModel, string(llm.Vendor)); err == nil {
			currency = modelPrice.Currency
		}
	}

	data := map[string]interface{}{
		"IsLLM":        true,
		"Name":         llm.Name,
		"CurrentUsage": spent,
		"Budget":       budget,
		"Currency":     currency,
		"Threshold":    threshold,
	}

	// Send to admins
	title := fmt.Sprintf("Budget Alert: LLM %s at %d%% Usage", llm.Name, threshold)
	if err := s.notificationSvc.Notify(baseNotificationID, title, "budget_alert.tmpl", data, models.NotifyAdmins); err != nil {
		return fmt.Errorf("failed to send admin notification: %v", err)
	}

	return nil
}

func (s *BudgetService) GetBudgetUsage() ([]models.BudgetUsage, error) {
	var usages []models.BudgetUsage
	now := time.Now()

	// Get all Apps
	var apps []models.App
	if err := s.db.Find(&apps).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch apps: %v", err)
	}
	for _, app := range apps {
		start := s.calculateBudgetPeriodStart(app.BudgetStartDate, now)
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
		start := s.calculateBudgetPeriodStart(llm.BudgetStartDate, now)
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
