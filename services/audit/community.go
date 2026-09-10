package audit

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// communityService records nothing and answers every query with
// ErrEnterpriseFeature.
type communityService struct{}

func newCommunityService() Service {
	return &communityService{}
}

func (s *communityService) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) { c.Next() }
}

func (s *communityService) Record(ctx context.Context, rec *models.AuditRecord) error {
	return nil
}

func (s *communityService) List(ctx context.Context, q Query) (*Page, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) Get(ctx context.Context, id uint) (*models.AuditRecord, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) Summary(ctx context.Context, q Query) (*Summary, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) Export(ctx context.Context, q Query, format string) ([]byte, string, error) {
	return nil, "", ErrEnterpriseFeature
}

func (s *communityService) Cleanup(ctx context.Context) (int64, error) {
	return 0, ErrEnterpriseFeature
}

func (s *communityService) Status() Status {
	return Status{Available: false}
}

func (s *communityService) Stop() {}
