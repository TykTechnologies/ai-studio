package model_router

import (
	"errors"

	"github.com/TykTechnologies/midsommar/v2/models"
)

var (
	// ErrEnterpriseFeature is returned when attempting to use enterprise-only features in CE
	ErrEnterpriseFeature = errors.New("model router is an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")
)

// communityService is a stub implementation of the model router service for Community Edition.
// It returns errors for all features that require Enterprise Edition.
type communityService struct{}

// newCommunityService creates a new community edition model router service stub.
func newCommunityService() Service {
	return &communityService{}
}

// CreateRouter returns an enterprise feature error in Community Edition.
func (s *communityService) CreateRouter(router *models.ModelRouter) error {
	return ErrEnterpriseFeature
}

// GetRouter returns an enterprise feature error in Community Edition.
func (s *communityService) GetRouter(id uint) (*models.ModelRouter, error) {
	return nil, ErrEnterpriseFeature
}

// GetRouterBySlug returns an enterprise feature error in Community Edition.
func (s *communityService) GetRouterBySlug(slug string, namespace string) (*models.ModelRouter, error) {
	return nil, ErrEnterpriseFeature
}

// UpdateRouter returns an enterprise feature error in Community Edition.
func (s *communityService) UpdateRouter(router *models.ModelRouter) error {
	return ErrEnterpriseFeature
}

// DeleteRouter returns an enterprise feature error in Community Edition.
func (s *communityService) DeleteRouter(id uint) error {
	return ErrEnterpriseFeature
}

// ListRouters returns an enterprise feature error in Community Edition.
func (s *communityService) ListRouters(pageSize int, pageNumber int, all bool) ([]models.ModelRouter, int64, int, error) {
	return nil, 0, 0, ErrEnterpriseFeature
}

// ListRoutersByNamespace returns an enterprise feature error in Community Edition.
func (s *communityService) ListRoutersByNamespace(namespace string) ([]models.ModelRouter, error) {
	return nil, ErrEnterpriseFeature
}

// GetActiveRouters returns an enterprise feature error in Community Edition.
func (s *communityService) GetActiveRouters() ([]models.ModelRouter, error) {
	return nil, ErrEnterpriseFeature
}

// GetActiveRoutersByNamespace returns an enterprise feature error in Community Edition.
func (s *communityService) GetActiveRoutersByNamespace(namespace string) ([]models.ModelRouter, error) {
	return nil, ErrEnterpriseFeature
}

// ValidateRouter returns an enterprise feature error in Community Edition.
func (s *communityService) ValidateRouter(router *models.ModelRouter) error {
	return ErrEnterpriseFeature
}

// ToggleRouterActive returns an enterprise feature error in Community Edition.
func (s *communityService) ToggleRouterActive(id uint, active bool) error {
	return ErrEnterpriseFeature
}
