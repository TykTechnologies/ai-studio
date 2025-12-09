package model_router

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

// Service defines the interface for model router management across both CE and ENT editions.
// Community Edition provides stub implementations that return enterprise feature errors.
// Enterprise Edition provides full model router configuration and management.
type Service interface {
	// CreateRouter creates a new model router configuration.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Creates router with pools, vendors, and mappings
	CreateRouter(router *models.ModelRouter) error

	// GetRouter retrieves a model router by ID with all relationships.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Returns fully populated router
	GetRouter(id uint) (*models.ModelRouter, error)

	// GetRouterBySlug retrieves a model router by slug and namespace.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Returns fully populated router
	GetRouterBySlug(slug string, namespace string) (*models.ModelRouter, error)

	// UpdateRouter updates an existing model router and its relationships.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Updates router with cascade to pools, vendors, mappings
	UpdateRouter(router *models.ModelRouter) error

	// DeleteRouter removes a model router by ID.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Deletes router (cascades to pools, vendors, mappings)
	DeleteRouter(id uint) error

	// ListRouters retrieves all model routers with pagination.
	// CE: Returns empty slice and ErrEnterpriseFeature
	// ENT: Returns paginated list of routers
	ListRouters(pageSize int, pageNumber int, all bool) ([]models.ModelRouter, int64, int, error)

	// ListRoutersByNamespace retrieves all model routers for a namespace.
	// CE: Returns empty slice and ErrEnterpriseFeature
	// ENT: Returns routers filtered by namespace
	ListRoutersByNamespace(namespace string) ([]models.ModelRouter, error)

	// GetActiveRouters retrieves all active model routers.
	// CE: Returns empty slice and ErrEnterpriseFeature
	// ENT: Returns active routers ready for operational use
	GetActiveRouters() ([]models.ModelRouter, error)

	// GetActiveRoutersByNamespace retrieves all active routers for a namespace.
	// CE: Returns empty slice and ErrEnterpriseFeature
	// ENT: Returns active routers for a specific namespace
	GetActiveRoutersByNamespace(namespace string) ([]models.ModelRouter, error)

	// ValidateRouter validates a router configuration.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Validates slug uniqueness, LLM references, pattern syntax
	ValidateRouter(router *models.ModelRouter) error

	// ToggleRouterActive enables or disables a router.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Updates active status
	ToggleRouterActive(id uint, active bool) error
}
