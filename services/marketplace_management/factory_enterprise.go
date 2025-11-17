//go:build enterprise
// +build enterprise

package marketplace_management

import (
	"gorm.io/gorm"
)

// NewService creates a new marketplace management service for Enterprise Edition
func NewService(db *gorm.DB) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(db)
	}
	// Fallback to nil - should never happen as enterprise init registers factory
	return nil
}
