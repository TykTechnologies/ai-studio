//go:build enterprise
// +build enterprise

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
// In enterprise builds, always uses enterprise implementation
func NewService(db *gorm.DB) Service {
	if enterpriseFactory == nil {
		panic("enterprise edge management factory not registered")
	}
	return enterpriseFactory(db)
}

// IsEnterpriseAvailable always returns true in enterprise builds
func IsEnterpriseAvailable() bool {
	return true
}
