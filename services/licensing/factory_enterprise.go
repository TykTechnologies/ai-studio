//go:build enterprise
// +build enterprise

package licensing

import (
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// NewService creates a new licensing service
// ENT: Returns enterprise implementation (must be registered)
func NewService(config Config, db *gorm.DB) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(config, db)
	}
	// Unreachable once edition.CheckRegistered has passed at startup.
	panic("Enterprise licensing factory not registered; see edition.CheckRegistered")
}
