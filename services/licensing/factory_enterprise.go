//go:build enterprise
// +build enterprise

package licensing

import (
	"log"

	"gorm.io/gorm"
)

// NewService creates a new licensing service
// ENT: Returns enterprise implementation (must be registered)
func NewService(config Config, db *gorm.DB) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(config, db)
	}
	// This should never happen in ENT builds if init() was called
	log.Fatal("Enterprise licensing factory not registered")
	return nil
}
