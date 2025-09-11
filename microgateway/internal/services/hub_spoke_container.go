// internal/services/hub_spoke_container.go
package services

import (
	"context"
	"fmt"

	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/providers"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// HubSpokeServiceContainer extends ServiceContainer with hub-and-spoke capabilities
type HubSpokeServiceContainer struct {
	*ServiceContainer
	
	// Hub-and-Spoke components
	ConfigProvider   providers.ConfigurationProvider
	ProviderFactory  *providers.ProviderFactory
	// Note: ControlServer and EdgeClient are managed at the main application level
	// to avoid circular import dependencies
}

// NewHubSpokeServiceContainer creates a service container with hub-and-spoke support
func NewHubSpokeServiceContainer(db *gorm.DB, cfg *config.Config) (*HubSpokeServiceContainer, error) {
	log.Info().
		Str("gateway_mode", cfg.HubSpoke.Mode).
		Msg("Creating hub-spoke service container")

	// Create provider factory
	providerFactory := providers.NewProviderFactory(cfg, db)
	
	// Create configuration provider based on mode
	configProvider, err := providerFactory.CreateProvider()
	if err != nil {
		return nil, fmt.Errorf("failed to create configuration provider: %w", err)
	}

	// Initialize base services first
	baseContainer, err := createBaseServiceContainer(db, cfg, configProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create base service container: %w", err)
	}

	hubSpokeContainer := &HubSpokeServiceContainer{
		ServiceContainer: baseContainer,
		ConfigProvider:   configProvider,
		ProviderFactory:  providerFactory,
	}

	// Initialize mode-specific components
	if err := hubSpokeContainer.initializeModeSpecificComponents(cfg); err != nil {
		return nil, fmt.Errorf("failed to initialize mode-specific components: %w", err)
	}

	log.Info().
		Str("gateway_mode", cfg.HubSpoke.Mode).
		Str("provider_type", string(configProvider.GetProviderType())).
		Msg("Hub-spoke service container created successfully")

	return hubSpokeContainer, nil
}

// createBaseServiceContainer creates the base service container with provider-aware services
func createBaseServiceContainer(db *gorm.DB, cfg *config.Config, configProvider providers.ConfigurationProvider) (*ServiceContainer, error) {
	// Initialize repository
	repo := database.NewRepository(db)

	// Initialize crypto service
	crypto := NewCryptoService(cfg.Security.EncryptionKey)

	// Initialize auth provider
	// For edge instances, we might want to use a provider-aware auth provider
	// For now, keep the existing database-backed auth
	var authProvider auth.AuthProvider
	if configProvider.GetProviderType() == providers.ProviderTypeDatabase {
		authProvider = auth.NewTokenAuthProvider(db)
	} else {
		// For gRPC providers, create a provider-aware auth provider
		authProvider = NewProviderAwareAuthProvider(configProvider)
	}

	// Initialize management services
	// For edge instances, these should be read-only or use the configuration provider
	var management ManagementServiceInterface
	var filterService FilterServiceInterface  
	var pluginService PluginServiceInterface

	if configProvider.GetProviderType() == providers.ProviderTypeDatabase {
		// Full management services for control/standalone
		management = NewManagementService(db, repo, crypto)
		filterService = NewFilterService(db, repo)
		pluginService = NewPluginService(db, repo)
	} else {
		// Read-only services for edge instances
		management = NewProviderAwareManagementService(configProvider, crypto)
		filterService = NewProviderAwareFilterService(configProvider)
		pluginService = NewProviderAwarePluginService(configProvider)
	}

	tokenService := NewTokenService(authProvider)

	// Create plugin service adapter
	pluginServiceAdapter := NewPluginServiceAdapter(pluginService)
	
	// Initialize plugin manager
	pluginManager := plugins.NewPluginManager(pluginServiceAdapter)
	
	// Load global data collection plugins if configured
	if cfg.Plugins.ConfigPath != "" || cfg.Plugins.ConfigServiceURL != "" {
		log.Info().Str("config_path", cfg.Plugins.ConfigPath).Msg("Loading global data collection plugins...")
		
		ctx := context.Background()
		if err := cfg.LoadPluginConfig(ctx); err != nil {
			log.Error().Err(err).Msg("Failed to load plugin configuration")
		} else if len(cfg.Plugins.DataCollectionPlugins) > 0 {
			if err := pluginManager.LoadGlobalDataCollectionPlugins(cfg.Plugins.DataCollectionPlugins); err != nil {
				log.Error().Err(err).Msg("Failed to load global data collection plugins")
			} else {
				log.Info().Int("count", len(cfg.Plugins.DataCollectionPlugins)).Msg("Global data collection plugins loaded")
			}
		}
	}

	// Initialize core services with provider support
	gatewayService := NewHubSpokeGatewayService(configProvider)
	
	var budgetService BudgetServiceInterface
	var analyticsService AnalyticsServiceInterface
	
	if configProvider.GetProviderType() == providers.ProviderTypeDatabase {
		// Full services for control/standalone
		budgetService = NewDatabaseBudgetService(db, repo, pluginManager)
		analyticsService = NewDatabaseAnalyticsService(db, repo, cfg.Analytics)
	} else {
		// Simplified services for edge instances
		budgetService = NewProviderAwareBudgetService(configProvider)
		analyticsService = NewProviderAwareAnalyticsService(configProvider, cfg.Analytics)
	}

	return &ServiceContainer{
		DB:         db,
		Repository: repo,

		GatewayService:   gatewayService,
		BudgetService:    budgetService,
		AnalyticsService: analyticsService,

		Management:     management,
		Token:          tokenService,
		FilterService:  filterService,
		PluginService:  pluginService,

		AuthProvider: authProvider,
		Crypto:       crypto,
		
		PluginManager: pluginManager,
	}, nil
}

// initializeModeSpecificComponents initializes components specific to the gateway mode
func (h *HubSpokeServiceContainer) initializeModeSpecificComponents(cfg *config.Config) error {
	switch cfg.HubSpoke.Mode {
	case "control":
		return h.initializeControlComponents(cfg)
	case "edge":
		return h.initializeEdgeComponents(cfg)
	case "standalone":
		// No additional components needed for standalone mode
		return nil
	default:
		return fmt.Errorf("unknown gateway mode: %s", cfg.HubSpoke.Mode)
	}
}

// initializeControlComponents initializes control instance specific components
func (h *HubSpokeServiceContainer) initializeControlComponents(cfg *config.Config) error {
	log.Info().Msg("Initializing control instance components")

	// Control server will be created and managed at the main application level
	log.Info().
		Int("grpc_port", cfg.HubSpoke.GRPCPort).
		Msg("Control instance components initialized")

	return nil
}

// initializeEdgeComponents initializes edge instance specific components  
func (h *HubSpokeServiceContainer) initializeEdgeComponents(cfg *config.Config) error {
	log.Info().Msg("Initializing edge instance components")

	// Edge client will be managed at the main application level
	if _, ok := h.ConfigProvider.(*providers.GRPCProvider); ok {
		log.Info().Msg("Edge client will be managed at main application level")
	}

	log.Info().
		Str("control_endpoint", cfg.HubSpoke.ControlEndpoint).
		Str("edge_namespace", cfg.HubSpoke.EdgeNamespace).
		Msg("Edge instance components initialized")

	return nil
}

// StartHubSpokeServices starts hub-and-spoke specific services
func (h *HubSpokeServiceContainer) StartHubSpokeServices(ctx context.Context) error {
	// Hub-and-spoke services are managed at the main application level
	// to avoid circular import dependencies
	log.Info().Msg("Hub-spoke service container ready")
	return nil
}

// StopHubSpokeServices stops hub-and-spoke specific services
func (h *HubSpokeServiceContainer) StopHubSpokeServices() {
	// Hub-and-spoke services are managed at the main application level
	log.Info().Msg("Hub-spoke service container stopped")
}

// GetHubSpokeStats returns hub-and-spoke specific statistics
func (h *HubSpokeServiceContainer) GetHubSpokeStats() map[string]interface{} {
	stats := make(map[string]interface{})

	stats["gateway_mode"] = h.ConfigProvider.GetNamespace()
	stats["provider_type"] = string(h.ConfigProvider.GetProviderType())
	stats["provider_healthy"] = h.ConfigProvider.IsHealthy()

	return stats
}