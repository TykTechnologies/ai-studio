// internal/services/container.go
package services

import (
	"context"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
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
	FilterService    FilterServiceInterface

	// Management services
	Management ManagementServiceInterface
	Token      TokenServiceInterface

	// Authentication
	AuthProvider auth.AuthProvider
	Cache        *auth.TokenCache

	// Utilities
	Crypto CryptoServiceInterface

	// Background tasks
	backgroundCancel context.CancelFunc
	backgroundWg     sync.WaitGroup
}

// NewServiceContainer creates a new service container with all dependencies
func NewServiceContainer(db *gorm.DB, cfg *config.Config) (*ServiceContainer, error) {
	// Initialize repository
	repo := database.NewRepository(db)

	// Initialize cache
	var cache *auth.TokenCache
	if cfg.Cache.Enabled {
		cache = auth.NewTokenCache(cfg.Cache.MaxSize, cfg.Cache.TTL)
	} else {
		cache = auth.NewTokenCache(0, 0) // Disabled cache
	}

	// Initialize crypto service
	crypto := NewCryptoService(cfg.Security.EncryptionKey)

	// Initialize auth provider
	authProvider := auth.NewTokenAuthProvider(db, cache)

	// Initialize core services
	gatewayService := NewDatabaseGatewayService(db, repo, cache)
	budgetService := NewDatabaseBudgetService(db, repo)
	analyticsService := NewDatabaseAnalyticsService(db, repo, cfg.Analytics)
	filterService := NewFilterService(db, repo)

	// Initialize management services
	management := NewManagementService(db, repo, crypto)
	tokenService := NewTokenService(authProvider)

	return &ServiceContainer{
		DB:         db,
		Repository: repo,

		GatewayService:   gatewayService,
		BudgetService:    budgetService,
		AnalyticsService: analyticsService,
		FilterService:    filterService,

		Management: management,
		Token:      tokenService,

		AuthProvider: authProvider,
		Cache:        cache,

		Crypto: crypto,
	}, nil
}

// StartBackgroundTasks starts all background tasks
func (sc *ServiceContainer) StartBackgroundTasks(ctx context.Context) {
	backgroundCtx, cancel := context.WithCancel(ctx)
	sc.backgroundCancel = cancel

	log.Info().Msg("Starting background tasks")

	// Start analytics buffer flush task
	if analyticsService, ok := sc.AnalyticsService.(*DatabaseAnalyticsService); ok {
		sc.backgroundWg.Add(1)
		go func() {
			defer sc.backgroundWg.Done()
			analyticsService.StartBufferFlush(backgroundCtx)
		}()
	}

	// Start budget monitoring task
	if budgetService, ok := sc.BudgetService.(*DatabaseBudgetService); ok {
		sc.backgroundWg.Add(1)
		go func() {
			defer sc.backgroundWg.Done()
			budgetService.StartMonitoring(backgroundCtx)
		}()
	}

	// Start cache cleanup task
	// Cache has its own cleanup routine, no need to start it here

	// Start token cleanup task
	if tokenAuthProvider, ok := sc.AuthProvider.(*auth.TokenAuthProvider); ok {
		sc.backgroundWg.Add(1)
		go func() {
			defer sc.backgroundWg.Done()
			sc.startTokenCleanup(backgroundCtx, tokenAuthProvider)
		}()
	}

	log.Info().Msg("Background tasks started")
}

// StopBackgroundTasks stops all background tasks gracefully
func (sc *ServiceContainer) StopBackgroundTasks() {
	if sc.backgroundCancel != nil {
		log.Info().Msg("Stopping background tasks")
		sc.backgroundCancel()
		sc.backgroundWg.Wait()
		log.Info().Msg("Background tasks stopped")
	}
}

// Cleanup performs final cleanup of all services
func (sc *ServiceContainer) Cleanup() {
	log.Info().Msg("Starting service container cleanup")

	// Flush any remaining analytics
	if analyticsService, ok := sc.AnalyticsService.(*DatabaseAnalyticsService); ok {
		if err := analyticsService.Flush(); err != nil {
			log.Error().Err(err).Msg("Failed to flush analytics during cleanup")
		}
	}

	// Close cache
	if sc.Cache != nil {
		sc.Cache.Close()
	}

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

// GetStats returns statistics about all services
func (sc *ServiceContainer) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Cache stats
	if sc.Cache != nil {
		stats["cache"] = sc.Cache.GetStats()
	}

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