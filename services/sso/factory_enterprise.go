//go:build enterprise
// +build enterprise

package sso

import (
	"log"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NewService creates a new SSO service
// ENT: Returns enterprise implementation via registered factory
func NewService(config *Config, router *gin.Engine, db *gorm.DB, notificationSvc NotificationService) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(config, router, db, notificationSvc)
	}
	log.Fatal("Enterprise SSO factory not registered")
	return nil
}
