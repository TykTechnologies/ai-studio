// Package aigateway provides a simple interface to use the Midsommar AI Gateway
// as an importable library in standalone applications.
package aigateway

import (
	"context"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/proxy"
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

// Config represents the configuration for the AI Gateway
type Config struct {
	Port int
}

// New creates a new Gateway instance using interface-based services with default database analytics.
// This approach allows for flexible backend implementations (database, file, API, etc.).
//
// Parameters:
//   - gatewayService: Service interface for configuration, authentication, and pricing
//   - budgetService: Budget interface for spending validation and tracking
//   - config: Gateway configuration including port settings
//
// Example with database backend:
//
//	db := setupDatabase()
//	service := services.NewService(db)
//	budgetService := services.NewBudgetService(db, service)
//
//	gateway := aigateway.New(
//		aigateway.NewDatabaseService(service),
//		aigateway.NewDatabaseBudgetService(budgetService),
//		&aigateway.Config{Port: 9090},
//	)
//	gateway.Start()
//
// Example with custom backend:
//
//	gateway := aigateway.New(
//		myCustomService,        // implements GatewayServiceInterface
//		myCustomBudgetService,  // implements GatewayBudgetServiceInterface
//		&aigateway.Config{Port: 9090},
//	)
func New(
	gatewayService GatewayServiceInterface,
	budgetService GatewayBudgetServiceInterface,
	config *Config,
) Gateway {
	// Use default database analytics handler (assumes analytics.Init was called)
	return NewWithAnalytics(gatewayService, budgetService, nil, config)
}

// NewWithAnalytics creates a new Gateway instance with a custom analytics handler.
// This allows full control over where analytics data is sent (HTTP API, message queue, etc.).
//
// Parameters:
//   - gatewayService: Service interface for configuration, authentication, and pricing
//   - budgetService: Budget interface for spending validation and tracking
//   - analyticsHandler: Analytics handler for recording usage data (nil uses existing global handler)
//   - config: Gateway configuration including port settings
//
// Example with HTTP analytics:
//
//	gateway := aigateway.NewWithAnalytics(
//		aigateway.NewDatabaseService(service),
//		aigateway.NewDatabaseBudgetService(budgetService),
//		aigateway.NewHTTPAnalyticsHandler("https://my-control-plane/api"),
//		&aigateway.Config{Port: 9090},
//	)
func NewWithAnalytics(
	gatewayService GatewayServiceInterface,
	budgetService GatewayBudgetServiceInterface,
	analyticsHandler AnalyticsHandler,
	config *Config,
) Gateway {
	// Set the global analytics handler if provided
	if analyticsHandler != nil {
		analyticsHandler.SetAsGlobalHandler()
	}

	proxyConfig := &proxy.Config{Port: config.Port}
	proxyInstance := proxy.New(gatewayService, budgetService, proxyConfig)
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
