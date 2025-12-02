package log_export

import (
	"context"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// ExportRequest contains parameters for initiating a proxy log export
type ExportRequest struct {
	SourceType   models.ExportSourceType
	SourceID     uint
	StartDate    time.Time
	EndDate      time.Time
	SearchFilter string
	RequestedBy  uint // Admin user ID
}

// Service defines the interface for proxy log export functionality.
// This is an Enterprise Edition feature - community edition returns errors.
type Service interface {
	// StartExport initiates a new export job (returns immediately, processes in background)
	// CE: Returns enterprise feature error
	// ENT: Creates job, starts background goroutine, returns job ID
	StartExport(ctx context.Context, req *ExportRequest) (*models.ProxyLogExport, error)

	// GetExport retrieves export job status by export ID
	GetExport(ctx context.Context, exportID string) (*models.ProxyLogExport, error)

	// GetDownloadPath returns file path if export is ready and not expired
	// Validates admin access and expiration
	GetDownloadPath(ctx context.Context, exportID string, userID uint) (string, error)

	// CleanupExpired removes expired export files and marks records as expired
	// Called periodically by cleanup routine
	CleanupExpired(ctx context.Context) error

	// Stop gracefully shuts down the service (stops cleanup goroutine)
	Stop()

	// IsEnterpriseAvailable returns whether enterprise features are available
	IsEnterpriseAvailable() bool
}
