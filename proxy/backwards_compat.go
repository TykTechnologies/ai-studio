package proxy

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// NewProxyLegacy is a backwards-compatible constructor for the Proxy
// This is provided for compatibility with existing code
func NewProxyLegacy(service *services.Service, config *Config, budgetService *services.BudgetService) *Proxy {
	// Create adaptors - using nil for authService since it's not used in tests
	serviceAdaptor := NewServiceAdaptor(service)
	budgetAdaptor := NewBudgetServiceAdaptor(budgetService)

	// Create a mock auth service adaptor
	mockAuthService := &MockAuthServiceAdaptor{}

	// Create the proxy with the adaptors
	return NewProxy(serviceAdaptor, config, budgetAdaptor, mockAuthService)
}

// MockAuthServiceAdaptor is a mock implementation of AuthServiceInterface for backwards compatibility
type MockAuthServiceAdaptor struct{}

func (m *MockAuthServiceAdaptor) GetAuthenticatedUser(c *gin.Context) *models.User {
	return nil
}

func (m *MockAuthServiceAdaptor) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func (m *MockAuthServiceAdaptor) SetUserSession(c *gin.Context, user *models.User) error {
	return nil
}
