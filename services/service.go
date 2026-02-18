package services

import (
	"context"
	"fmt"
	"os"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/TykTechnologies/midsommar/v2/services/edge_management"
	"github.com/TykTechnologies/midsommar/v2/services/group_access"
	"github.com/TykTechnologies/midsommar/v2/services/licensing"
	"github.com/TykTechnologies/midsommar/v2/services/log_export"
	"github.com/TykTechnologies/midsommar/v2/services/model_router"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
	"gorm.io/gorm"
)

type Service struct {
	DB                  *gorm.DB
	Budget              budget.Service
	GroupAccessService  group_access.Service
	NotificationService *NotificationService
	LogExportService    log_export.Service
	// Hub-and-Spoke Services
	EdgeService           *EdgeService
	NamespaceService      *NamespaceService
	EdgeManagementService edge_management.Service
	PluginService         *PluginService
	PluginManifestService *PluginManifestService
	AIStudioPluginManager *AIStudioPluginManager
	PluginMetadataLoader  *PluginMetadataLoader
	MarketplaceService    *MarketplaceService
	// Object Hooks
	HookRegistry *HookRegistry
	HookManager  *HookManager
	// System Events
	EventBus     eventbridge.Bus
	SystemEvents *SystemEventEmitter
	// Licensing (set after creation via SetLicensingService)
	LicensingService licensing.Service
	// Model Router (Enterprise)
	ModelRouterService model_router.Service
	// Sync Status (Hub-and-Spoke)
	SyncStatusService *SyncStatusService
}

func NewService(db *gorm.DB) *Service {
	return NewServiceWithOCI(db, nil)
}

func NewServiceWithOCI(db *gorm.DB, ociConfig *ociplugins.OCIConfig) *Service {
	secrets.SetDBRef(db)
	notificationService := NewNotificationService(db, "", "", 0, "", "", nil) // SMTP will be configured when needed
	budgetSvc := budget.NewService(db, notificationService)
	groupAccessSvc := group_access.NewService(db)
	modelRouterSvc := model_router.NewService(db)

	// Initialize log export service with storage path from environment
	exportStoragePath := os.Getenv("EXPORT_STORAGE_PATH")
	if exportStoragePath == "" {
		exportStoragePath = "./data/exports"
	}
	siteURL := os.Getenv("SITE_URL")
	logExportSvc := log_export.NewService(db, notificationService, exportStoragePath, siteURL)

	// Initialize hub-and-spoke services
	edgeService := NewEdgeService(db)
	namespaceService := NewNamespaceService(db, edgeService)
	edgeManagementService := edge_management.NewService(db)
	syncStatusService := NewSyncStatusService(db)

	// Initialize plugin services with OCI support
	var pluginService *PluginService
	var pluginManifestService *PluginManifestService
	var err error

	var ociClient *ociplugins.OCIPluginClient
	var aiStudioPluginManager *AIStudioPluginManager

	if ociConfig != nil {
		// Create plugin service with OCI support
		pluginService, err = NewPluginServiceWithOCI(db, ociConfig)
		if err != nil {
			// Fallback to basic plugin service if OCI setup fails
			pluginService = NewPluginService(db)
			pluginManifestService = NewPluginManifestService(db, nil)
			aiStudioPluginManager = NewAIStudioPluginManager(db, nil)
		} else {
			// Create OCI client and services with OCI support
			ociClient, _ = ociplugins.NewOCIPluginClient(ociConfig)
			pluginManifestService = NewPluginManifestService(db, ociClient)
			aiStudioPluginManager = NewAIStudioPluginManager(db, ociClient)
		}
	} else {
		// No OCI support
		pluginService = NewPluginService(db)
		pluginManifestService = NewPluginManifestService(db, nil)
		aiStudioPluginManager = NewAIStudioPluginManager(db, nil)
	}

	// Wire up manifest service to plugin manager for auto-manifest functionality
	if aiStudioPluginManager != nil && pluginManifestService != nil {
		aiStudioPluginManager.SetManifestService(pluginManifestService)
	}

	// Wire up plugin manager to plugin service for config schema functionality
	if pluginService != nil && aiStudioPluginManager != nil {
		pluginService.SetPluginManager(aiStudioPluginManager)
		logger.Debug("Wired AI Studio plugin manager to plugin service for config schema functionality")
	} else {
		logger.Debugf("Failed to wire plugin manager to plugin service (plugin_service_nil: %v, ai_studio_plugin_manager_nil: %v)",
			pluginService == nil, aiStudioPluginManager == nil)
	}

	// Wire enterprise security service to OCI clients for signature verification
	if ociConfig != nil {
		secConfig := &plugin_security.Config{
			OCIConfig:                  ociConfig,
			AllowInternalNetworkAccess: os.Getenv("ALLOW_INTERNAL_NETWORK_ACCESS") == "true",
		}
		secService := plugin_security.NewService(secConfig)
		if pluginService != nil {
			pluginService.SetSecurityService(secService)
		}
		if aiStudioPluginManager != nil {
			aiStudioPluginManager.SetSecurityService(secService)
		}
		logger.Debug("Wired plugin security service to OCI clients")
	}

	// Initialize plugin metadata loader with enhanced config provider support
	var pluginMetadataLoader *PluginMetadataLoader
	if aiStudioPluginManager != nil {
		pluginMetadataLoader = NewPluginMetadataLoader(db, aiStudioPluginManager)
		logger.Debug("Initialized plugin metadata loader with enhanced config provider support")
	} else {
		logger.Debug("AI Studio plugin manager not available - plugin metadata loader will not be available")
	}

	// Initialize marketplace service (will be started separately in main.go)
	var marketplaceService *MarketplaceService
	if ociClient != nil && pluginService != nil && aiStudioPluginManager != nil {
		// Marketplace requires OCI support
		// Configuration will be passed from main.go when starting the service
		marketplaceService = nil // Will be initialized in main.go with proper config
		logger.Debug("Marketplace service will be initialized with configuration from main.go")
	}

	// Initialize object hooks system
	hookRegistry := NewHookRegistry()
	var hookManager *HookManager
	if aiStudioPluginManager != nil {
		hookManager = NewHookManager(hookRegistry, aiStudioPluginManager)
		logger.Debug("Initialized object hooks system")
	} else {
		logger.Debug("AI Studio plugin manager not available - object hooks will not be available")
	}

	// Create service instance
	service := &Service{
		DB:                    db,
		NotificationService:   notificationService,
		Budget:                budgetSvc,
		GroupAccessService:    groupAccessSvc,
		LogExportService:      logExportSvc,
		EdgeService:           edgeService,
		NamespaceService:      namespaceService,
		EdgeManagementService: edgeManagementService,
		PluginService:         pluginService,
		PluginManifestService: pluginManifestService,
		AIStudioPluginManager: aiStudioPluginManager,
		PluginMetadataLoader:  pluginMetadataLoader,
		MarketplaceService:    marketplaceService,
		HookRegistry:          hookRegistry,
		HookManager:           hookManager,
		ModelRouterService:    modelRouterSvc,
		SyncStatusService:     syncStatusService,
	}

	// Wire service reference to AI Studio plugin manager for proper service provider injection
	if aiStudioPluginManager != nil {
		aiStudioPluginManager.SetService(service)

		// Set global service reference for GRPCServer access
		SetGlobalServiceReference(service)

		logger.Debug("Wired service reference to AI Studio plugin manager for service provider injection")
	}

	return service
}

func (s *Service) GetDB() *gorm.DB {
	return s.DB
}

// GetPluginClient gets a loaded plugin client by plugin ID
func (s *Service) GetPluginClient(pluginID uint) (pb.PluginServiceClient, error) {
	if s.AIStudioPluginManager == nil {
		return nil, fmt.Errorf("AI Studio plugin manager not available")
	}

	loadedPlugin, err := s.AIStudioPluginManager.LoadPlugin(pluginID)
	if err != nil {
		return nil, fmt.Errorf("failed to load plugin: %w", err)
	}

	return loadedPlugin.GRPCClient, nil
}

// Cleanup performs graceful cleanup of all services
func (s *Service) Cleanup() error {
	logger.Info("Starting service cleanup...")

	var errors []error

	// Stop log export service (cleanup goroutine)
	if s.LogExportService != nil {
		logger.Info("Stopping log export service...")
		s.LogExportService.Stop()
		logger.Info("Log export service stopped")
	}

	// Shutdown plugin manager first (most critical)
	if s.AIStudioPluginManager != nil {
		if err := s.AIStudioPluginManager.Shutdown(); err != nil {
			logger.Errorf("Failed to shutdown plugin manager: %v", err)
			errors = append(errors, fmt.Errorf("plugin manager shutdown: %w", err))
		} else {
			logger.Info("Plugin manager shutdown completed")
		}
	}

	// Stop marketplace service if running
	if s.MarketplaceService != nil {
		logger.Info("Stopping marketplace service...")
		// MarketplaceService.Stop() would need context - it's already managed via defer cancel() in main.go
		// No explicit stop needed as context cancellation handles it
		logger.Info("Marketplace service will be stopped via context cancellation")
	}

	// Cleanup hook registry
	if s.HookRegistry != nil {
		logger.Info("Cleaning up hook registry...")
		// Hook registry cleanup if needed
		// Currently no explicit cleanup required
	}

	// Close database connections
	if s.DB != nil {
		logger.Info("Closing database connections...")
		sqlDB, err := s.DB.DB()
		if err == nil {
			if err := sqlDB.Close(); err != nil {
				logger.Errorf("Failed to close database: %v", err)
				errors = append(errors, fmt.Errorf("database close: %w", err))
			} else {
				logger.Info("Database connections closed")
			}
		}
	}

	logger.Info("Service cleanup completed")

	if len(errors) > 0 {
		return fmt.Errorf("errors during cleanup: %v", errors)
	}
	return nil
}

// CleanupWithContext performs graceful cleanup with a timeout context
func (s *Service) CleanupWithContext(ctx context.Context) error {
	// Create a channel to signal completion
	done := make(chan error, 1)

	go func() {
		done <- s.Cleanup()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		logger.Warn("Service cleanup timeout exceeded")
		return fmt.Errorf("cleanup timeout: %w", ctx.Err())
	}
}

// SetEventBus sets the event bus and initializes the system event emitter
func (s *Service) SetEventBus(bus eventbridge.Bus) {
	s.EventBus = bus
	if bus != nil {
		s.SystemEvents = NewSystemEventEmitter(bus, "control")
		s.SubscribeResourceInstanceChanges(bus)
		logger.Debug("Initialized system event emitter")
	}
}

// SetLicensingService sets the licensing service for plugin license checks
func (s *Service) SetLicensingService(svc licensing.Service) {
	s.LicensingService = svc
	logger.Debug("Licensing service set on main service")
}
