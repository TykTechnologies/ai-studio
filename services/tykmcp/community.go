package tykmcp

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// communityService does nothing and answers every operation with
// ErrEnterpriseFeature.
type communityService struct{}

func newCommunityService() Service {
	return &communityService{}
}

// NewCommunityService returns the always-refusing stub regardless of which
// implementation is linked in (the API's fallback for tests that never call
// InitTykMCP).
func NewCommunityService() Service {
	return newCommunityService()
}

func (s *communityService) ListConnections(ctx context.Context) ([]models.TykConnectionResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) GetConnection(ctx context.Context, id uint) (*models.TykConnectionResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) CreateConnection(ctx context.Context, actor Actor, in ConnectionInput) (*models.TykConnectionResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) UpdateConnection(ctx context.Context, actor Actor, id uint, p ConnectionPatch) (*models.TykConnectionResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) DeleteConnection(ctx context.Context, actor Actor, id uint, force bool) error {
	return ErrEnterpriseFeature
}

func (s *communityService) ActivateConnection(ctx context.Context, actor Actor, id uint) (*models.TykConnectionResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) DisableConnection(ctx context.Context, actor Actor, id uint, reason string) (*models.TykConnectionResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) ProbeConnection(ctx context.Context, actor Actor, id uint) (*ProbeResult, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) ProbeInput(ctx context.Context, in ConnectionInput) (*ProbeResult, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) TriggerSync(ctx context.Context, actor Actor, id uint) error {
	return ErrEnterpriseFeature
}

func (s *communityService) Status() Status {
	return Status{Available: false}
}

func (s *communityService) Stop() {}
