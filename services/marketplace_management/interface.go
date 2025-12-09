package marketplace_management

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

// Service defines the interface for marketplace management operations
// Community Edition: Returns enterprise-only errors
// Enterprise Edition: Full CRUD operations for managing multiple marketplace sources
type Service interface {
	// AddMarketplace adds a new marketplace index
	// Returns error in CE, success in ENT
	AddMarketplace(url string, isDefault bool) (*models.MarketplaceIndex, error)

	// RemoveMarketplace removes a marketplace index by ID
	// Cannot remove the default Tyk marketplace
	// Returns error in CE, success in ENT
	RemoveMarketplace(id uint) error

	// SetDefaultMarketplace sets a marketplace as the default
	// Only one marketplace can be default at a time
	// Returns error in CE, success in ENT
	SetDefaultMarketplace(id uint) error

	// ToggleMarketplace activates or deactivates a marketplace
	// Inactive marketplaces are not synced
	// Returns error in CE, success in ENT
	ToggleMarketplace(id uint, active bool) error

	// ListMarketplaces returns all marketplace indexes (active and inactive)
	// CE: Returns only the default marketplace
	// ENT: Returns all configured marketplaces
	ListMarketplaces() ([]*models.MarketplaceIndex, error)

	// ValidateMarketplaceURL validates a marketplace URL before adding
	// Checks URL format, accessibility, and index.yaml structure
	// Returns error in CE, validation result in ENT
	ValidateMarketplaceURL(url string) (*ValidationResult, error)

	// GetMarketplace retrieves a specific marketplace by ID
	GetMarketplace(id uint) (*models.MarketplaceIndex, error)

	// UpdateMarketplace updates marketplace properties (but not URL)
	// Returns error in CE, success in ENT
	UpdateMarketplace(id uint, updates *MarketplaceUpdate) error
}

// ValidationResult contains marketplace URL validation results
type ValidationResult struct {
	Valid        bool   `json:"valid"`
	PluginCount  int    `json:"plugin_count"`
	APIVersion   string `json:"api_version"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// MarketplaceUpdate contains updatable marketplace fields
type MarketplaceUpdate struct {
	IsActive  *bool `json:"is_active,omitempty"`
	IsDefault *bool `json:"is_default,omitempty"`
}
