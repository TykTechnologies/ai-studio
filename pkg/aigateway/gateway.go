// Package aigateway provides a simple interface to use the Midsommar AI Gateway
// as an importable library in standalone applications.
package aigateway

import (
	"context"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// Gateway represents an AI Gateway instance that can proxy requests to LLM providers,
// enforce authentication, budgets, and policies, and provide analytics.
type Gateway interface {
	// Start starts the gateway as an HTTP server on the configured port
	Start() error

	// Stop gracefully stops the gateway server
	Stop(ctx context.Context) error

	// Handler returns an http.Handler that can be integrated into existing HTTP servers
	Handler() http.Handler

	// Reload reloads the gateway configuration (LLMs, datasources, filters)
	Reload() error
}

// gateway wraps the existing proxy.Proxy to provide a cleaner API
type gateway struct {
	proxy *proxy.Proxy
}

// New creates a new Gateway instance using the existing services and configuration.
// This is the simplest way to create a gateway that works with the current Midsommar setup.
//
// Parameters:
//   - service: The service layer that provides access to database operations
//   - config: Proxy configuration including port settings
//   - budgetService: Service for budget enforcement and tracking
//
// Example:
//
//	service := services.NewService(db)
//	budgetService := services.NewBudgetService(db, service)
//	gateway := aigateway.New(service, &proxy.Config{Port: 9090}, budgetService)
//	gateway.Start()
func New(service *services.Service, config *proxy.Config, budgetService *services.BudgetService) Gateway {
	proxyInstance := proxy.NewProxy(service, config, budgetService)
	return &gateway{
		proxy: proxyInstance,
	}
}

// Start starts the gateway as an HTTP server
func (g *gateway) Start() error {
	return g.proxy.Start()
}

// Stop gracefully stops the gateway server
func (g *gateway) Stop(ctx context.Context) error {
	return g.proxy.Stop(ctx)
}

// Handler returns the HTTP handler for integration with existing servers
func (g *gateway) Handler() http.Handler {
	return g.proxy.Handler()
}

// Reload reloads the gateway configuration
func (g *gateway) Reload() error {
	return g.proxy.Reload()
}
