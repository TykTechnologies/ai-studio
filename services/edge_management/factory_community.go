//go:build !enterprise
// +build !enterprise

package edge_management

import "gorm.io/gorm"

// FactoryFunc is the function signature for creating edge management services
type FactoryFunc func(db *gorm.DB) Service

// enterpriseFactory holds the enterprise implementation factory
var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory registers the enterprise implementation
// This is called by enterprise module init()
func RegisterEnterpriseFactory(f FactoryFunc) {
	enterpriseFactory = f
}

// NewService creates an edge management service
// Returns enterprise implementation if available, otherwise community stub
func NewService(db *gorm.DB) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(db)
	}
	return newCommunityService()
}

// IsEnterpriseAvailable returns true if enterprise edge management is available
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}
