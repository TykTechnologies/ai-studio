package aigateway

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// ConfigProvider is an interface that allows custom configuration sources
// for the AI Gateway. This enables users to provide LLM configurations,
// datasources, and filters from various sources (database, files, APIs, etc.)
type ConfigProvider interface {
	// GetLLMs returns the list of active LLM configurations
	GetLLMs(ctx context.Context) ([]models.LLM, error)

	// GetDatasources returns the list of active datasource configurations
	GetDatasources(ctx context.Context) ([]models.Datasource, error)

	// GetFilters returns the list of filters to be applied
	GetFilters(ctx context.Context) ([]models.Filter, error)

	// Watch returns a channel that signals when configuration changes occur.
	// This enables hot reloading of the gateway configuration.
	Watch() <-chan struct{}
}

// ServiceProvider is an interface that provides the core service operations
// needed by the gateway. This allows users to provide custom implementations
// or wrap existing services.
type ServiceProvider interface {
	// GetActiveLLMs returns the list of active LLM configurations
	GetActiveLLMs() ([]models.LLM, error)

	// GetActiveDatasources returns the list of active datasource configurations
	GetActiveDatasources() ([]models.Datasource, error)

	// CallToolOperation executes a tool operation
	CallToolOperation(toolID uint, operationID string, params map[string][]string, payload map[string]interface{}, headers map[string][]string) (interface{}, error)

	// GetToolBySlug retrieves a tool by its slug identifier
	GetToolBySlug(slug string) (*models.Tool, error)

	// Additional service methods can be added here as needed
}

// Note: NewWithConfigProvider will be implemented in a future version
// Currently, the proxy requires a concrete services.Service implementation
// which makes it challenging to provide a clean abstraction without significant changes.
// For now, users should use the New() function with existing services.

// Future enhancement: We could extend this by either:
// 1. Creating a service interface and updating the proxy to accept interfaces
// 2. Or providing configuration injection methods on the existing gateway
