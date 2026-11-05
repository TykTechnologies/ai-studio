package entitlements

import (
	"github.com/TykTechnologies/midsommar/v2/services/licensing"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Deps is everything the enterprise implementation needs from the host.
type Deps struct {
	DB *gorm.DB
	// Licensing returns the licensing service, nil while none is set. It is
	// a getter because the license is attached after the service exists.
	Licensing func() licensing.Service
}

// FactoryFunc builds an entitlements service. The enterprise submodule
// registers one from its init() when built with -tags enterprise.
type FactoryFunc func(deps Deps) Service

var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory is called by the enterprise implementation's init().
func RegisterEnterpriseFactory(f FactoryFunc) {
	enterpriseFactory = f
}

// NewService returns the enterprise implementation when registered, otherwise
// the community stub. An enterprise build without the implementation fails
// edition.CheckRegistered at startup.
func NewService(deps Deps) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(deps)
	}
	return NewCommunityService()
}

// IsEnterpriseAvailable reports whether the enterprise implementation is linked in.
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}
