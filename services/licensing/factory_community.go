//go:build !enterprise
// +build !enterprise

package licensing

import (
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// NewService creates a new licensing service
// CE: Returns community stub (always valid)
func NewService(config Config, db *gorm.DB) Service {
	return newCommunityService()
}
