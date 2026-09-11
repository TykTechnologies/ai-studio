package rbac

import "gorm.io/gorm"

// FactoryFunc builds an RBAC service. The enterprise submodule registers one
// from its init() when built with -tags enterprise.
type FactoryFunc func(db *gorm.DB) Service

var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory is called by the enterprise implementation's init().
func RegisterEnterpriseFactory(f FactoryFunc) {
	enterpriseFactory = f
}

// NewService returns the enterprise implementation when registered, otherwise
// the community stub. It never panics, so tests can build a Service by hand
// in either edition.
func NewService(db *gorm.DB) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(db)
	}
	return newCommunityService(db)
}

// IsEnterpriseAvailable reports whether the enterprise implementation is linked in.
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}
