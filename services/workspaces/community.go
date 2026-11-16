package workspaces

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// communityService: Community Edition keeps every object in the Default
// workspace; every call answers ErrEnterpriseFeature (the API's 402).
type communityService struct {
	db *gorm.DB
}

func newCommunityService(db *gorm.DB) Service {
	return &communityService{db: db}
}

func (s *communityService) Enabled() bool { return false }

func (s *communityService) List(context.Context) ([]models.Workspace, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) Get(context.Context, uint) (*models.Workspace, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) Create(context.Context, *models.User, Input) (*models.Workspace, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) Update(context.Context, *models.User, uint, Input) (*models.Workspace, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) Delete(context.Context, *models.User, uint) error {
	return ErrEnterpriseFeature
}

func (s *communityService) Counts(context.Context, uint) (map[string]int64, error) {
	return nil, ErrEnterpriseFeature
}
