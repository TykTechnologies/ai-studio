package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

// atomicUsageCache is a thread-safe cache entry
type atomicUsageCache struct {
	spent    uint64 // stored as integer (float * 10000)
	cachedAt int64  // unix timestamp in nanoseconds
}

type BudgetService struct {
	db              *gorm.DB
	usageCache      map[usageKey]usageData         // legacy cache
	atomicCache     map[usageKey]*atomicUsageCache // optimistic cache
	cacheMutex      sync.RWMutex
	updateMutex     sync.Mutex // separate mutex for background updates
	cacheDuration   time.Duration
	notificationSvc *NotificationService
	templatePath    string
	updateQueue     chan cacheUpdateRequest
	queueRunning    int32 // atomic flag to track if update worker is running
}

// cacheUpdateRequest represents a request to update the cache
type cacheUpdateRequest struct {
	key usageKey
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
		atomicCache:     make(map[usageKey]*atomicUsageCache),
		cacheMutex:      sync.RWMutex{},
		updateMutex:     sync.Mutex{},
		cacheDuration:   5 * time.Minute,
		notificationSvc: notificationSvc,
		templatePath:    templatePath,
		updateQueue:     make(chan cacheUpdateRequest, 100), // Buffer size of 100
		queueRunning:    0,
	}
	go s.cleanupCache()
	return s
}

// startUpdateWorker ensures the update worker is running
func (s *BudgetService) startUpdateWorker() {
	// Only start if not already running
	if atomic.CompareAndSwapInt32(&s.queueRunning, 0, 1) {
		go s.processUpdateQueue()
	}
}

// processUpdateQueue processes cache update requests in the background
func (s *BudgetService) processUpdateQueue() {
	defer atomic.StoreInt32(&s.queueRunning, 0)

	// Use a map to track unique keys to update
	pendingUpdates := make(map[usageKey]struct{})

	for {
		select {
		case req, ok := <-s.updateQueue:
			if !ok {
				// Channel closed
				return
			}

			// Just mark this key as needing an update
			pendingUpdates[req.key] = struct{}{}

		case <-time.After(100 * time.Millisecond):
			// Process accumulated requests after a short delay
			if len(pendingUpdates) > 0 {
				s.processPendingUpdates(pendingUpdates)
				pendingUpdates = make(map[usageKey]struct{}) // Clear the map
			}

			// If no more requests for a while, exit
			if len(s.updateQueue) == 0 {
				select {
				case req, ok := <-s.updateQueue:
					if ok {
						// Got one more request, process it in the next iteration
						pendingUpdates[req.key] = struct{}{}
						continue
					}
				case <-time.After(5 * time.Second):
					// No requests for 5 seconds, exit the goroutine
					return
				}
			}
		}
	}
}

// processPendingUpdates processes a batch of update requests
func (s *BudgetService) processPendingUpdates(updates map[usageKey]struct{}) {
	now := time.Now()

	for key := range updates {
		var spent float64
		var err error

		// Process based on entity type
		if key.entityType == "App" {
			spent, err = s.fetchAppSpending(key.entityID, key.startDate, now)
		} else if key.entityType == "LLM" {
			spent, err = s.fetchLLMSpending(key.entityID, key.startDate, now)
		}

		if err != nil {
			log.Printf("Error updating cache for %s %d: %v", key.entityType, key.entityID, err)
			continue
		}

		// Update both caches
		s.updateCaches(key, spent, now)

		// Always trigger budget analysis for consistency
		if key.entityType == "App" {
			var app models.App
			if err := s.db.First(&app, key.entityID).Error; err == nil {
				go s.AnalyzeBudgetUsage(&app, nil)
			}
		} else if key.entityType == "LLM" {
			var llm models.LLM
			if err := s.db.First(&llm, key.entityID).Error; err == nil {
				go s.AnalyzeBudgetUsage(nil, &llm)
			}
		}
	}
}

// updateCaches updates both the legacy and atomic caches
func (s *BudgetService) updateCaches(key usageKey, spent float64, now time.Time) {
	// Update legacy cache
	s.cacheMutex.Lock()
	s.usageCache[key] = usageData{
		spent:    spent,
		cachedAt: now,
	}
	s.cacheMutex.Unlock()

	// Update atomic cache (create if doesn't exist)
	s.cacheMutex.RLock()
	cache, exists := s.atomicCache[key]
	s.cacheMutex.RUnlock()

	if !exists {
		s.cacheMutex.Lock()
		cache, exists = s.atomicCache[key]
		if !exists {
			cache = &atomicUsageCache{}
			s.atomicCache[key] = cache
		}
		s.cacheMutex.Unlock()
	}

	// Update atomic values
	atomic.StoreUint64(&cache.spent, uint64(spent*10000))
	atomic.StoreInt64(&cache.cachedAt, now.UnixNano())
}

// fetchAppSpending gets the spending for an app directly from the database
func (s *BudgetService) fetchAppSpending(appID uint, start, end time.Time) (float64, error) {
	// Adjust end time to include full day
	end = time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, end.Location())

	var rawTotal float64
	query := s.db.Model(&models.LLMChatRecord{}).
		Where("app_id = ? AND time_stamp >= ? AND time_stamp <= ?", appID, start, end).
		Select("COALESCE(SUM(cost), 0)")

	err := query.Scan(&rawTotal).Error
	if err != nil {
		return 0, err
	}

	return rawTotal / 10000.0, nil
}

// fetchLLMSpending gets the spending for an LLM directly from the database
func (s *BudgetService) fetchLLMSpending(llmID uint, start, end time.Time) (float64, error) {
	// Adjust end time to include full day
	end = time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, end.Location())

	var rawTotal float64
	query := s.db.Model(&models.LLMChatRecord{}).
		Where("llm_id = ? AND time_stamp >= ? AND time_stamp <= ?", llmID, start, end).
		Select("COALESCE(SUM(cost), 0)")

	err := query.Scan(&rawTotal).Error
	if err != nil {
		return 0, err
	}

	return rawTotal / 10000.0, nil
}

func (s *BudgetService) cleanupCache() {
	ticker := time.NewTicker(5 * time.Minute) // Less frequent cleanup (every 5 minutes)
	defer ticker.Stop()                       // Ensure the ticker is cleaned up if the function exits

	for range ticker.C {
		// Use a separate goroutine to avoid blocking the ticker
		go func() {
			// Create a list of keys to delete
			var keysToDelete []usageKey

			// First identify expired keys with a read lock
			s.cacheMutex.RLock()
			now := time.Now()
			for key, data := range s.usageCache {
				if now.Sub(data.cachedAt) > s.cacheDuration {
					keysToDelete = append(keysToDelete, key)
				}
			}
			s.cacheMutex.RUnlock()

			// Only lock for the actual deletion if we have keys to delete
			if len(keysToDelete) > 0 {
				s.cacheMutex.Lock()
				for _, key := range keysToDelete {
					// Double-check the key is still expired (it might have been updated)
					if data, exists := s.usageCache[key]; exists && now.Sub(data.cachedAt) > s.cacheDuration {
						delete(s.usageCache, key)
					}
				}
				s.cacheMutex.Unlock()
				log.Printf("Cleaned up %d expired cache entries", len(keysToDelete))
			}
		}()
	}
}

// GetMonthlySpending calculates total spending for an app since its budget start date or the current month.
// Now uses the optimistic cache approach
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

	// Try atomic cache first (no locking)
	spent, cacheHit := s.getFromAtomicCache(key)
	if cacheHit {
		log.Printf("App %d spending from atomic cache: %.2f", appID, spent)
		return spent, nil
	}

	// Fall back to legacy cache with read lock
	s.cacheMutex.RLock()
	data, exists := s.usageCache[key]
	if exists && time.Since(data.cachedAt) < s.cacheDuration {
		spent = data.spent
		s.cacheMutex.RUnlock()
		log.Printf("App %d spending from legacy cache: %.2f", appID, spent)
		return spent, nil
	}
	s.cacheMutex.RUnlock()

	log.Printf("App %d spending cache miss, querying database", appID)

	// Queue an update for the future and fetch immediately
	spent, err := s.fetchAppSpending(appID, start, end)
	if err != nil {
		return 0, err
	}

	// Update both caches
	s.updateCaches(key, spent, time.Now())

	return spent, nil
}

// GetLLMMonthlySpending calculates total spending for a given LLM since its budget start date or the current month.
// Now uses the optimistic cache approach
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

	// Try atomic cache first (no locking)
	spent, cacheHit := s.getFromAtomicCache(key)
	if cacheHit {
		log.Printf("LLM %d spending from atomic cache: %.2f", llmID, spent)
		return spent, nil
	}

	// Fall back to legacy cache with read lock
	s.cacheMutex.RLock()
	data, exists := s.usageCache[key]
	if exists && time.Since(data.cachedAt) < s.cacheDuration {
		spent = data.spent
		s.cacheMutex.RUnlock()
		log.Printf("LLM %d spending from legacy cache: %.2f", llmID, spent)
		return spent, nil
	}
	s.cacheMutex.RUnlock()

	log.Printf("LLM %d spending cache miss, querying database", llmID)

	// Queue an update for the future and fetch immediately
	spent, err := s.fetchLLMSpending(llmID, start, end)
	if err != nil {
		return 0, err
	}

	// Update both caches
	s.updateCaches(key, spent, time.Now())

	return spent, nil
}

// CheckBudget verifies if a request would exceed either App or LLM budget by first checking the cache,
// then falling back to checking for 100% threshold notifications.
// Returns app usage percentage, llm usage percentage, and error if budget exceeded
func (s *BudgetService) CheckBudget(app *models.App, llm *models.LLM) (float64, float64, error) {
	now := time.Now()
	var appUsage, llmUsage float64
	var appBudgetExceeded, llmBudgetExceeded bool

	// Ensure the update worker is running
	s.startUpdateWorker()

	// Check for app budget
	if app.MonthlyBudget != nil && *app.MonthlyBudget > 0 {
		start := s.calculateBudgetPeriodStart(app.BudgetStartDate, now)

		// Create the key for cache lookup
		key := usageKey{
			entityID:   app.ID,
			entityType: "App",
			startDate:  start,
		}

		// Try atomic cache first (no locking)
		appSpent, cacheHit := s.getFromAtomicCache(key)

		if !cacheHit {
			// Fall back to legacy cache with read lock
			s.cacheMutex.RLock()
			data, exists := s.usageCache[key]
			if exists && time.Since(data.cachedAt) < s.cacheDuration {
				appSpent = data.spent
				cacheHit = true
			}
			s.cacheMutex.RUnlock()
		}

		if cacheHit {
			log.Printf("App %d spending from cache: %.2f/%.2f", app.ID, appSpent, *app.MonthlyBudget)
		} else {
			// Cache miss - queue an update but use a default value for now
			// This makes the check optimistic - we'll assume budget is not exceeded
			// until we have data that proves otherwise
			s.updateQueue <- cacheUpdateRequest{key: key}

			// Check for existing notifications to see if we already know the budget is exceeded
			monthOffset := int(start.Sub(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24 / 30)
			baseNotificationID := fmt.Sprintf("budget_app_%d_%d_%d_%d",
				app.ID,
				monthOffset,
				int(*app.MonthlyBudget),
				100, // 100% threshold
			)

			// Quick check for notification existence
			var notification models.Notification
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			err := s.db.WithContext(ctx).Where("(notification_id = ? OR notification_id LIKE ?) AND sent_at >= ?",
				fmt.Sprintf("%s_owner", baseNotificationID),
				fmt.Sprintf("%s_admin_%%", baseNotificationID),
				start).First(&notification).Error

			if err == nil {
				// Notification exists, budget is exceeded
				appBudgetExceeded = true
				appUsage = 100
				log.Printf("App %d budget exceeded (notification exists)", app.ID)
			} else {
				// No notification, use a conservative estimate
				appSpent = 0
			}
		}

		// Calculate usage percentage if we have spending data
		if !appBudgetExceeded {
			appUsage = (appSpent / *app.MonthlyBudget) * 100

			// Check if budget is exceeded
			if appSpent >= *app.MonthlyBudget {
				appBudgetExceeded = true
				log.Printf("App %d budget exceeded: %.2f/%.2f", app.ID, appSpent, *app.MonthlyBudget)
			}
		}
	}

	// Check for LLM budget
	if llm.MonthlyBudget != nil && *llm.MonthlyBudget > 0 {
		start := s.calculateBudgetPeriodStart(llm.BudgetStartDate, now)

		// Create the key for cache lookup
		key := usageKey{
			entityID:   llm.ID,
			entityType: "LLM",
			startDate:  start,
		}

		// Try atomic cache first (no locking)
		llmSpent, cacheHit := s.getFromAtomicCache(key)

		if !cacheHit {
			// Fall back to legacy cache with read lock
			s.cacheMutex.RLock()
			data, exists := s.usageCache[key]
			if exists && time.Since(data.cachedAt) < s.cacheDuration {
				llmSpent = data.spent
				cacheHit = true
			}
			s.cacheMutex.RUnlock()
		}

		if cacheHit {
			log.Printf("LLM %d spending from cache: %.2f/%.2f", llm.ID, llmSpent, *llm.MonthlyBudget)
		} else {
			// Cache miss - queue an update but use a default value for now
			s.updateQueue <- cacheUpdateRequest{key: key}

			// Check for existing notifications to see if we already know the budget is exceeded
			monthOffset := int(start.Sub(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24 / 30)
			baseNotificationID := fmt.Sprintf("budget_llm_%d_%d_%d_%d",
				llm.ID,
				monthOffset,
				int(*llm.MonthlyBudget),
				100, // 100% threshold
			)

			// Quick check for notification existence
			var notification models.Notification
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			err := s.db.WithContext(ctx).Where("notification_id LIKE ? AND sent_at >= ?",
				fmt.Sprintf("%s_admin_%%", baseNotificationID),
				start).First(&notification).Error

			if err == nil {
				// Notification exists, budget is exceeded
				llmBudgetExceeded = true
				llmUsage = 100
				log.Printf("LLM %d budget exceeded (notification exists)", llm.ID)
			} else {
				// No notification, use a conservative estimate
				llmSpent = 0
			}
		}

		// Calculate usage percentage if we have spending data
		if !llmBudgetExceeded {
			llmUsage = (llmSpent / *llm.MonthlyBudget) * 100

			// Check if budget is exceeded
			if llmSpent >= *llm.MonthlyBudget {
				llmBudgetExceeded = true
				log.Printf("LLM %d budget exceeded: %.2f/%.2f", llm.ID, llmSpent, *llm.MonthlyBudget)
			}
		}
	}

	// Queue budget analysis in the background instead of blocking
	go s.AnalyzeBudgetUsage(app, llm)

	// Return appropriate error if budget is exceeded
	if appBudgetExceeded {
		return 100, llmUsage, fmt.Errorf("app monthly budget exceeded")
	}

	if llmBudgetExceeded {
		return appUsage, 100, fmt.Errorf("LLM monthly budget exceeded")
	}

	return appUsage, llmUsage, nil
}

// getFromAtomicCache retrieves a value from the atomic cache without locking
func (s *BudgetService) getFromAtomicCache(key usageKey) (float64, bool) {
	// First check if the key exists with a read lock
	s.cacheMutex.RLock()
	cache, exists := s.atomicCache[key]
	s.cacheMutex.RUnlock()

	if !exists {
		return 0, false
	}

	// Read atomic values
	spentRaw := atomic.LoadUint64(&cache.spent)
	cachedAtNano := atomic.LoadInt64(&cache.cachedAt)

	// Convert to proper types
	spent := float64(spentRaw) / 10000.0
	cachedAt := time.Unix(0, cachedAtNano)

	// Check if cache is still valid
	if time.Since(cachedAt) < s.cacheDuration {
		return spent, true
	}

	return 0, false
}

// AnalyzeBudgetUsage analyzes current budget usage and sends notifications if thresholds are reached
// This now runs in the background and doesn't block the main request flow
func (s *BudgetService) AnalyzeBudgetUsage(app *models.App, llm *models.LLM) {
	now := time.Now()

	// Use a separate context with timeout for database operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check app budget
	if app != nil && app.MonthlyBudget != nil && *app.MonthlyBudget > 0 {
		start := s.calculateBudgetPeriodStart(app.BudgetStartDate, now)

		// Create cache key
		key := usageKey{
			entityID:   app.ID,
			entityType: "App",
			startDate:  start,
		}

		// Try to get from cache first
		spent, cacheHit := s.getFromAtomicCache(key)
		if !cacheHit {
			s.cacheMutex.RLock()
			data, exists := s.usageCache[key]
			if exists && time.Since(data.cachedAt) < s.cacheDuration {
				spent = data.spent
				cacheHit = true
			}
			s.cacheMutex.RUnlock()
		}

		// If cache miss, fetch from database
		var err error
		if !cacheHit {
			spent, err = s.fetchAppSpending(app.ID, start, now)
			if err != nil {
				log.Printf("Error calculating app spending: %v", err)
				return
			}

			// Update cache in the background
			s.updateCaches(key, spent, now)
		}

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
		err80 := s.db.WithContext(ctx).Where("(notification_id = ? OR notification_id LIKE ?) AND sent_at >= ?",
			fmt.Sprintf("%s_owner", baseNotificationID80),
			fmt.Sprintf("%s_admin_%%", baseNotificationID80),
			start).First(&existing80).Error
		err100 := s.db.WithContext(ctx).Where("(notification_id = ? OR notification_id LIKE ?) AND sent_at >= ?",
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

	// Check LLM budget
	if llm != nil && llm.MonthlyBudget != nil && *llm.MonthlyBudget > 0 {
		start := s.calculateBudgetPeriodStart(llm.BudgetStartDate, now)

		// Create cache key
		key := usageKey{
			entityID:   llm.ID,
			entityType: "LLM",
			startDate:  start,
		}

		// Try to get from cache first
		spent, cacheHit := s.getFromAtomicCache(key)
		if !cacheHit {
			s.cacheMutex.RLock()
			data, exists := s.usageCache[key]
			if exists && time.Since(data.cachedAt) < s.cacheDuration {
				spent = data.spent
				cacheHit = true
			}
			s.cacheMutex.RUnlock()
		}

		// If cache miss, fetch from database
		var err error
		if !cacheHit {
			spent, err = s.fetchLLMSpending(llm.ID, start, now)
			if err != nil {
				log.Printf("Error calculating LLM spending: %v", err)
				return
			}

			// Update cache in the background
			s.updateCaches(key, spent, now)
		}

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
		err80 := s.db.WithContext(ctx).Where("notification_id LIKE ? AND sent_at >= ?",
			fmt.Sprintf("%s_admin_%%", baseNotificationID80),
			start).First(&existing80).Error
		err100 := s.db.WithContext(ctx).Where("notification_id LIKE ? AND sent_at >= ?",
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

// ClearCache clears the spending cache, forcing next queries to hit the database
func (s *BudgetService) ClearCache() {
	s.cacheMutex.Lock()
	s.usageCache = make(map[usageKey]usageData)
	s.atomicCache = make(map[usageKey]*atomicUsageCache)
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

	// Calculate end of budget period
	var endDate time.Time
	if start.Month() == 12 {
		endDate = time.Date(start.Year()+1, 1, start.Day()-1, 23, 59, 59, 0, start.Location())
	} else {
		endDate = time.Date(start.Year(), start.Month()+1, start.Day()-1, 23, 59, 59, 0, start.Location())
	}

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
		"StartDate":    start,
		"EndDate":      endDate,
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

	// Calculate end of budget period
	var endDate time.Time
	if start.Month() == 12 {
		endDate = time.Date(start.Year()+1, 1, start.Day()-1, 23, 59, 59, 0, start.Location())
	} else {
		endDate = time.Date(start.Year(), start.Month()+1, start.Day()-1, 23, 59, 59, 0, start.Location())
	}

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
		"StartDate":    start,
		"EndDate":      endDate,
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
