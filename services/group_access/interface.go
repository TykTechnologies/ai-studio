package group_access

import "github.com/TykTechnologies/midsommar/v2/models"

// Entitlements contains the catalogues and chats accessible to a user
type Entitlements struct {
	Catalogues     []models.Catalogue
	DataCatalogues []models.DataCatalogue
	ToolCatalogues []models.ToolCatalogue
	Chats          []models.Chat
}

// Service defines the interface for group-based access control
// Enterprise Edition: Enforces group-based filtering of resources
// Community Edition: Bypasses all filtering (everyone sees everything)
type Service interface {
	// IsFilteringEnabled returns true if group-based filtering should be applied
	// ENT: returns true, CE: returns false
	IsFilteringEnabled() bool

	// GetUserEntitlements returns the catalogues, chats, and resources accessible to a user
	// ENT: filtered by user's group memberships
	// CE: returns all resources (no filtering)
	GetUserEntitlements(userID uint) (*Entitlements, error)

	// CanAccessCatalogue checks if a user has access to a specific LLM catalogue
	// ENT: checks group membership
	// CE: always returns true
	CanAccessCatalogue(userID, catalogueID uint) (bool, error)

	// CanAccessDataCatalogue checks if a user has access to a specific data catalogue
	// ENT: checks group membership
	// CE: always returns true
	CanAccessDataCatalogue(userID, dataCatalogueID uint) (bool, error)

	// CanAccessToolCatalogue checks if a user has access to a specific tool catalogue
	// ENT: checks group membership
	// CE: always returns true
	CanAccessToolCatalogue(userID, toolCatalogueID uint) (bool, error)
}
