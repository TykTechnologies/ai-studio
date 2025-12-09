package model_router

import (
	"gorm.io/gorm"
)

// FactoryFunc is a function type that creates a model router service instance.
// Enterprise code registers its factory via init() function.
type FactoryFunc func(db *gorm.DB) Service

// enterpriseFactory holds the enterprise implementation factory if available.
// This is set by the enterprise submodule's init() function when building with -tags enterprise.
var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory allows the enterprise implementation to register itself.
// This is called from the enterprise submodule's init() function.
func RegisterEnterpriseFactory(f FactoryFunc) {
	enterpriseFactory = f
}

// NewService creates a new model router service instance.
// Returns enterprise implementation if available (when built with -tags enterprise),
// otherwise returns community edition stub.
func NewService(db *gorm.DB) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(db)
	}
	// Fallback to community edition stub
	return newCommunityService()
}

// IsEnterpriseAvailable returns true if enterprise features are available.
// Used by edition detection API and feature gating.
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}
