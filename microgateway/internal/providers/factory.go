// internal/providers/factory.go
package providers

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// ProviderFactory creates configuration providers based on gateway mode
type ProviderFactory struct {
	config *config.Config
	db     *gorm.DB
}

// NewProviderFactory creates a new provider factory
func NewProviderFactory(cfg *config.Config, db *gorm.DB) *ProviderFactory {
	return &ProviderFactory{
		config: cfg,
		db:     db,
	}
}

// CreateProvider creates the appropriate configuration provider based on gateway mode
func (f *ProviderFactory) CreateProvider() (ConfigurationProvider, error) {
	switch f.config.HubSpoke.Mode {
	case "standalone", "control":
		// Use database provider for standalone and control modes
		return f.createDatabaseProvider()
		
	case "edge":
		// Use gRPC provider for edge mode
		return f.createGRPCProvider()
		
	default:
		return nil, fmt.Errorf("unsupported gateway mode: %s", f.config.HubSpoke.Mode)
	}
}

// createDatabaseProvider creates a database-backed configuration provider
func (f *ProviderFactory) createDatabaseProvider() (ConfigurationProvider, error) {
	log.Info().
		Str("mode", f.config.HubSpoke.Mode).
		Msg("Creating database configuration provider")
	
	// For control/standalone mode, namespace is empty (sees all configurations)
	namespace := ""
	
	provider := NewDatabaseProvider(f.db, namespace)
	
	if !provider.IsHealthy() {
		return nil, fmt.Errorf("database provider health check failed")
	}
	
	return provider, nil
}

// createGRPCProvider creates a gRPC-backed configuration provider for edge instances
func (f *ProviderFactory) createGRPCProvider() (ConfigurationProvider, error) {
	log.Info().
		Str("control_endpoint", f.config.HubSpoke.ControlEndpoint).
		Str("edge_namespace", f.config.HubSpoke.EdgeNamespace).
		Msg("Creating gRPC configuration provider")
	
	// Create the real gRPC provider
	provider := NewGRPCProvider(f.config.HubSpoke.EdgeNamespace)
	
	log.Info().Msg("gRPC configuration provider created - will be connected to edge client")
	
	return provider, nil
}