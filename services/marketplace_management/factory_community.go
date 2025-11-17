//go:build !enterprise
// +build !enterprise

package marketplace_management

import (
	"gorm.io/gorm"
)

// NewService creates a new marketplace management service for Community Edition
func NewService(db *gorm.DB) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(db)
	}
	return newCommunityService(db)
}
