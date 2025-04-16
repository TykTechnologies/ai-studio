package proxy

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// LLMProvider defines the interface for accessing LLM data
type LLMProvider interface {
	GetActiveLLMs() (models.LLMs, error)
	GetLLMByID(id uint) (*models.LLM, error)
	GetLLMSettingsByID(id uint) (*models.LLMSettings, error)
}

// DatasourceProvider defines the interface for accessing datasource data
type DatasourceProvider interface {
	GetActiveDatasources() (models.Datasources, error)
	GetDatasourceByID(id uint) (*models.Datasource, error)
}

// CredentialProvider defines the interface for accessing credential data
type CredentialProvider interface {
	GetCredentialBySecret(secret string) (*models.Credential, error)
}

// AppProvider defines the interface for accessing app data
type AppProvider interface {
	GetAppByCredentialID(credID uint) (*models.App, error)
}

// AnalyticsProvider defines the interface for accessing analytics data
type AnalyticsProvider interface {
	GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error)
}

// DatabaseProvider provides access to the database
type DatabaseProvider interface {
	GetDB() *gorm.DB
}

// AuthProvider defines methods for authentication operations
type AuthProvider interface {
	AuthenticateUser(email, password string) (*models.User, error)
	GetUserByAPIKey(apiKey string) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	AddUserToGroup(userID, groupID uint) error
}

// ProxyServiceInterface combines all the service interfaces needed by the proxy
type ProxyServiceInterface interface {
	LLMProvider
	DatasourceProvider
	CredentialProvider
	AppProvider
	AnalyticsProvider
	DatabaseProvider
	AuthProvider
}

// BudgetServiceInterface defines the interface for budget-related operations
type BudgetServiceInterface interface {
	// CheckBudget verifies if a request would exceed either App or LLM budget
	// Returns app usage percentage, llm usage percentage, and error if budget exceeded
	CheckBudget(app *models.App, llm *models.LLM) (float64, float64, error)

	// AnalyzeBudgetUsage analyzes current budget usage and sends notifications if thresholds are reached
	AnalyzeBudgetUsage(app *models.App, llm *models.LLM)

	// ClearCache clears the spending cache
	ClearCache()
}

// AuthServiceInterface defines the interface for authentication-related operations
type AuthServiceInterface interface {
	// GetAuthenticatedUser retrieves a user from various authentication methods
	GetAuthenticatedUser(c *gin.Context) *models.User

	// AuthMiddleware provides middleware for authentication
	AuthMiddleware() gin.HandlerFunc

	// SetUserSession sets a session for a user
	SetUserSession(c *gin.Context, user *models.User) error
}

// ProxyDependencies combines all dependencies needed by the proxy
type ProxyDependencies interface {
	GetService() ProxyServiceInterface
	GetBudgetService() BudgetServiceInterface
	GetAuthService() AuthServiceInterface
}

// StandardProxyDependencies provides a standard implementation of ProxyDependencies
// This can be used for backward compatibility with existing code
type StandardProxyDependencies struct {
	Service       ProxyServiceInterface
	BudgetService BudgetServiceInterface
	AuthService   AuthServiceInterface
}

func (d *StandardProxyDependencies) GetService() ProxyServiceInterface {
	return d.Service
}

func (d *StandardProxyDependencies) GetBudgetService() BudgetServiceInterface {
	return d.BudgetService
}

func (d *StandardProxyDependencies) GetAuthService() AuthServiceInterface {
	return d.AuthService
}
