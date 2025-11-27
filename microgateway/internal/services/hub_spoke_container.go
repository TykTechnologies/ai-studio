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
	log.Debug().
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

	log.Debug().
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

	// Always use database-backed auth since edge instances now sync to SQLite
	authProvider := auth.NewTokenAuthProvider(db)

	// Initialize management services
	// For edge instances, these should be read-only or use the configuration provider
	var management ManagementServiceInterface
	var filterService FilterServiceInterface  
	var pluginService PluginServiceInterface

	// Always use full database services since edge instances now sync to SQLite
	management = NewManagementService(db, repo, crypto)
	filterService = NewFilterService(db, repo)
	pluginService = NewPluginService(db, repo)
	
	log.Debug().
		Str("provider_type", string(configProvider.GetProviderType())).
		Msg("Using full database services (edge instances now use synced SQLite)")

	tokenService := NewTokenService(authProvider)

	// Create plugin service adapter
	pluginServiceAdapter := NewPluginServiceAdapter(pluginService)

	// Initialize plugin manager with OCI support if configured
	var pluginManager *plugins.PluginManager
	var err error

	if cfg.OCIPlugins.CacheDir != "" {
		// OCI support enabled
		ociConfig := cfg.OCIPlugins.ToOCIConfig()
		pluginManager, err = plugins.NewPluginManagerWithOCI(pluginServiceAdapter, ociConfig)
		if err != nil {
			log.Error().Err(err).Msg("Failed to initialize OCI plugin support, using standard plugin manager")
			pluginManager = plugins.NewPluginManager(pluginServiceAdapter)
		} else {
			log.Debug().
				Str("cache_dir", ociConfig.CacheDir).
				Int("public_keys", len(ociConfig.DefaultPublicKeys)).
				Bool("require_signature", ociConfig.RequireSignature).
				Msg("OCI plugin support enabled in hub-spoke container")
		}
	} else {
		// Standard plugin manager
		pluginManager = plugins.NewPluginManager(pluginServiceAdapter)
		log.Debug().Msg("OCI plugin support disabled in hub-spoke container")
	}
	
	// Load global data collection plugins if configured
	if cfg.Plugins.ConfigPath != "" || cfg.Plugins.ConfigServiceURL != "" {
		log.Debug().Str("config_path", cfg.Plugins.ConfigPath).Msg("Loading global data collection plugins")

		ctx := context.Background()
		if err = cfg.LoadPluginConfig(ctx); err != nil {
			log.Error().Err(err).Msg("Failed to load plugin configuration")
		} else if len(cfg.Plugins.DataCollectionPlugins) > 0 {
			if err := pluginManager.LoadGlobalDataCollectionPlugins(cfg.Plugins.DataCollectionPlugins); err != nil {
				log.Error().Err(err).Msg("Failed to load global data collection plugins")
			} else {
				log.Debug().Int("count", len(cfg.Plugins.DataCollectionPlugins)).Msg("Global data collection plugins loaded")
			}
		}
	}

	// Pre-warm OCI plugins during startup
	if err = pluginManager.PreWarmOCIPlugins(context.Background()); err != nil {
		log.Error().Err(err).Msg("Failed to pre-warm OCI plugins during startup")
		// Don't fail startup, but log the error for investigation
	}

	// Initialize core services with provider support
	var gatewayService GatewayServiceInterface
	
	if configProvider.GetProviderType() == providers.ProviderTypeDatabase {
		// Control/standalone: use database provider directly
		gatewayService = NewDatabaseGatewayService(db, repo)
	} else {
		// Edge: use HybridGatewayService - DatabaseGatewayService + on-demand token validation
		gatewayService = NewHybridGatewayService(db, repo, cfg.HubSpoke.EdgeNamespace, cfg.HubSpoke)
		log.Debug().Msg("Edge instance using HybridGatewayService with synced SQLite + on-demand token validation")
	}
	
	// Always use full database services since edge instances now sync to SQLite
	budgetService := NewDatabaseBudgetService(db, repo, pluginManager)
	analyticsService := NewDatabaseAnalyticsService(db, repo, cfg.Analytics)

	// Create microgateway management server for plugin service broker
	managementServer := NewMicrogatewayManagementServer(
		db,
		gatewayService,
		budgetService,
		management,
		crypto,
	)

	// Connect management server to plugin manager for bidirectional plugin communication
	pluginManager.SetManagementServer(managementServer)
	log.Debug().Msg("Management server connected to plugin manager")

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
	log.Debug().Msg("Initializing control instance components")

	// Control server will be created and managed at the main application level
	log.Debug().
		Int("grpc_port", cfg.HubSpoke.GRPCPort).
		Msg("Control instance components initialized")

	return nil
}

// initializeEdgeComponents initializes edge instance specific components
func (h *HubSpokeServiceContainer) initializeEdgeComponents(cfg *config.Config) error {
	log.Debug().Msg("Initializing edge instance components")

	// Edge client will be managed at the main application level
	if _, ok := h.ConfigProvider.(*providers.GRPCProvider); ok {
		log.Debug().Msg("Edge client will be managed at main application level")
	}

	log.Debug().
		Str("control_endpoint", cfg.HubSpoke.ControlEndpoint).
		Str("edge_namespace", cfg.HubSpoke.EdgeNamespace).
		Msg("Edge instance components initialized")

	return nil
}

// StartHubSpokeServices starts hub-and-spoke specific services
func (h *HubSpokeServiceContainer) StartHubSpokeServices(ctx context.Context) error {
	// Hub-and-spoke services are managed at the main application level
	// to avoid circular import dependencies
	log.Debug().Msg("Hub-spoke service container ready")
	return nil
}

// StopHubSpokeServices stops hub-and-spoke specific services
func (h *HubSpokeServiceContainer) StopHubSpokeServices() {
	// Hub-and-spoke services are managed at the main application level
	log.Debug().Msg("Hub-spoke service container stopped")
}

// GetHubSpokeStats returns hub-and-spoke specific statistics
func (h *HubSpokeServiceContainer) GetHubSpokeStats() map[string]interface{} {
	stats := make(map[string]interface{})

	stats["gateway_mode"] = h.ConfigProvider.GetNamespace()
	stats["provider_type"] = string(h.ConfigProvider.GetProviderType())
	stats["provider_healthy"] = h.ConfigProvider.IsHealthy()

	return stats
}