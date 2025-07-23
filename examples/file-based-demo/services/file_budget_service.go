package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
)

// FileBudgetService implements aigateway.GatewayBudgetServiceInterface using JSON configuration files
type FileBudgetService struct {
	configDir  string
	appBudgets map[uint]*AppBudget
	llmBudgets map[uint]*LLMBudget
	mu         sync.RWMutex
}

// AppBudget represents budget tracking for an application
type AppBudget struct {
	AppID         uint            `json:"app_id"`
	MonthlyLimit  float64         `json:"monthly_limit"`
	CurrentUsage  float64         `json:"current_usage"`
	Currency      string          `json:"currency"`
	ResetDate     time.Time       `json:"reset_date"`
	Notifications map[string]bool `json:"notifications"`
}

// LLMBudget represents budget tracking for an LLM
type LLMBudget struct {
	LLMID        uint      `json:"llm_id"`
	MonthlyLimit float64   `json:"monthly_limit"`
	CurrentUsage float64   `json:"current_usage"`
	Currency     string    `json:"currency"`
	ResetDate    time.Time `json:"reset_date"`
}

// Budget configuration structures
type budgetConfig struct {
	AppID         uint            `json:"app_id"`
	MonthlyLimit  float64         `json:"monthly_limit"`
	CurrentUsage  float64         `json:"current_usage"`
	Currency      string          `json:"currency"`
	ResetDate     string          `json:"reset_date"`
	Notifications map[string]bool `json:"notifications"`
}

type llmBudgetConfig struct {
	LLMID        uint    `json:"llm_id"`
	MonthlyLimit float64 `json:"monthly_limit"`
	CurrentUsage float64 `json:"current_usage"`
	Currency     string  `json:"currency"`
	ResetDate    string  `json:"reset_date"`
}

// NewFileBudgetService creates a new file-based budget service
func NewFileBudgetService(configDir string) (*FileBudgetService, error) {
	service := &FileBudgetService{
		configDir:  configDir,
		appBudgets: make(map[uint]*AppBudget),
		llmBudgets: make(map[uint]*LLMBudget),
	}

	if err := service.loadBudgets(); err != nil {
		return nil, fmt.Errorf("failed to load budgets: %w", err)
	}

	return service, nil
}

// loadBudgets loads budget configurations from budgets.json
func (s *FileBudgetService) loadBudgets() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(filepath.Join(s.configDir, "budgets.json"))
	if err != nil {
		return err
	}

	var config struct {
		AppBudgets []budgetConfig    `json:"app_budgets"`
		LLMBudgets []llmBudgetConfig `json:"llm_budgets"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	// Load app budgets
	for _, budgetConf := range config.AppBudgets {
		resetDate, err := time.Parse(time.RFC3339, budgetConf.ResetDate)
		if err != nil {
			resetDate = time.Now().AddDate(0, 1, 0) // Default to next month
		}

		notifications := budgetConf.Notifications
		if notifications == nil {
			notifications = make(map[string]bool)
		}

		s.appBudgets[budgetConf.AppID] = &AppBudget{
			AppID:         budgetConf.AppID,
			MonthlyLimit:  budgetConf.MonthlyLimit,
			CurrentUsage:  budgetConf.CurrentUsage,
			Currency:      budgetConf.Currency,
			ResetDate:     resetDate,
			Notifications: notifications,
		}
	}

	// Load LLM budgets
	for _, llmBudgetConf := range config.LLMBudgets {
		resetDate, err := time.Parse(time.RFC3339, llmBudgetConf.ResetDate)
		if err != nil {
			resetDate = time.Now().AddDate(0, 1, 0) // Default to next month
		}

		s.llmBudgets[llmBudgetConf.LLMID] = &LLMBudget{
			LLMID:        llmBudgetConf.LLMID,
			MonthlyLimit: llmBudgetConf.MonthlyLimit,
			CurrentUsage: llmBudgetConf.CurrentUsage,
			Currency:     llmBudgetConf.Currency,
			ResetDate:    resetDate,
		}
	}

	return nil
}

// CheckBudget verifies if a request would exceed either App or LLM budget
func (s *FileBudgetService) CheckBudget(app *models.App, llm *models.LLM) (appUsage, llmUsage float64, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check app budget
	if appBudget, exists := s.appBudgets[app.ID]; exists {
		if appBudget.CurrentUsage >= appBudget.MonthlyLimit {
			return 100.0, 0.0, fmt.Errorf("app budget exceeded: %s has used $%.2f of $%.2f monthly limit",
				app.Name, appBudget.CurrentUsage, appBudget.MonthlyLimit)
		}
		appUsage = (appBudget.CurrentUsage / appBudget.MonthlyLimit) * 100.0
	}

	// Check LLM budget
	if llmBudget, exists := s.llmBudgets[llm.ID]; exists {
		if llmBudget.CurrentUsage >= llmBudget.MonthlyLimit {
			return appUsage, 100.0, fmt.Errorf("LLM budget exceeded: %s has used $%.2f of $%.2f monthly limit",
				llm.Name, llmBudget.CurrentUsage, llmBudget.MonthlyLimit)
		}
		llmUsage = (llmBudget.CurrentUsage / llmBudget.MonthlyLimit) * 100.0
	}

	return appUsage, llmUsage, nil
}

// AnalyzeBudgetUsage analyzes current budget usage and sends notifications if thresholds are reached
func (s *FileBudgetService) AnalyzeBudgetUsage(app *models.App, llm *models.LLM) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Analyze app budget usage
	if appBudget, exists := s.appBudgets[app.ID]; exists {
		usage := (appBudget.CurrentUsage / appBudget.MonthlyLimit) * 100.0

		// Check notification thresholds
		if usage >= 100.0 && !appBudget.Notifications["100_percent"] {
			fmt.Printf("[BUDGET ALERT] App '%s' has exceeded 100%% of monthly budget ($%.2f/$%.2f)\n",
				app.Name, appBudget.CurrentUsage, appBudget.MonthlyLimit)
			appBudget.Notifications["100_percent"] = true
		} else if usage >= 90.0 && !appBudget.Notifications["90_percent"] {
			fmt.Printf("[BUDGET WARNING] App '%s' has reached 90%% of monthly budget ($%.2f/$%.2f)\n",
				app.Name, appBudget.CurrentUsage, appBudget.MonthlyLimit)
			appBudget.Notifications["90_percent"] = true
		} else if usage >= 80.0 && !appBudget.Notifications["80_percent"] {
			fmt.Printf("[BUDGET WARNING] App '%s' has reached 80%% of monthly budget ($%.2f/$%.2f)\n",
				app.Name, appBudget.CurrentUsage, appBudget.MonthlyLimit)
			appBudget.Notifications["80_percent"] = true
		} else if usage >= 50.0 && !appBudget.Notifications["50_percent"] {
			fmt.Printf("[BUDGET INFO] App '%s' has reached 50%% of monthly budget ($%.2f/$%.2f)\n",
				app.Name, appBudget.CurrentUsage, appBudget.MonthlyLimit)
			appBudget.Notifications["50_percent"] = true
		}
	}

	// Analyze LLM budget usage
	if llmBudget, exists := s.llmBudgets[llm.ID]; exists {
		usage := (llmBudget.CurrentUsage / llmBudget.MonthlyLimit) * 100.0

		if usage >= 100.0 {
			fmt.Printf("[BUDGET ALERT] LLM '%s' has exceeded 100%% of monthly budget ($%.2f/$%.2f)\n",
				llm.Name, llmBudget.CurrentUsage, llmBudget.MonthlyLimit)
		} else if usage >= 90.0 {
			fmt.Printf("[BUDGET WARNING] LLM '%s' has reached 90%% of monthly budget ($%.2f/$%.2f)\n",
				llm.Name, llmBudget.CurrentUsage, llmBudget.MonthlyLimit)
		} else if usage >= 80.0 {
			fmt.Printf("[BUDGET WARNING] LLM '%s' has reached 80%% of monthly budget ($%.2f/$%.2f)\n",
				llm.Name, llmBudget.CurrentUsage, llmBudget.MonthlyLimit)
		}
	}
}

// AddUsage adds usage cost to both app and LLM budgets
func (s *FileBudgetService) AddUsage(appID, llmID uint, cost float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Add to app budget
	if appBudget, exists := s.appBudgets[appID]; exists {
		appBudget.CurrentUsage += cost
	}

	// Add to LLM budget
	if llmBudget, exists := s.llmBudgets[llmID]; exists {
		llmBudget.CurrentUsage += cost
	}
}

// GetAppBudget returns the current budget status for an app
func (s *FileBudgetService) GetAppBudget(appID uint) (*AppBudget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if budget, exists := s.appBudgets[appID]; exists {
		return budget, nil
	}
	return nil, fmt.Errorf("budget not found for app ID: %d", appID)
}

// GetLLMBudget returns the current budget status for an LLM
func (s *FileBudgetService) GetLLMBudget(llmID uint) (*LLMBudget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if budget, exists := s.llmBudgets[llmID]; exists {
		return budget, nil
	}
	return nil, fmt.Errorf("budget not found for LLM ID: %d", llmID)
}

// SaveBudgets persists the current budget state back to the JSON file
func (s *FileBudgetService) SaveBudgets() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	appBudgets := make([]budgetConfig, 0, len(s.appBudgets))
	for _, budget := range s.appBudgets {
		appBudgets = append(appBudgets, budgetConfig{
			AppID:         budget.AppID,
			MonthlyLimit:  budget.MonthlyLimit,
			CurrentUsage:  budget.CurrentUsage,
			Currency:      budget.Currency,
			ResetDate:     budget.ResetDate.Format(time.RFC3339),
			Notifications: budget.Notifications,
		})
	}

	llmBudgets := make([]llmBudgetConfig, 0, len(s.llmBudgets))
	for _, budget := range s.llmBudgets {
		llmBudgets = append(llmBudgets, llmBudgetConfig{
			LLMID:        budget.LLMID,
			MonthlyLimit: budget.MonthlyLimit,
			CurrentUsage: budget.CurrentUsage,
			Currency:     budget.Currency,
			ResetDate:    budget.ResetDate.Format(time.RFC3339),
		})
	}

	config := struct {
		AppBudgets []budgetConfig    `json:"app_budgets"`
		LLMBudgets []llmBudgetConfig `json:"llm_budgets"`
	}{
		AppBudgets: appBudgets,
		LLMBudgets: llmBudgets,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(s.configDir, "budgets.json"), data, 0644)
}

// Reload reloads budget configurations from file
func (s *FileBudgetService) Reload() error {
	return s.loadBudgets()
}

// Ensure FileBudgetService implements the interface
var _ aigateway.GatewayBudgetServiceInterface = (*FileBudgetService)(nil)
