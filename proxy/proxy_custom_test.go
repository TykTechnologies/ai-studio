package proxy

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// This test demonstrates how to use the proxy with custom implementations
func TestCustomProxyImplementation(t *testing.T) {
	// Create custom implementations
	customService := &TestCustomService{}
	customBudget := &TestCustomBudgetService{}
	customAuth := &TestCustomAuthService{}

	// Create dependencies container
	deps := &TestCustomDependencies{
		service:       customService,
		budgetService: customBudget,
		authService:   customAuth,
	}

	// Create the proxy config
	config := &Config{
		Port: 9999, // Use a high port number to avoid conflicts
	}

	// Create the proxy with custom implementations
	proxy := NewEmbeddedProxy(deps, config)

	// Validate the proxy was created correctly
	if proxy == nil {
		t.Fatal("Failed to create proxy with custom implementations")
	}

	// The proxy is ready to be started
	// We don't actually start it in this test to avoid binding to ports
}

// Custom implementations for testing

type TestCustomDependencies struct {
	service       *TestCustomService
	budgetService *TestCustomBudgetService
	authService   *TestCustomAuthService
}

func (d *TestCustomDependencies) GetService() ProxyServiceInterface {
	return d.service
}

func (d *TestCustomDependencies) GetBudgetService() BudgetServiceInterface {
	return d.budgetService
}

func (d *TestCustomDependencies) GetAuthService() AuthServiceInterface {
	return d.authService
}

// Custom service implementation
type TestCustomService struct{}

// Implement LLMProvider interface
func (s *TestCustomService) GetActiveLLMs() (models.LLMs, error) {
	return models.LLMs{}, nil
}

func (s *TestCustomService) GetLLMByID(id uint) (*models.LLM, error) {
	return nil, nil
}

func (s *TestCustomService) GetLLMSettingsByID(id uint) (*models.LLMSettings, error) {
	return nil, nil
}

// Implement DatasourceProvider interface
func (s *TestCustomService) GetActiveDatasources() (models.Datasources, error) {
	return models.Datasources{}, nil
}

func (s *TestCustomService) GetDatasourceByID(id uint) (*models.Datasource, error) {
	return nil, nil
}

// Implement CredentialProvider interface
func (s *TestCustomService) GetCredentialBySecret(secret string) (*models.Credential, error) {
	return nil, nil
}

// Implement AppProvider interface
func (s *TestCustomService) GetAppByCredentialID(credID uint) (*models.App, error) {
	return nil, nil
}

// Implement AnalyticsProvider interface
func (s *TestCustomService) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	return nil, nil
}

// Implement DatabaseProvider interface
func (s *TestCustomService) GetDB() *gorm.DB {
	return nil
}

// Implement AuthProvider interface
func (s *TestCustomService) AuthenticateUser(email, password string) (*models.User, error) {
	return nil, nil
}

func (s *TestCustomService) GetUserByAPIKey(apiKey string) (*models.User, error) {
	return nil, nil
}

func (s *TestCustomService) GetUserByEmail(email string) (*models.User, error) {
	return nil, nil
}

func (s *TestCustomService) AddUserToGroup(userID, groupID uint) error {
	return nil
}

// Custom budget service implementation
type TestCustomBudgetService struct{}

func (b *TestCustomBudgetService) CheckBudget(app *models.App, llm *models.LLM) (float64, float64, error) {
	return 0, 0, nil
}

func (b *TestCustomBudgetService) AnalyzeBudgetUsage(app *models.App, llm *models.LLM) {
	// No-op for testing
}

func (b *TestCustomBudgetService) ClearCache() {
	// No-op for testing
}

// Custom auth service implementation
type TestCustomAuthService struct{}

func (a *TestCustomAuthService) GetAuthenticatedUser(c *gin.Context) *models.User {
	return nil
}

func (a *TestCustomAuthService) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func (a *TestCustomAuthService) SetUserSession(c *gin.Context, user *models.User) error {
	return nil
}
