// internal/providers/interfaces.go
package providers

import (
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
)

// ConfigurationProvider defines the interface for providing configuration data
// This abstraction allows switching between database-backed (control/standalone) 
// and gRPC-backed (edge) configuration sources
type ConfigurationProvider interface {
	// LLM operations
	GetLLM(id uint) (*database.LLM, error)
	GetLLMBySlug(slug string) (*database.LLM, error)
	ListLLMs(namespace string, active bool) ([]database.LLM, error)
	
	// App operations
	GetApp(id uint) (*database.App, error)
	ListApps(namespace string, active bool) ([]database.App, error)
	
	// Token operations
	GetToken(token string) (*database.APIToken, error)
	ValidateToken(token string) (*database.APIToken, error)
	
	// Model Price operations
	GetModelPrice(vendor, model string) (*database.ModelPrice, error)
	ListModelPrices(namespace string) ([]database.ModelPrice, error)
	
	// Filter operations
	GetFilter(id uint) (*database.Filter, error)
	GetFiltersForLLM(llmID uint) ([]database.Filter, error)
	ListFilters(namespace string, active bool) ([]database.Filter, error)
	
	// Plugin operations
	GetPlugin(id uint) (*database.Plugin, error)
	GetPluginsForLLM(llmID uint) ([]database.Plugin, error)
	GetAllLLMAssociatedPlugins() ([]database.Plugin, error)
	ListPlugins(namespace string, hookType string, active bool) ([]database.Plugin, error)
	
	// Provider metadata
	GetProviderType() ProviderType
	IsHealthy() bool
	GetNamespace() string
}

// ProviderType represents the type of configuration provider
type ProviderType string

const (
	ProviderTypeDatabase ProviderType = "database"
	ProviderTypeGRPC     ProviderType = "grpc"
)

// NamespaceFilter defines common namespace filtering behavior
type NamespaceFilter interface {
	MatchesNamespace(objectNamespace, requestNamespace string) bool
}

// DefaultNamespaceFilter implements standard namespace matching logic
type DefaultNamespaceFilter struct{}

// MatchesNamespace returns true if an object should be visible to a request
// Rules:
// - Empty object namespace ("") means global - visible to all
// - Empty request namespace ("") only sees global objects  
// - Specific request namespace sees global objects + objects with matching namespace
func (f *DefaultNamespaceFilter) MatchesNamespace(objectNamespace, requestNamespace string) bool {
	// Global objects (empty namespace) are visible to all
	if objectNamespace == "" {
		return true
	}
	
	// If request has no namespace, only global objects are visible
	if requestNamespace == "" {
		return objectNamespace == ""
	}
	
	// Match specific namespace
	return objectNamespace == requestNamespace
}