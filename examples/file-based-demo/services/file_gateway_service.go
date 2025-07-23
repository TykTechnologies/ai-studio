// Package services provides file-based implementations of the AI Gateway service interfaces.
// This allows the demo to run without requiring a database, using JSON configuration files instead.
package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
)

// FileGatewayService implements aigateway.GatewayServiceInterface using JSON configuration files
type FileGatewayService struct {
	configDir string
	llms      []models.LLM
	apps      []models.App
	creds     []models.Credential
	pricing   []models.ModelPrice
}

// Configuration file structures that match our JSON files
type llmConfig struct {
	ID            uint    `json:"id"`
	Name          string  `json:"name"`
	Slug          string  `json:"slug"`
	Vendor        string  `json:"vendor"`
	Endpoint      string  `json:"endpoint"`
	APIKey        string  `json:"api_key"`
	Model         string  `json:"model"`
	Active        bool    `json:"active"`
	MaxTokens     int     `json:"max_tokens"`
	MonthlyBudget float64 `json:"monthly_budget"`
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

		s.llms[i] = models.LLM{
			ID:            llmConf.ID,
			Name:          llmConf.Name,
			Vendor:        models.Vendor(llmConf.Vendor),
			APIEndpoint:   llmConf.Endpoint,
			APIKey:        apiKey,
			DefaultModel:  llmConf.Model,
			Active:        llmConf.Active,
			MonthlyBudget: &llmConf.MonthlyBudget,
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

// GetActiveLLMs returns all active LLMs
func (s *FileGatewayService) GetActiveLLMs() ([]models.LLM, error) {
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
func (s *FileGatewayService) GetToolBySlug(slug string) (*models.Tool, error) {
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

// GetUserByID returns a mock user (not implemented for this demo)
func (s *FileGatewayService) GetUserByID(id uint) (*models.User, error) {
	return &models.User{
		ID:    id,
		Email: fmt.Sprintf("user%d@example.com", id),
		Name:  fmt.Sprintf("Demo User %d", id),
	}, nil
}

// Reload reloads all configuration files
func (s *FileGatewayService) Reload() error {
	return s.loadConfigurations()
}

// Ensure FileGatewayService implements the interface
var _ aigateway.GatewayServiceInterface = (*FileGatewayService)(nil)
