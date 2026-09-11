package webhooks

import (
	"context"
	"io"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// communityService does nothing and answers every operation with
// ErrEnterpriseFeature.
type communityService struct{}

func newCommunityService() Service {
	return &communityService{}
}

func (s *communityService) ListTargets(ctx context.Context, f TargetFilter) (*TargetList, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) GetTarget(ctx context.Context, id string) (*TargetDetail, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) CreateTarget(ctx context.Context, actor Actor, in TargetInput) (*models.WebhookTargetResponse, string, error) {
	return nil, "", ErrEnterpriseFeature
}

func (s *communityService) UpdateTarget(ctx context.Context, actor Actor, id string, lockVersion int, p TargetPatch) (*models.WebhookTargetResponse, bool, error) {
	return nil, false, ErrEnterpriseFeature
}

func (s *communityService) DeleteTarget(ctx context.Context, actor Actor, id string) error {
	return ErrEnterpriseFeature
}

func (s *communityService) ApproveTarget(ctx context.Context, actor Actor, id string, note string) (*models.WebhookTargetResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) RejectTarget(ctx context.Context, actor Actor, id string, reason string) (*models.WebhookTargetResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) RevokeTarget(ctx context.Context, actor Actor, id string, reason string) (*models.WebhookTargetResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) PauseTarget(ctx context.Context, actor Actor, id string) (*models.WebhookTargetResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) ResumeTarget(ctx context.Context, actor Actor, id string) (*models.WebhookTargetResponse, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) RotateSecret(ctx context.Context, actor Actor, id string) (*RotatedSecret, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) SendTest(ctx context.Context, actor Actor, id string, topic string) (string, error) {
	return "", ErrEnterpriseFeature
}

func (s *communityService) PreviewTemplate(ctx context.Context, in PreviewInput) (*PreviewResult, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) ListPresets() []Preset {
	return nil
}

func (s *communityService) ListTopics(ctx context.Context) (*TopicList, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) ListDeliveries(ctx context.Context, q DeliveryQuery) (*DeliveryPage, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) GetDelivery(ctx context.Context, id string) (*DeliveryDetail, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) ReplayDelivery(ctx context.Context, actor Actor, id string, reRender bool) (string, error) {
	return "", ErrEnterpriseFeature
}

func (s *communityService) CancelDelivery(ctx context.Context, actor Actor, id string) error {
	return ErrEnterpriseFeature
}

func (s *communityService) ReplayDeadLetters(ctx context.Context, actor Actor, req ReplayRequest) (*ReplayResult, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) Stats(ctx context.Context, window string) (*Stats, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) Export(ctx context.Context, q DeliveryQuery, format string, w io.Writer) error {
	return ErrEnterpriseFeature
}

func (s *communityService) Cleanup(ctx context.Context) (int64, error) {
	return 0, ErrEnterpriseFeature
}

func (s *communityService) Status() Status {
	return Status{Available: false}
}

func (s *communityService) Stop() {}
