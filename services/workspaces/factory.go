package workspaces

import (
	"github.com/TykTechnologies/midsommar/v2/services/rbac"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// FactoryFunc builds a Workspaces service. The enterprise submodule
// registers one from its init() when built with -tags enterprise. authz is
// the RBAC service: Workspaces are manageable only when it is enabled.
type FactoryFunc func(db *gorm.DB, authz rbac.Service) Service

var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory is called by the enterprise implementation's init().
func RegisterEnterpriseFactory(f FactoryFunc) {
	enterpriseFactory = f
}

// NewService returns the enterprise implementation when registered,
// otherwise the community stub.
func NewService(db *gorm.DB, authz rbac.Service) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(db, authz)
	}
	return newCommunityService(db)
}

// IsEnterpriseAvailable reports whether the enterprise implementation is linked in.
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}
