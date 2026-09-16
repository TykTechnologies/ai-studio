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

func (s *communityService) ListServers(ctx context.Context, f ServerFilter) (*ServerList, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) GetServer(ctx context.Context, id uint) (*models.MCPServerResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) UpdateServer(ctx context.Context, actor Actor, id uint, p ServerPatch) (*models.MCPServerResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) DeleteServer(ctx context.Context, actor Actor, id uint) error {
	return ErrEnterpriseFeature
}

func (s *communityService) PublishServer(ctx context.Context, actor Actor, id uint) (*models.MCPServerResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) UnpublishServer(ctx context.Context, actor Actor, id uint) (*models.MCPServerResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) SetServerGroups(ctx context.Context, actor Actor, id uint, groupIDs []uint) (*models.MCPServerResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) SetServerBundle(ctx context.Context, actor Actor, id uint, pins []PinInput) (*models.MCPServerResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) ListPolicies(ctx context.Context, connectionID uint, f PolicyFilter) ([]models.TykPolicyResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) ListSyncRuns(ctx context.Context, connectionID uint, limit int) ([]models.MCPSyncRun, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) RunSync(ctx context.Context, actor Actor, connectionID uint) (*models.MCPSyncRun, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) SyncAppGrants(ctx context.Context, appID uint) error {
	return nil
}

func (s *communityService) AppMCPSummary(ctx context.Context, appID uint) (*AppMCPSummary, error) {
	return &AppMCPSummary{Servers: []AppMCPServerView{}}, nil
}
