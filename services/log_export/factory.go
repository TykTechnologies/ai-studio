package log_export

import (
	"gorm.io/gorm"
)

// NotificationService is the interface we need from services package.
// This avoids import cycle between services and services/log_export.
type NotificationService interface{}

// FactoryFunc is a function type that creates a log export service instance.
// Enterprise code registers its factory via init() function.
type FactoryFunc func(db *gorm.DB, notificationSvc NotificationService, storagePath, siteURL string) Service

// enterpriseFactory holds the enterprise implementation factory if available.
// This is set by the enterprise submodule's init() function when building with -tags enterprise.
var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory allows the enterprise implementation to register itself.
// This is called from the enterprise submodule's init() function.
func RegisterEnterpriseFactory(f FactoryFunc) {
	enterpriseFactory = f
}

// NewService creates a new log export service instance.
// Returns enterprise implementation if available (when built with -tags enterprise),
// otherwise returns community edition stub.
func NewService(db *gorm.DB, notificationSvc NotificationService, storagePath, siteURL string) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(db, notificationSvc, storagePath, siteURL)
	}
	// Fallback to community edition stub
	return newCommunityService()
}

// IsEnterpriseAvailable returns true if enterprise features are available.
// Used by edition detection API and feature gating.
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}
