package proxy

import (
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ServiceAdaptor adapts the concrete Service to the ProxyServiceInterface
type ServiceAdaptor struct {
	service *services.Service
}

// NewServiceAdaptor creates a new ServiceAdaptor
func NewServiceAdaptor(service *services.Service) *ServiceAdaptor {
	return &ServiceAdaptor{service: service}
}

// Implement LLMProvider
func (a *ServiceAdaptor) GetActiveLLMs() (models.LLMs, error) {
	return a.service.GetActiveLLMs()
}

func (a *ServiceAdaptor) GetLLMByID(id uint) (*models.LLM, error) {
	return a.service.GetLLMByID(id)
}

func (a *ServiceAdaptor) GetLLMSettingsByID(id uint) (*models.LLMSettings, error) {
	return a.service.GetLLMSettingsByID(id)
}

// Implement DatasourceProvider
func (a *ServiceAdaptor) GetActiveDatasources() (models.Datasources, error) {
	return a.service.GetActiveDatasources()
}

func (a *ServiceAdaptor) GetDatasourceByID(id uint) (*models.Datasource, error) {
	return a.service.GetDatasourceByID(id)
}

// Implement CredentialProvider
func (a *ServiceAdaptor) GetCredentialBySecret(secret string) (*models.Credential, error) {
	return a.service.GetCredentialBySecret(secret)
}

// Implement AppProvider
func (a *ServiceAdaptor) GetAppByCredentialID(credID uint) (*models.App, error) {
	return a.service.GetAppByCredentialID(credID)
}

// Implement AnalyticsProvider
func (a *ServiceAdaptor) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	return a.service.GetModelPriceByModelNameAndVendor(modelName, vendor)
}

// Implement DatabaseProvider
func (a *ServiceAdaptor) GetDB() *gorm.DB {
	return a.service.GetDB()
}

// Implement AuthProvider
func (a *ServiceAdaptor) AuthenticateUser(email, password string) (*models.User, error) {
	return a.service.AuthenticateUser(email, password)
}

func (a *ServiceAdaptor) GetUserByAPIKey(apiKey string) (*models.User, error) {
	return a.service.GetUserByAPIKey(apiKey)
}

func (a *ServiceAdaptor) GetUserByEmail(email string) (*models.User, error) {
	return a.service.GetUserByEmail(email)
}

func (a *ServiceAdaptor) AddUserToGroup(userID, groupID uint) error {
	return a.service.AddUserToGroup(userID, groupID)
}

// BudgetServiceAdaptor adapts the concrete BudgetService to the BudgetServiceInterface
type BudgetServiceAdaptor struct {
	budgetService *services.BudgetService
}

// NewBudgetServiceAdaptor creates a new BudgetServiceAdaptor
func NewBudgetServiceAdaptor(budgetService *services.BudgetService) *BudgetServiceAdaptor {
	return &BudgetServiceAdaptor{budgetService: budgetService}
}

// Implement BudgetServiceInterface
func (a *BudgetServiceAdaptor) CheckBudget(app *models.App, llm *models.LLM) (float64, float64, error) {
	return a.budgetService.CheckBudget(app, llm)
}

func (a *BudgetServiceAdaptor) AnalyzeBudgetUsage(app *models.App, llm *models.LLM) {
	a.budgetService.AnalyzeBudgetUsage(app, llm)
}

func (a *BudgetServiceAdaptor) ClearCache() {
	a.budgetService.ClearCache()
}

// AuthServiceAdaptor adapts the concrete AuthService to the AuthServiceInterface
type AuthServiceAdaptor struct {
	authService *auth.AuthService
}

// NewAuthServiceAdaptor creates a new AuthServiceAdaptor
func NewAuthServiceAdaptor(authService *auth.AuthService) *AuthServiceAdaptor {
	return &AuthServiceAdaptor{authService: authService}
}

// Implement AuthServiceInterface
func (a *AuthServiceAdaptor) GetAuthenticatedUser(c *gin.Context) *models.User {
	return a.authService.GetAuthenticatedUser(c)
}

func (a *AuthServiceAdaptor) AuthMiddleware() gin.HandlerFunc {
	return a.authService.AuthMiddleware()
}

func (a *AuthServiceAdaptor) SetUserSession(c *gin.Context, user *models.User) error {
	return a.authService.SetUserSession(c, user)
}

// NewStandardProxyDependencies creates a new StandardProxyDependencies with all adaptors set up
func NewStandardProxyDependencies(service *services.Service, budgetService *services.BudgetService, authService *auth.AuthService) *StandardProxyDependencies {
	return &StandardProxyDependencies{
		Service:       NewServiceAdaptor(service),
		BudgetService: NewBudgetServiceAdaptor(budgetService),
		AuthService:   NewAuthServiceAdaptor(authService),
	}
}
