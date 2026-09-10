package audit

import (
	"github.com/TykTechnologies/midsommar/v2/config"
	"gorm.io/gorm"
)

// FactoryFunc builds an audit service. The enterprise submodule registers one
// from its init() when built with -tags enterprise.
type FactoryFunc func(db *gorm.DB, cfg config.AuditConfig) Service

var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory is called by the enterprise implementation's init().
func RegisterEnterpriseFactory(f FactoryFunc) {
	enterpriseFactory = f
}

// NewService returns the enterprise implementation when registered, otherwise
// the community no-op.
func NewService(db *gorm.DB, cfg config.AuditConfig) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(db, cfg)
	}
	return newCommunityService()
}

// IsEnterpriseAvailable reports whether the enterprise implementation is linked in.
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}
