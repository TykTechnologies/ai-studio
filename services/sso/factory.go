package sso

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// FactoryFunc is a function that creates an SSO service
type FactoryFunc func(config *Config, router *gin.Engine, db *gorm.DB, notificationSvc NotificationService) Service

// enterpriseFactory holds the enterprise implementation factory
var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory registers the enterprise SSO factory
// This is called by the enterprise submodule's init() function
func RegisterEnterpriseFactory(f FactoryFunc) {
	enterpriseFactory = f
}

// IsEnterpriseAvailable returns true if enterprise SSO is available
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}

// NewService is implemented in factory_community.go or factory_enterprise.go
// based on build tags
