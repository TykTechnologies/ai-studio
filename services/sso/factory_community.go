//go:build !enterprise
// +build !enterprise

package sso

import (
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
)

// NewService creates a new SSO service
// CE: Returns community stub (always returns enterprise-only errors)
func NewService(config *Config, router *gin.Engine, db *gorm.DB, notificationSvc NotificationService) Service {
	return newCommunityService()
}
