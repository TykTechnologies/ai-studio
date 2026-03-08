// Package services provides file-based implementations of the AI Gateway service interfaces.
// This allows the demo to run without requiring a database, using JSON configuration files instead.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// FileGatewayService implements services.ServiceInterface using JSON configuration files
type FileGatewayService struct {
	configDir string
	llms      []models.LLM
	apps      []models.App
	creds     []models.Credential
	pricing   []models.ModelPrice
	filters   []models.Filter
}

// Configuration file structures that match our JSON files
type llmConfig struct {
	ID            uint    `json:"id"`
	Name          string  `json:"name"`
	Vendor        string  `json:"vendor"`
	Endpoint      string  `json:"endpoint"`
	APIKey        string  `json:"api_key"`
	Model         string  `json:"model"`
	Active        bool    `json:"active"`
	MaxTokens     int     `json:"max_tokens"`
	MonthlyBudget float64 `json:"monthly_budget"`
	FilterIDs     []uint  `json:"filter_ids"`
}

type credentialConfig struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Secret      string `json:"secret"`
	Active      bool   `json:"active"`
	Description string `json:"description"`
}

type appConfig struct {
	ID              uint    `json:"id"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	UserID          uint    `json:"user_id"`
	CredentialID    uint    `json:"credential_id"`
	LLMIDs          []uint  `json:"llm_ids"`
	DatasourceIDs   []uint  `json:"datasource_ids"`
	ToolIDs         []uint  `json:"tool_ids"`
	MonthlyBudget   float64 `json:"monthly_budget"`
	BudgetStartDate string  `json:"budget_start_date"`
}

type pricingConfig struct {
	ID            uint    `json:"id"`
	Model         string  `json:"model"`
	Vendor        string  `json:"vendor"`
	PromptPrice   float64 `json:"prompt_price"`
	ResponsePrice float64 `json:"response_price"`
	Currency      string  `json:"currency"`
	PerTokens     int     `json:"per_tokens"`
}

type filterConfig struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Script      string `json:"script"`
}

// NewFileGatewayService creates a new file-based gateway service
func NewFileGatewayService(configDir string) (*FileGatewayService, error) {
	service := &FileGatewayService{
		configDir: configDir,
	}

	if err := service.loadConfigurations(); err != nil {
		return nil, fmt.Errorf("failed to load configurations: %w", err)
	}

	return service, nil
}

// loadConfigurations loads all configuration files
func (s *FileGatewayService) loadConfigurations() error {
	// Load filters first since LLMs may reference them
	if err := s.loadFilters(); err != nil {
		return fmt.Errorf("failed to load filters: %w", err)
	}

	if err := s.loadLLMs(); err != nil {
		return fmt.Errorf("failed to load LLMs: %w", err)
	}

	if err := s.loadCredentials(); err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}

	if err := s.loadApps(); err != nil {
		return fmt.Errorf("failed to load apps: %w", err)
	}

	if err := s.loadPricing(); err != nil {
		return fmt.Errorf("failed to load pricing: %w", err)
	}

	return nil
}

// loadLLMs loads LLM configurations from llms.json
func (s *FileGatewayService) loadLLMs() error {
	data, err := os.ReadFile(filepath.Join(s.configDir, "llms.json"))
	if err != nil {
		return err
	}

	var config struct {
		LLMs []llmConfig `json:"llms"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	s.llms = make([]models.LLM, len(config.LLMs))
	for i, llmConf := range config.LLMs {
		// Resolve environment variables in API key
		apiKey := llmConf.APIKey
		if strings.HasPrefix(apiKey, "$") {
			envVar := strings.TrimPrefix(apiKey, "$")
			if envValue := os.Getenv(envVar); envValue != "" {
				apiKey = envValue
			}
		}

		// Find associated filters
		var filters []*models.Filter
		for _, filterID := range llmConf.FilterIDs {
			for j := range s.filters {
				if s.filters[j].ID == filterID {
					filters = append(filters, &s.filters[j])
					break
				}
			}
		}

		s.llms[i] = models.LLM{
			ID:            llmConf.ID,
			Name:          llmConf.Name,
			Vendor:        models.Vendor(llmConf.Vendor),
			APIEndpoint:   llmConf.Endpoint,
			APIKey:        apiKey,
			DefaultModel:  llmConf.Model,
			Active:        llmConf.Active,
			MonthlyBudget: &llmConf.MonthlyBudget,
			Filters:       filters,
		}
	}

	return nil
}

// loadCredentials loads credential configurations from credentials.json
func (s *FileGatewayService) loadCredentials() error {
	data, err := os.ReadFile(filepath.Join(s.configDir, "credentials.json"))
	if err != nil {
		return err
	}

	var config struct {
		Credentials []credentialConfig `json:"credentials"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	s.creds = make([]models.Credential, len(config.Credentials))
	for i, credConf := range config.Credentials {
		s.creds[i] = models.Credential{
			ID:     credConf.ID,
			KeyID:  credConf.Name, // Use name as KeyID for demo
			Secret: credConf.Secret,
			Active: credConf.Active,
		}
	}

	return nil
}

// loadApps loads app configurations from apps.json
func (s *FileGatewayService) loadApps() error {
	data, err := os.ReadFile(filepath.Join(s.configDir, "apps.json"))
	if err != nil {
		return err
	}

	var config struct {
		Apps []appConfig `json:"apps"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	s.apps = make([]models.App, len(config.Apps))
	for i, appConf := range config.Apps {
		// Parse budget start date
		var budgetStartDate *time.Time
		if appConf.BudgetStartDate != "" {
			if t, err := time.Parse(time.RFC3339, appConf.BudgetStartDate); err == nil {
				budgetStartDate = &t
			}
		}

		// Find credential
		var credential models.Credential
		for _, cred := range s.creds {
			if cred.ID == appConf.CredentialID {
				credential = cred
				break
			}
		}

		// Find associated LLMs
		llms := make([]models.LLM, 0, len(appConf.LLMIDs))
		for _, llmID := range appConf.LLMIDs {
			for _, llm := range s.llms {
				if llm.ID == llmID {
					llms = append(llms, llm)
					break
				}
			}
		}

		s.apps[i] = models.App{
			ID:              appConf.ID,
			Name:            appConf.Name,
			Description:     appConf.Description,
			UserID:          appConf.UserID,
			CredentialID:    appConf.CredentialID,
			Credential:      credential,
			MonthlyBudget:   &appConf.MonthlyBudget,
			BudgetStartDate: budgetStartDate,
			LLMs:            llms,
		}
	}

	return nil
}

// loadPricing loads pricing configurations from pricing.json
func (s *FileGatewayService) loadPricing() error {
	data, err := os.ReadFile(filepath.Join(s.configDir, "pricing.json"))
	if err != nil {
		return err
	}

	var config struct {
		ModelPrices []pricingConfig `json:"model_prices"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	s.pricing = make([]models.ModelPrice, len(config.ModelPrices))
	for i, priceConf := range config.ModelPrices {
		s.pricing[i] = models.ModelPrice{
			ID:        priceConf.ID,
			ModelName: priceConf.Model,
			Vendor:    priceConf.Vendor,
			CPT:       priceConf.PromptPrice,
			CPIT:      priceConf.ResponsePrice,
			Currency:  priceConf.Currency,
		}
	}

	return nil
}

// loadFilters loads filter configurations from filters.json
func (s *FileGatewayService) loadFilters() error {
	data, err := os.ReadFile(filepath.Join(s.configDir, "filters.json"))
	if err != nil {
		// If filters.json doesn't exist, just continue with empty filters
		s.filters = []models.Filter{}
		return nil
	}

	var config struct {
		Filters []filterConfig `json:"filters"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	s.filters = make([]models.Filter, len(config.Filters))
	for i, filterConf := range config.Filters {
		s.filters[i] = models.Filter{
			ID:          filterConf.ID,
			Name:        filterConf.Name,
			Description: filterConf.Description,
			Script:      []byte(filterConf.Script),
		}
	}

	return nil
}

// GetActiveLLMs returns all active LLMs
func (s *FileGatewayService) GetActiveLLMs(ctx context.Context) ([]models.LLM, error) {
	var activeLLMs []models.LLM
	for _, llm := range s.llms {
		if llm.Active {
			activeLLMs = append(activeLLMs, llm)
		}
	}
	return activeLLMs, nil
}

// GetActiveDatasources returns all active datasources (empty for this demo)
func (s *FileGatewayService) GetActiveDatasources() ([]models.Datasource, error) {
	return []models.Datasource{}, nil
}

// GetToolBySlug returns a tool by its slug (not implemented for this demo)
func (s *FileGatewayService) GetToolBySlug(ctx context.Context, slug string) (*models.Tool, error) {
	return nil, fmt.Errorf("tool not found: %s", slug)
}

// GetCredentialBySecret returns a credential by its secret
func (s *FileGatewayService) GetCredentialBySecret(secret string) (*models.Credential, error) {
	for _, cred := range s.creds {
		if cred.Secret == secret && cred.Active {
			return &cred, nil
		}
	}
	return nil, fmt.Errorf("credential not found or inactive")
}

// GetAppByID returns an app by its ID
func (s *FileGatewayService) GetAppByID(id uint) (*models.App, error) {
	for _, app := range s.apps {
		if app.ID == id {
			return &app, nil
		}
	}
	return nil, fmt.Errorf("app not found for ID: %d", id)
}

// GetAppByCredentialID returns an app by its credential ID
func (s *FileGatewayService) GetAppByCredentialID(credID uint) (*models.App, error) {
	for _, app := range s.apps {
		if app.CredentialID == credID {
			return &app, nil
		}
	}
	return nil, fmt.Errorf("app not found for credential ID: %d", credID)
}

// GetModelPriceByModelNameAndVendor returns pricing for a specific model and vendor
func (s *FileGatewayService) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	for _, price := range s.pricing {
		if price.ModelName == modelName && price.Vendor == vendor {
			return &price, nil
		}
	}
	return nil, fmt.Errorf("pricing not found for model %s from vendor %s", modelName, vendor)
}

// CallToolOperation executes a tool operation (not implemented for this demo)
func (s *FileGatewayService) CallToolOperation(toolID uint, operationID string, params map[string][]string, payload map[string]interface{}, headers map[string][]string) (interface{}, error) {
	return nil, fmt.Errorf("tool operations not supported in file-based demo")
}

// GetDB returns nil as we don't use a database
func (s *FileGatewayService) GetDB() interface{} {
	return nil
}

// GetLLMByID returns an LLM by its ID
func (s *FileGatewayService) GetLLMByID(ctx context.Context, id uint) (*models.LLM, error) {
	for _, llm := range s.llms {
		if llm.ID == id {
			return &llm, nil
		}
	}
	return nil, fmt.Errorf("LLM not found: %d", id)
}

// GetLLMSettingsByID returns LLM settings by ID (not implemented for demo)
func (s *FileGatewayService) GetLLMSettingsByID(id uint) (*models.LLMSettings, error) {
	return nil, fmt.Errorf("LLM settings not supported in file-based demo")
}

// GetDatasourceByID returns a datasource by its ID (not implemented for demo)
func (s *FileGatewayService) GetDatasourceByID(ctx context.Context, id uint) (*models.Datasource, error) {
	return nil, fmt.Errorf("datasource not found: %d", id)
}

// AuthenticateUser authenticates a user (not implemented for demo)
func (s *FileGatewayService) AuthenticateUser(email, password string) (*models.User, error) {
	return nil, fmt.Errorf("user authentication not supported in file-based demo")
}

// GetUserByAPIKey returns a user by API key (not implemented for demo)
func (s *FileGatewayService) GetUserByAPIKey(apiKey string) (*models.User, error) {
	return nil, fmt.Errorf("API key authentication not supported in file-based demo")
}

// GetUserByEmail returns a user by email (not implemented for demo)
func (s *FileGatewayService) GetUserByEmail(email string) (*models.User, error) {
	return nil, fmt.Errorf("user lookup by email not supported in file-based demo")
}

// GetUserByID returns a mock user (not implemented for this demo)
func (s *FileGatewayService) GetUserByID(id uint, preload ...string) (*models.User, error) {
	return &models.User{
		ID:    id,
		Email: fmt.Sprintf("user%d@example.com", id),
		Name:  fmt.Sprintf("Demo User %d", id),
	}, nil
}

// AddUserToGroup adds a user to a group (not implemented for demo)
func (s *FileGatewayService) AddUserToGroup(userID, groupID uint) error {
	return fmt.Errorf("user group management not supported in file-based demo")
}

// GetValidAccessTokenByToken returns an error since OAuth is not supported in file-based demo
func (s *FileGatewayService) GetValidAccessTokenByToken(token string) (*models.AccessToken, error) {
	return nil, fmt.Errorf("OAuth access tokens not supported in file-based demo")
}

// GetOAuthClient returns an error since OAuth is not supported in file-based demo
func (s *FileGatewayService) GetOAuthClient(clientID string) (*models.OAuthClient, error) {
	return nil, fmt.Errorf("OAuth clients not supported in file-based demo")
}

// GetToolByID returns a tool by its ID (not implemented for demo)
func (s *FileGatewayService) GetToolByID(ctx context.Context, id uint) (*models.Tool, error) {
	return nil, fmt.Errorf("tool not found: %d", id)
}

// GetFilterByID returns a filter by its ID
func (s *FileGatewayService) GetFilterByID(id uint) (*models.Filter, error) {
	for _, filter := range s.filters {
		if filter.ID == id {
			return &filter, nil
		}
	}
	return nil, fmt.Errorf("filter not found: %d", id)
}

// GetAllFilters returns all filters (simplified version for file-based demo)
func (s *FileGatewayService) GetAllFilters(pageSize int, pageNumber int, all bool) ([]models.Filter, int64, int, error) {
	// For simplicity, ignore pagination in file-based demo and return all filters
	totalCount := int64(len(s.filters))
	totalPages := 1
	if pageSize > 0 && !all {
		totalPages = int((totalCount + int64(pageSize) - 1) / int64(pageSize))
	}
	return s.filters, totalCount, totalPages, nil
}

// Reload reloads all configuration files
func (s *FileGatewayService) Reload() error {
	return s.loadConfigurations()
}
