package main

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MockDependencies implements the ProxyDependencies interface
type MockDependencies struct {
	service       *MockService
	budgetService *MockBudgetService
	authService   *MockAuthService
}

// NewMockDependencies creates a new instance of MockDependencies using the provided config
func NewMockDependencies(cfg *Config) *MockDependencies {
	service := NewMockService(cfg)
	budgetService := NewMockBudgetService()
	authService := NewMockAuthService(cfg)

	return &MockDependencies{
		service:       service,
		budgetService: budgetService,
		authService:   authService,
	}
}

// GetService returns the mocked proxy service interface
func (d *MockDependencies) GetService() proxy.ProxyServiceInterface {
	return d.service
}

// GetBudgetService returns the mocked budget service interface
func (d *MockDependencies) GetBudgetService() proxy.BudgetServiceInterface {
	return d.budgetService
}

// GetAuthService returns the mocked auth service interface
func (d *MockDependencies) GetAuthService() proxy.AuthServiceInterface {
	return d.authService
}

// MockService implements the ProxyServiceInterface
type MockService struct {
	llms        map[uint]*models.LLM
	datasources map[uint]*models.Datasource
	credentials map[string]*models.Credential
	apps        map[uint]*models.App
	users       map[string]*models.User
	prices      map[string]*models.ModelPrice
}

// NewMockService creates a new MockService with data from the config
func NewMockService(cfg *Config) *MockService {
	service := &MockService{
		llms:        make(map[uint]*models.LLM),
		datasources: make(map[uint]*models.Datasource),
		credentials: make(map[string]*models.Credential),
		apps:        make(map[uint]*models.App),
		users:       make(map[string]*models.User),
		prices:      make(map[string]*models.ModelPrice),
	}

	// Populate the service with data from config
	for i, llm := range cfg.LLMs {
		if llm.Active {
			id := uint(i + 1)
			modelLLM := &models.LLM{
				ID:          id,
				Name:        llm.Name,
				Vendor:      models.Vendor(llm.Vendor),
				APIEndpoint: llm.APIEndpoint,
				APIKey:      llm.APIKey,
				Active:      true,
			}
			service.llms[id] = modelLLM
		}
	}

	for i, ds := range cfg.Datasources {
		if ds.Active {
			id := uint(i + 1)
			modelDS := &models.Datasource{
				ID:           id,
				Name:         ds.Name,
				DBSourceType: ds.Type,
				Active:       true,
			}
			service.datasources[id] = modelDS
		}
	}

	// Setup users and their credentials
	for _, user := range cfg.Users {
		id := user.ID
		// Create user
		modelUser := &models.User{
			ID:    id,
			Email: user.Email,
		}
		service.users[user.APIKey] = modelUser

		// Create credential
		cred := &models.Credential{
			ID:     id,
			Secret: user.APIKey,
			KeyID:  "key_" + user.APIKey,
			Active: true,
		}
		fmt.Println("Adding credential:", cred.Secret)
		service.credentials[cred.Secret] = cred

		// Create app
		llmArr := make([]models.LLM, 0)
		for _, llm := range service.llms {
			if llm.Active {
				llmArr = append(llmArr, *llm)
			}
		}

		app := &models.App{
			ID:           id,
			UserID:       id,
			CredentialID: id,
			Name:         "App for " + user.Email,
			LLMs:         llmArr,
		}
		service.apps[id] = app
	}

	return service
}

// GetActiveLLMs returns all active LLMs
func (s *MockService) GetActiveLLMs() (models.LLMs, error) {
	llms := models.LLMs{}
	for _, llm := range s.llms {
		if llm.Active {
			llms = append(llms, *llm)
		}
	}
	return llms, nil
}

// GetLLMByID returns an LLM by ID
func (s *MockService) GetLLMByID(id uint) (*models.LLM, error) {
	llm, ok := s.llms[id]
	if !ok {
		return nil, nil
	}
	return llm, nil
}

// GetLLMSettingsByID returns LLM settings by ID
func (s *MockService) GetLLMSettingsByID(id uint) (*models.LLMSettings, error) {
	// For demonstration, return simple settings
	return &models.LLMSettings{
		ID:          id,
		MaxTokens:   2000,
		Temperature: 0.7,
		TopP:        0.9,
	}, nil
}

// GetActiveDatasources returns all active datasources
func (s *MockService) GetActiveDatasources() (models.Datasources, error) {
	datasources := models.Datasources{}
	for _, ds := range s.datasources {
		if ds.Active {
			datasources = append(datasources, *ds)
		}
	}
	return datasources, nil
}

// GetDatasourceByID returns a datasource by ID
func (s *MockService) GetDatasourceByID(id uint) (*models.Datasource, error) {
	ds, ok := s.datasources[id]
	if !ok {
		return nil, nil
	}
	return ds, nil
}

// GetCredentialBySecret returns a credential by secret
func (s *MockService) GetCredentialBySecret(secret string) (*models.Credential, error) {
	cred, ok := s.credentials[secret]
	if !ok {
		return nil, fmt.Errorf("credential not found")
	}
	return cred, nil
}

// GetAppByCredentialID returns an app by credential ID
func (s *MockService) GetAppByCredentialID(credID uint) (*models.App, error) {
	app, ok := s.apps[credID]
	if !ok {
		return nil, nil
	}
	return app, nil
}

// GetModelPriceByModelNameAndVendor returns a model price by model name and vendor
func (s *MockService) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	key := vendor + ":" + modelName
	price, ok := s.prices[key]
	if !ok {
		// Create a default price
		// Create a default price for the LLMChatRecord
		price = &models.ModelPrice{
			ModelName: modelName,
			Vendor:    vendor,
		}
		s.prices[key] = price
	}
	return price, nil
}

// GetDB returns a mock DB connection (nil for our mock implementation)
func (s *MockService) GetDB() *gorm.DB {
	return nil
}

// AuthenticateUser authenticates a user by email and password
func (s *MockService) AuthenticateUser(email, password string) (*models.User, error) {
	// Simplified implementation that always returns the first user
	for _, user := range s.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, nil
}

// GetUserByAPIKey returns a user by API key
func (s *MockService) GetUserByAPIKey(apiKey string) (*models.User, error) {
	user, ok := s.users[apiKey]
	if !ok {
		return nil, nil
	}
	return user, nil
}

// GetUserByEmail returns a user by email
func (s *MockService) GetUserByEmail(email string) (*models.User, error) {
	for _, user := range s.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, nil
}

// AddUserToGroup adds a user to a group (no-op in our mock)
func (s *MockService) AddUserToGroup(userID, groupID uint) error {
	// No-op for mock implementation
	return nil
}

// MockBudgetService implements the BudgetServiceInterface
type MockBudgetService struct{}

// NewMockBudgetService creates a new MockBudgetService
func NewMockBudgetService() *MockBudgetService {
	return &MockBudgetService{}
}

// CheckBudget checks if a request would exceed budget (always returns 0% usage in our mock)
func (b *MockBudgetService) CheckBudget(app *models.App, llm *models.LLM) (float64, float64, error) {
	// For demo purposes, always return 0% usage
	return 0.0, 0.0, nil
}

// AnalyzeBudgetUsage analyzes budget usage (no-op in our mock)
func (b *MockBudgetService) AnalyzeBudgetUsage(app *models.App, llm *models.LLM) {
	// No-op for mock implementation
}

// ClearCache clears the spending cache (no-op in our mock)
func (b *MockBudgetService) ClearCache() {
	// No-op for mock implementation
}

// MockAuthService implements the AuthServiceInterface
type MockAuthService struct {
	users map[string]*models.User
}

// NewMockAuthService creates a new MockAuthService
func NewMockAuthService(cfg *Config) *MockAuthService {
	service := &MockAuthService{
		users: make(map[string]*models.User),
	}

	// Setup users
	for i, user := range cfg.Users {
		id := uint(i + 1)
		service.users[user.APIKey] = &models.User{
			ID:    id,
			Email: user.Email,
		}
	}

	return service
}

// GetAuthenticatedUser returns an authenticated user (always returns null in our mock)
func (a *MockAuthService) GetAuthenticatedUser(c *gin.Context) *models.User {
	// For demo purposes, always return nil (bypassing authentication)
	return nil
}

// AuthMiddleware provides auth middleware (no-op in our mock)
func (a *MockAuthService) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// No-op for mock implementation - bypass authentication
		c.Next()
	}
}

// SetUserSession sets a user session (no-op in our mock)
func (a *MockAuthService) SetUserSession(c *gin.Context, user *models.User) error {
	// No-op for mock implementation
	return nil
}
