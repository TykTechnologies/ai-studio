// internal/services/container.go
package services

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// ServiceContainer holds all application services
type ServiceContainer struct {
	// Database
	DB         *gorm.DB
	Repository *database.Repository

	// Core services
	GatewayService   GatewayServiceInterface
	BudgetService    BudgetServiceInterface
	AnalyticsService AnalyticsServiceInterface

	// Management services
	Management     ManagementServiceInterface
	Token          TokenServiceInterface
	FilterService  FilterServiceInterface
	PluginService  PluginServiceInterface

	// Authentication (simplified)
	AuthProvider auth.AuthProvider

	// Utilities
	Crypto CryptoServiceInterface
	
	// Plugin management
	PluginManager *plugins.PluginManager
}

// NewServiceContainer creates a new service container with essential dependencies only
func NewServiceContainer(db *gorm.DB, cfg *config.Config) (*ServiceContainer, error) {
	// Initialize repository
	repo := database.NewRepository(db)

	// Initialize crypto service
	crypto := NewCryptoService(cfg.Security.EncryptionKey)

	// Initialize auth provider (no caching)
	authProvider := auth.NewTokenAuthProvider(db)

	// Initialize management services first (needed for plugin manager)
	management := NewManagementService(db, repo, crypto)
	tokenService := NewTokenService(authProvider)
	filterService := NewFilterService(db, repo)
	pluginService := NewPluginService(db, repo)

	// Create plugin service adapter to break circular dependency
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
			log.Info().
				Str("cache_dir", ociConfig.CacheDir).
				Int("public_keys", len(ociConfig.DefaultPublicKeys)).
				Bool("require_signature", ociConfig.RequireSignature).
				Msg("OCI plugin support enabled")
		}
	} else {
		// Standard plugin manager
		pluginManager = plugins.NewPluginManager(pluginServiceAdapter)
		log.Info().Msg("OCI plugin support disabled - using standard plugin manager")
	}
	
	// Load global data collection plugins if configured
	if cfg.Plugins.ConfigPath != "" || cfg.Plugins.ConfigServiceURL != "" {
		log.Info().Str("config_path", cfg.Plugins.ConfigPath).Msg("Loading global data collection plugins in service container...")
		
		// Load plugin configuration
		ctx := context.Background()
		if err = cfg.LoadPluginConfig(ctx); err != nil {
			log.Error().Err(err).Msg("Failed to load plugin configuration")
		} else {
			log.Info().Int("count", len(cfg.Plugins.DataCollectionPlugins)).Msg("Plugin configurations loaded in service container")
			
			if len(cfg.Plugins.DataCollectionPlugins) > 0 {
				// Load global plugins
				if err := pluginManager.LoadGlobalDataCollectionPlugins(cfg.Plugins.DataCollectionPlugins); err != nil {
					log.Error().Err(err).Msg("Failed to load global data collection plugins")
				} else {
					log.Info().Int("count", len(cfg.Plugins.DataCollectionPlugins)).Msg("Global data collection plugins loaded in service container")
				}
			} else {
				log.Info().Msg("No data collection plugins configured")
			}
		}
	} else {
		log.Info().Msg("No plugin configuration specified - skipping data collection plugins")
	}

	// Pre-warm OCI plugins during startup
	if err = pluginManager.PreWarmOCIPlugins(context.Background()); err != nil {
		log.Error().Err(err).Msg("Failed to pre-warm OCI plugins during startup")
		// Don't fail startup, but log the error for investigation
	}

	// Initialize core services with plugin manager support
	gatewayService := NewDatabaseGatewayService(db, repo)
	budgetService := NewDatabaseBudgetService(db, repo, pluginManager)
	analyticsService := NewDatabaseAnalyticsService(db, repo, cfg.Analytics)

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

// StartBackgroundTasks starts minimal essential tasks only
func (sc *ServiceContainer) StartBackgroundTasks(ctx context.Context) {
	log.Info().Msg("Starting essential background tasks")
	
	// Only start token cleanup (essential for security)
	if tokenAuthProvider, ok := sc.AuthProvider.(*auth.TokenAuthProvider); ok {
		go func() {
			sc.startTokenCleanup(ctx, tokenAuthProvider)
		}()
	}

	log.Info().Msg("Essential background tasks started")
}

// StopBackgroundTasks stops background tasks gracefully  
func (sc *ServiceContainer) StopBackgroundTasks() {
	log.Info().Msg("Stopping background tasks")
	// Token cleanup will stop when context is cancelled
}

// Cleanup performs final cleanup of all services
func (sc *ServiceContainer) Cleanup() {
	log.Info().Msg("Starting service container cleanup")

	// Simple cleanup - no complex operations needed
	log.Info().Msg("Service container cleanup completed")
}

// Health checks all service health including plugins
func (sc *ServiceContainer) Health() error {
	// Check database health
	if err := database.IsHealthy(sc.DB); err != nil {
		return fmt.Errorf("database unhealthy: %w", err)
	}

	// Check plugin health
	if sc.PluginManager != nil {
		if !sc.PluginManager.IsAllPluginsReady() {
			healthSummary := sc.PluginManager.GetPluginHealthSummary()
			if failedCount, ok := healthSummary["failed_plugins"].(int); ok && failedCount > 0 {
				return fmt.Errorf("plugin health check failed: %d plugins failed", failedCount)
			}
			if loadingCount, ok := healthSummary["loading_plugins"].(int); ok && loadingCount > 0 {
				return fmt.Errorf("plugin health check failed: %d plugins still loading", loadingCount)
			}
		}

		// Check OCI plugin system health if enabled
		if ociClient := sc.PluginManager.GetOCIClient(); ociClient != nil {
			ociStats := sc.PluginManager.GetOCIStats()
			if enabled, ok := ociStats["enabled"].(bool); ok && enabled {
				// Add OCI-specific health checks here if needed
				log.Debug().Interface("oci_stats", ociStats).Msg("OCI plugin system health check")
			}
		}
	}

	return nil
}

// GetStats returns basic statistics about services
func (sc *ServiceContainer) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Auth provider stats
	if tokenAuthProvider, ok := sc.AuthProvider.(*auth.TokenAuthProvider); ok {
		if tokenStats, err := tokenAuthProvider.GetStats(); err == nil {
			stats["tokens"] = tokenStats
		}
	}

	// Analytics stats
	if analyticsService, ok := sc.AnalyticsService.(*DatabaseAnalyticsService); ok {
		stats["analytics"] = analyticsService.GetStats()
	}

	return stats
}

// startTokenCleanup runs periodic cleanup of expired tokens
func (sc *ServiceContainer) startTokenCleanup(ctx context.Context, provider *auth.TokenAuthProvider) {
	ticker := time.NewTicker(1 * time.Hour) // Cleanup every hour
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := provider.CleanupExpiredTokens(); err != nil {
				log.Error().Err(err).Msg("Failed to cleanup expired tokens")
			} else {
				log.Debug().Msg("Cleaned up expired tokens")
			}
		}
	}
}