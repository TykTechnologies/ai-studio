//go:build !enterprise
// +build !enterprise

package marketplace_management

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// communityService is the Community Edition stub for marketplace management
// It only supports the single default marketplace and returns errors for management operations
type communityService struct {
	db *gorm.DB
}

// newCommunityService creates a new community marketplace management service
func newCommunityService(db *gorm.DB) Service {
	return &communityService{db: db}
}

// AddMarketplace returns an enterprise-only error in Community Edition
func (s *communityService) AddMarketplace(url string, isDefault bool) (*models.MarketplaceIndex, error) {
	log.Warn().
		Str("url", url).
		Msg("⚠️  Marketplace Management: Adding custom marketplaces requires Enterprise Edition")
	return nil, ErrEnterpriseOnly
}

// RemoveMarketplace returns an enterprise-only error in Community Edition
func (s *communityService) RemoveMarketplace(id uint) error {
	log.Warn().
		Uint("id", id).
		Msg("⚠️  Marketplace Management: Removing marketplaces requires Enterprise Edition")
	return ErrEnterpriseOnly
}

// SetDefaultMarketplace returns an enterprise-only error in Community Edition
func (s *communityService) SetDefaultMarketplace(id uint) error {
	log.Warn().
		Uint("id", id).
		Msg("⚠️  Marketplace Management: Changing default marketplace requires Enterprise Edition")
	return ErrEnterpriseOnly
}

// ToggleMarketplace returns an enterprise-only error in Community Edition
func (s *communityService) ToggleMarketplace(id uint, active bool) error {
	log.Warn().
		Uint("id", id).
		Bool("active", active).
		Msg("⚠️  Marketplace Management: Toggling marketplaces requires Enterprise Edition")
	return ErrEnterpriseOnly
}

// ListMarketplaces returns only the default marketplace in Community Edition
func (s *communityService) ListMarketplaces() ([]*models.MarketplaceIndex, error) {
	// Community Edition: Only show the single default marketplace
	defaultIndex, err := models.GetDefaultMarketplaceIndex(s.db)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// No default marketplace exists yet
			return []*models.MarketplaceIndex{}, nil
		}
		return nil, err
	}
	return []*models.MarketplaceIndex{defaultIndex}, nil
}

// ValidateMarketplaceURL returns an enterprise-only error in Community Edition
func (s *communityService) ValidateMarketplaceURL(url string) (*ValidationResult, error) {
	log.Warn().
		Str("url", url).
		Msg("⚠️  Marketplace Management: Validating custom marketplace URLs requires Enterprise Edition")
	return nil, ErrEnterpriseOnly
}

// GetMarketplace retrieves a marketplace by ID (works in CE for default marketplace)
func (s *communityService) GetMarketplace(id uint) (*models.MarketplaceIndex, error) {
	var marketplace models.MarketplaceIndex
	err := s.db.First(&marketplace, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrMarketplaceNotFound
		}
		return nil, err
	}
	return &marketplace, nil
}

// UpdateMarketplace returns an enterprise-only error in Community Edition
func (s *communityService) UpdateMarketplace(id uint, updates *MarketplaceUpdate) error {
	log.Warn().
		Uint("id", id).
		Msg("⚠️  Marketplace Management: Updating marketplaces requires Enterprise Edition")
	return ErrEnterpriseOnly
}
