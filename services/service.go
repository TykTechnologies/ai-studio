package services

import (
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
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
	}

	return &Service{
		DB:                    db,
		NotificationService:   notificationService,
		Budget:                budgetService,
		EdgeService:           edgeService,
		NamespaceService:      namespaceService,
		PluginService:         pluginService,
		PluginManifestService: pluginManifestService,
		AIStudioPluginManager: aiStudioPluginManager,
	}
}

func (s *Service) GetDB() *gorm.DB {
	return s.DB
}
