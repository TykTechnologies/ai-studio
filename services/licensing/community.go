//go:build !enterprise
// +build !enterprise

package licensing

import (
	"github.com/gin-gonic/gin"
)

// communityService is a stub implementation for Community Edition
// No license required, all features available, no telemetry
type communityService struct{}

// newCommunityService creates a new community edition licensing service
func newCommunityService() Service {
	return &communityService{}
}

// Start is a no-op for community edition
func (s *communityService) Start() error {
	return nil
}

// Stop is a no-op for community edition
func (s *communityService) Stop() {
}

// IsValid always returns true for community edition
func (s *communityService) IsValid() bool {
	return true
}

// Entitlement always returns a feature with value true for community edition
func (s *communityService) Entitlement(name string) (Feature, bool) {
	return Feature{
		Name:  name,
		Value: true,
	}, true
}

// DaysLeft returns -1 for community edition (never expires)
func (s *communityService) DaysLeft() int {
	return -1
}

// TelemetryMiddleware returns a no-op middleware for community edition
func (s *communityService) TelemetryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// SendTelemetry is a no-op for community edition
func (s *communityService) SendTelemetry() error {
	return nil
}

// GetLicenseInfo returns nil for community edition
func (s *communityService) GetLicenseInfo() *LicenseInfo {
	return nil
}
