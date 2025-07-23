package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

// BudgetServiceInterface defines budget operations needed by the application.
// This interface allows implementations to use databases, files, or other storage backends.
type BudgetServiceInterface interface {
	CheckBudget(app *models.App, llm *models.LLM) (float64, float64, error)
	AnalyzeBudgetUsage(app *models.App, llm *models.LLM)
}

// ServiceInterface defines the core operations needed by the application.
// This interface abstracts the data layer, allowing implementations to use databases,
// files, APIs, or any other backend while providing the same functionality.
//
// This unified interface consolidates functionality from the original ServiceInterface
// and GatewayServiceInterface to eliminate duplication and confusion.
type ServiceInterface interface {
	// LLM Management
	GetActiveLLMs() ([]models.LLM, error)
	GetLLMByID(id uint) (*models.LLM, error)
	GetLLMSettingsByID(id uint) (*models.LLMSettings, error)

	// Datasource Management
	GetActiveDatasources() ([]models.Datasource, error)
	GetDatasourceByID(id uint) (*models.Datasource, error)

	// Authentication & Authorization
	GetCredentialBySecret(secret string) (*models.Credential, error)
	AuthenticateUser(email, password string) (*models.User, error)
	GetUserByAPIKey(apiKey string) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	GetUserByID(id uint, preload ...string) (*models.User, error)
	AddUserToGroup(userID, groupID uint) error

	// OAuth Authentication (returns errors for implementations that don't support OAuth)
	GetValidAccessTokenByToken(token string) (*models.AccessToken, error)
	GetOAuthClient(clientID string) (*models.OAuthClient, error)

	// App Management
	GetAppByCredentialID(credID uint) (*models.App, error)

	// Tool Management
	GetToolByID(id uint) (*models.Tool, error)
	GetToolBySlug(slug string) (*models.Tool, error)
	CallToolOperation(toolID uint, operationID string, params map[string][]string, payload map[string]interface{}, headers map[string][]string) (interface{}, error)

	// Analytics & Pricing
	GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error)
}
