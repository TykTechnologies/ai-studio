package log_export

import (
	"context"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Error definitions for log export service
var (
	// ErrEnterpriseFeature is returned when attempting to use log export in community edition
	ErrEnterpriseFeature = errors.New("proxy log export is an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")

	// ErrExportNotFound is returned when an export record doesn't exist
	ErrExportNotFound = errors.New("export not found")

	// ErrExportNotReady is returned when trying to download an incomplete export
	ErrExportNotReady = errors.New("export is not ready for download")

	// ErrExportExpired is returned when trying to download an expired export
	ErrExportExpired = errors.New("export has expired")

	// ErrUnauthorized is returned when user lacks permission
	ErrUnauthorized = errors.New("unauthorized access to export")
)

// communityService is the stub implementation for Community Edition.
// All export operations return ErrEnterpriseFeature.
type communityService struct{}

// newCommunityService creates a new community edition stub service.
func newCommunityService() Service {
	return &communityService{}
}

// StartExport returns an enterprise feature error in community edition.
func (s *communityService) StartExport(ctx context.Context, req *ExportRequest) (*models.ProxyLogExport, error) {
	return nil, ErrEnterpriseFeature
}

// GetExport returns an enterprise feature error in community edition.
func (s *communityService) GetExport(ctx context.Context, exportID string) (*models.ProxyLogExport, error) {
	return nil, ErrEnterpriseFeature
}

// GetDownloadPath returns an enterprise feature error in community edition.
func (s *communityService) GetDownloadPath(ctx context.Context, exportID string, userID uint) (string, error) {
	return "", ErrEnterpriseFeature
}

// CleanupExpired is a no-op in community edition.
func (s *communityService) CleanupExpired(ctx context.Context) error {
	return nil
}

// Stop is a no-op in community edition.
func (s *communityService) Stop() {}

// IsEnterpriseAvailable returns false for community edition.
func (s *communityService) IsEnterpriseAvailable() bool {
	return false
}
