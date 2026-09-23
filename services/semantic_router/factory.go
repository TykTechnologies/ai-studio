package semantic_router

import "gorm.io/gorm"

// FactoryFunc creates the service. The Enterprise module registers one from
// its init().
type FactoryFunc func(db *gorm.DB) Service

var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory installs the Enterprise implementation.
func RegisterEnterpriseFactory(f FactoryFunc) { enterpriseFactory = f }

// NewService returns the Enterprise service when it is built in, otherwise
// the Community Edition stub.
func NewService(db *gorm.DB) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(db)
	}
	return newCommunityService()
}

// IsEnterpriseAvailable reports whether the Enterprise service is built in.
func IsEnterpriseAvailable() bool { return enterpriseFactory != nil }
