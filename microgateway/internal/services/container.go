// internal/services/container.go
package services

import (
	"context"
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
	
	// Initialize plugin manager
	pluginManager := plugins.NewPluginManager(pluginServiceAdapter)
	
	// Load global data collection plugins if configured
	if cfg.Plugins.ConfigPath != "" || cfg.Plugins.ConfigServiceURL != "" {
		log.Info().Str("config_path", cfg.Plugins.ConfigPath).Msg("Loading global data collection plugins in service container...")
		
		// Load plugin configuration
		ctx := context.Background()
		if err := cfg.LoadPluginConfig(ctx); err != nil {
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

// Health checks all service health
func (sc *ServiceContainer) Health() error {
	// Check database health
	if err := database.IsHealthy(sc.DB); err != nil {
		return err
	}

	// All other services are healthy if database is healthy
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