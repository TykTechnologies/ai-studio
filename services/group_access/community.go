//go:build !enterprise
// +build !enterprise

package group_access

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// communityService provides a no-op implementation that bypasses all filtering
// All users have access to all resources in Community Edition
type communityService struct {
	db *gorm.DB
}

func newCommunityService(db *gorm.DB) Service {
	return &communityService{db: db}
}

// IsFilteringEnabled returns false in Community Edition
func (s *communityService) IsFilteringEnabled() bool {
	return false
}

// GetUserEntitlements returns ALL catalogues and chats (no filtering)
func (s *communityService) GetUserEntitlements(userID uint) (*Entitlements, error) {
	// CE: Return all catalogues - no group-based filtering
	var catalogues []models.Catalogue
	if err := s.db.Preload("LLMs").Find(&catalogues).Error; err != nil {
		return nil, err
	}

	var dataCatalogues []models.DataCatalogue
	if err := s.db.Preload("Datasources").Preload("Tags").Find(&dataCatalogues).Error; err != nil {
		return nil, err
	}

	var toolCatalogues []models.ToolCatalogue
	if err := s.db.Preload("Tools").Preload("Tags").Find(&toolCatalogues).Error; err != nil {
		return nil, err
	}

	var chats []models.Chat
	if err := s.db.Preload("Groups").Preload("Filters").Preload("DefaultDataSource").
		Preload("ExtraContext").Preload("DefaultTools").
		Find(&chats).Error; err != nil {
		return nil, err
	}

	return &Entitlements{
		Catalogues:     catalogues,
		DataCatalogues: dataCatalogues,
		ToolCatalogues: toolCatalogues,
		Chats:          chats,
	}, nil
}

// CanAccessCatalogue always returns true in Community Edition
func (s *communityService) CanAccessCatalogue(userID, catalogueID uint) (bool, error) {
	// CE: No filtering - everyone can access everything
	return true, nil
}

// CanAccessDataCatalogue always returns true in Community Edition
func (s *communityService) CanAccessDataCatalogue(userID, dataCatalogueID uint) (bool, error) {
	// CE: No filtering - everyone can access everything
	return true, nil
}

// CanAccessToolCatalogue always returns true in Community Edition
func (s *communityService) CanAccessToolCatalogue(userID, toolCatalogueID uint) (bool, error) {
	// CE: No filtering - everyone can access everything
	return true, nil
}
