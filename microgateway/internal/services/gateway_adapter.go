// internal/services/gateway_adapter.go
package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
)

// GatewayServiceAdapter adapts our DatabaseGatewayService to implement services.ServiceInterface
type GatewayServiceAdapter struct {
	gatewayService GatewayServiceInterface
	management     ManagementServiceInterface
	analytics      AnalyticsServiceInterface
}

// NewGatewayServiceAdapter creates a new adapter that implements services.ServiceInterface
func NewGatewayServiceAdapter(
	gatewayService GatewayServiceInterface,
	management ManagementServiceInterface,
	analytics AnalyticsServiceInterface,
) services.ServiceInterface {
	return &GatewayServiceAdapter{
		gatewayService: gatewayService,
		management:     management,
		analytics:      analytics,
	}
}

// GetActiveLLMs returns all active LLMs
func (a *GatewayServiceAdapter) GetActiveLLMs() ([]models.LLM, error) {
	llmInterfaces, err := a.gatewayService.GetActiveLLMs()
	if err != nil {
		return nil, err
	}

	// Convert from interface{} to models.LLM
	llms := make([]models.LLM, len(llmInterfaces))
	for i, llmInterface := range llmInterfaces {
		if dbLLM, ok := llmInterface.(*database.LLM); ok {
			llm := a.convertDatabaseLLMToModel(dbLLM)
			llms[i] = llm
		} else {
			return nil, fmt.Errorf("unexpected LLM type at index %d", i)
		}
	}

	return llms, nil
}

// GetLLMByID returns an LLM by its ID
func (a *GatewayServiceAdapter) GetLLMByID(id uint) (*models.LLM, error) {
	dbLLM, err := a.management.GetLLM(id)
	if err != nil {
		return nil, err
	}

	llm := a.convertDatabaseLLMToModel(dbLLM)
	return &llm, nil
}

// GetLLMSettingsByID returns LLM settings (not implemented for now)
func (a *GatewayServiceAdapter) GetLLMSettingsByID(id uint) (*models.LLMSettings, error) {
	return nil, fmt.Errorf("LLM settings not implemented in microgateway")
}

// GetActiveDatasources returns active datasources (empty for microgateway)
func (a *GatewayServiceAdapter) GetActiveDatasources() ([]models.Datasource, error) {
	return []models.Datasource{}, nil
}

// GetDatasourceByID returns a datasource by ID (not implemented)
func (a *GatewayServiceAdapter) GetDatasourceByID(id uint) (*models.Datasource, error) {
	return nil, fmt.Errorf("datasource with ID %d not found", id)
}

// GetCredentialBySecret returns a credential by secret
func (a *GatewayServiceAdapter) GetCredentialBySecret(secret string) (*models.Credential, error) {
	credInterface, err := a.gatewayService.GetCredentialBySecret(secret)
	if err != nil {
		return nil, err
	}

	if dbCred, ok := credInterface.(*database.Credential); ok {
		cred := a.convertDatabaseCredentialToModel(dbCred)
		return &cred, nil
	}

	return nil, fmt.Errorf("unexpected credential type")
}

// AuthenticateUser authenticates a user (not implemented for microgateway)
func (a *GatewayServiceAdapter) AuthenticateUser(email, password string) (*models.User, error) {
	return nil, fmt.Errorf("user authentication not supported in microgateway")
}

// GetUserByAPIKey returns a user by API key (not implemented)
func (a *GatewayServiceAdapter) GetUserByAPIKey(apiKey string) (*models.User, error) {
	return nil, fmt.Errorf("user API key authentication not supported in microgateway")
}

// GetUserByEmail returns a user by email (not implemented)
func (a *GatewayServiceAdapter) GetUserByEmail(email string) (*models.User, error) {
	return nil, fmt.Errorf("user lookup by email not supported in microgateway")
}

// GetUserByID returns a user by ID (simplified implementation)
func (a *GatewayServiceAdapter) GetUserByID(id uint, preload ...string) (*models.User, error) {
	// Return a basic user for compatibility
	return &models.User{
		ID:    id,
		Email: fmt.Sprintf("user%d@microgateway.local", id),
		Name:  fmt.Sprintf("User %d", id),
	}, nil
}

// AddUserToGroup adds a user to a group (not implemented)
func (a *GatewayServiceAdapter) AddUserToGroup(userID, groupID uint) error {
	return fmt.Errorf("user group management not supported in microgateway")
}

// GetValidAccessTokenByToken returns an access token (not implemented)
func (a *GatewayServiceAdapter) GetValidAccessTokenByToken(token string) (*models.AccessToken, error) {
	return nil, fmt.Errorf("OAuth access tokens not supported in microgateway")
}

// GetOAuthClient returns an OAuth client (not implemented)
func (a *GatewayServiceAdapter) GetOAuthClient(clientID string) (*models.OAuthClient, error) {
	return nil, fmt.Errorf("OAuth clients not supported in microgateway")
}

// GetAppByCredentialID returns an app by credential ID
func (a *GatewayServiceAdapter) GetAppByCredentialID(credID uint) (*models.App, error) {
	appInterface, err := a.gatewayService.GetAppByCredentialID(credID)
	if err != nil {
		return nil, err
	}

	if dbApp, ok := appInterface.(*database.App); ok {
		app := a.convertDatabaseAppToModel(dbApp)
		return &app, nil
	}

	return nil, fmt.Errorf("unexpected app type")
}

// GetToolByID returns a tool by ID (not implemented)
func (a *GatewayServiceAdapter) GetToolByID(id uint) (*models.Tool, error) {
	return nil, fmt.Errorf("tool with ID %d not found", id)
}

// GetToolBySlug returns a tool by slug (not implemented)
func (a *GatewayServiceAdapter) GetToolBySlug(slug string) (*models.Tool, error) {
	return nil, fmt.Errorf("tool with slug %s not found", slug)
}

// CallToolOperation executes a tool operation (not implemented)
func (a *GatewayServiceAdapter) CallToolOperation(toolID uint, operationID string, params map[string][]string, payload map[string]interface{}, headers map[string][]string) (interface{}, error) {
	return nil, fmt.Errorf("tool operations not supported in microgateway")
}

// GetModelPriceByModelNameAndVendor returns model pricing (placeholder implementation)
func (a *GatewayServiceAdapter) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	// Return basic pricing for common models
	return &models.ModelPrice{
		ID:        1,
		ModelName: modelName,
		Vendor:    vendor,
		CPT:       0.001,  // $0.001 per token (placeholder)
		CPIT:      0.002,  // $0.002 per token (placeholder)
		Currency:  "USD",
	}, nil
}

// GetFilterByID returns a filter by ID (not implemented for now)
func (a *GatewayServiceAdapter) GetFilterByID(id uint) (*models.Filter, error) {
	return nil, fmt.Errorf("filter with ID %d not found", id)
}

// GetAllFilters returns all filters (not implemented for now)
func (a *GatewayServiceAdapter) GetAllFilters(pageSize int, pageNumber int, all bool) ([]models.Filter, int64, int, error) {
	return []models.Filter{}, 0, 0, nil
}

// Conversion helper functions
func (a *GatewayServiceAdapter) convertDatabaseLLMToModel(dbLLM *database.LLM) models.LLM {
	return models.LLM{
		ID:          dbLLM.ID,
		Name:        dbLLM.Name,
		Vendor:      models.Vendor(dbLLM.Vendor),
		APIKey:      "", // Don't expose encrypted API keys
		APIEndpoint: dbLLM.Endpoint,
		DefaultModel: dbLLM.DefaultModel,
		Active:      dbLLM.IsActive,
		MonthlyBudget: &dbLLM.MonthlyBudget,
	}
}

func (a *GatewayServiceAdapter) convertDatabaseCredentialToModel(dbCred *database.Credential) models.Credential {
	return models.Credential{
		ID:     dbCred.ID,
		KeyID:  dbCred.KeyID,
		Secret: dbCred.SecretHash, // Note: this is the hashed version
		Active: dbCred.IsActive,
	}
}

func (a *GatewayServiceAdapter) convertDatabaseAppToModel(dbApp *database.App) models.App {
	return models.App{
		ID:              dbApp.ID,
		Name:            dbApp.Name,
		Description:     dbApp.Description,
		UserID:          1, // Default user ID for microgateway
		CredentialID:    dbApp.ID, // Use app ID as credential reference
		MonthlyBudget:   &dbApp.MonthlyBudget,
		BudgetStartDate: dbApp.BudgetStartDate,
	}
}