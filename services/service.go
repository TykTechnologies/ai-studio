package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

type Service struct {
	DB                     *gorm.DB
	Budget                 *BudgetService
	NotificationService    *NotificationService
	// Hub-and-Spoke Services
	EdgeService            *EdgeService
	NamespaceService       *NamespaceService
	PluginService          *PluginService
	PluginManifestService  *PluginManifestService
	AIStudioPluginManager  *AIStudioPluginManager
	PluginMetadataLoader   *PluginMetadataLoader
	MarketplaceService     *MarketplaceService
}

func NewService(db *gorm.DB) *Service {
	return NewServiceWithOCI(db, nil)
}

func NewServiceWithOCI(db *gorm.DB, ociConfig *ociplugins.OCIConfig) *Service {
	secrets.SetDBRef(db)
	notificationService := NewNotificationService(db, "", "", 0, "", "", nil) // SMTP will be configured when needed
	budgetService := NewBudgetService(db, notificationService)

	// Initialize hub-and-spoke services
	edgeService := NewEdgeService(db)
	namespaceService := NewNamespaceService(db, edgeService)

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
		logger.Info("Wired AI Studio plugin manager to plugin service for config schema functionality")
	} else {
		logger.Warnf("Failed to wire plugin manager to plugin service (plugin_service_nil: %v, ai_studio_plugin_manager_nil: %v)",
			pluginService == nil, aiStudioPluginManager == nil)
	}

	// Initialize plugin metadata loader with enhanced config provider support
	var pluginMetadataLoader *PluginMetadataLoader
	if aiStudioPluginManager != nil {
		pluginMetadataLoader = NewPluginMetadataLoader(db, aiStudioPluginManager)
		logger.Info("Initialized plugin metadata loader with enhanced config provider support")
	} else {
		logger.Warn("AI Studio plugin manager not available - plugin metadata loader will not be available")
	}

	// Initialize marketplace service (will be started separately in main.go)
	var marketplaceService *MarketplaceService
	if ociClient != nil && pluginService != nil && aiStudioPluginManager != nil {
		// Marketplace requires OCI support
		// Configuration will be passed from main.go when starting the service
		marketplaceService = nil // Will be initialized in main.go with proper config
		logger.Info("Marketplace service will be initialized with configuration from main.go")
	}

	// Create service instance
	service := &Service{
		DB:                    db,
		NotificationService:   notificationService,
		Budget:                budgetService,
		EdgeService:           edgeService,
		NamespaceService:      namespaceService,
		PluginService:         pluginService,
		PluginManifestService: pluginManifestService,
		AIStudioPluginManager: aiStudioPluginManager,
		PluginMetadataLoader:  pluginMetadataLoader,
		MarketplaceService:    marketplaceService,
	}

	// Wire service reference to AI Studio plugin manager for proper service provider injection
	if aiStudioPluginManager != nil {
		aiStudioPluginManager.SetService(service)

		// Set global service reference for GRPCServer access
		SetGlobalServiceReference(service)

		logger.Info("Wired service reference to AI Studio plugin manager for service provider injection")
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
