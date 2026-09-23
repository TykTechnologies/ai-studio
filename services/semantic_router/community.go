package semantic_router

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/models"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"gorm.io/gorm"
)

// communityService is the Community Edition stub: every method answers
// ErrEnterpriseFeature.
type communityService struct{}

func newCommunityService() Service { return &communityService{} }

func (s *communityService) CreateRouter(*models.SemanticRouter) error { return ErrEnterpriseFeature }

func (s *communityService) GetRouter(uint) (*models.SemanticRouter, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) UpdateRouter(*models.SemanticRouter) error { return ErrEnterpriseFeature }

func (s *communityService) DeleteRouter(uint) error { return ErrEnterpriseFeature }

func (s *communityService) ListRouters(int, int, bool, ...func(*gorm.DB) *gorm.DB) ([]models.SemanticRouter, int64, int, error) {
	return nil, 0, 0, ErrEnterpriseFeature
}

func (s *communityService) ToggleRouterActive(uint, bool) error { return ErrEnterpriseFeature }

func (s *communityService) ValidateRouter(*models.SemanticRouter) error { return ErrEnterpriseFeature }

func (s *communityService) Test(context.Context, *models.SemanticRouter, sr.Request) (*sr.Decision, error) {
	return nil, ErrEnterpriseFeature
}
