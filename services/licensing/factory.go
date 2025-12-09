package licensing

import (
	"gorm.io/gorm"
)

// FactoryFunc is a function that creates a licensing service
type FactoryFunc func(config Config, db *gorm.DB) Service

// enterpriseFactory holds the enterprise implementation factory
var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory registers the enterprise licensing factory
// This is called by the enterprise submodule's init() function
func RegisterEnterpriseFactory(f FactoryFunc) {
	enterpriseFactory = f
}

// IsEnterpriseAvailable returns true if enterprise licensing is available
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}

// NewService is implemented in factory_community.go or factory_enterprise.go
// based on build tags
