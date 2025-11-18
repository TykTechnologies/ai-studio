//go:build !enterprise
// +build !enterprise

package group_access

import "gorm.io/gorm"

// FactoryFunc is the function signature for creating group access services
type FactoryFunc func(db *gorm.DB) Service

// enterpriseFactory holds the enterprise implementation factory
var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory registers the enterprise implementation
// This is called by enterprise module init()
func RegisterEnterpriseFactory(f FactoryFunc) {
	enterpriseFactory = f
}

// NewService creates a group access service
// Returns enterprise implementation if available, otherwise community stub
func NewService(db *gorm.DB) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(db)
	}
	return newCommunityService(db)
}

// IsFilteringEnabled returns true if enterprise group filtering is available
func IsFilteringEnabled() bool {
	return enterpriseFactory != nil
}
