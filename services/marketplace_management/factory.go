package marketplace_management

import (
	"gorm.io/gorm"
)

// ServiceFactory is a function that creates a marketplace management service
type ServiceFactory func(db *gorm.DB) Service

var enterpriseFactory ServiceFactory

// RegisterEnterpriseFactory registers the enterprise marketplace management factory
// This is called by the enterprise initialization code
func RegisterEnterpriseFactory(factory ServiceFactory) {
	enterpriseFactory = factory
}

// NewService creates a new marketplace management service
// Implemented in factory_community.go and factory_enterprise.go with build tags

// IsEnterpriseAvailable returns true if enterprise marketplace management is available
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}
