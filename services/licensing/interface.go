package licensing

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Service defines the licensing service interface
// Community Edition: Always valid, no checks, no telemetry
// Enterprise Edition: JWT validation, periodic checks, telemetry
type Service interface {
	// Start initializes and starts the licensing service
	// - Validates license at boot (ENT: exits if invalid)
	// - Starts periodic validation checks (ENT: every 24h)
	// - Starts telemetry collection (ENT: every 1h)
	Start() error

	// Stop gracefully stops all background processes
	Stop()

	// IsValid returns whether the license is currently valid
	// CE: Always returns true
	// ENT: Returns true if JWT is valid and not expired
	IsValid() bool

	// Entitlement checks if a specific feature is available
	// Returns the feature value and whether it exists
	// CE: All features return true
	// ENT: Checks JWT scope claim
	Entitlement(name string) (Feature, bool)

	// DaysLeft returns the number of days until license expiry
	// CE: Returns -1 (never expires)
	// ENT: Returns days until JWT exp claim
	DaysLeft() int

	// TelemetryMiddleware returns a Gin middleware for tracking HTTP requests
	// CE: No-op middleware
	// ENT: Tracks actions, status codes, access types
	TelemetryMiddleware() gin.HandlerFunc

	// SendTelemetry manually triggers a telemetry collection and send
	// CE: No-op
	// ENT: Collects stats and sends to telemetry endpoint
	SendTelemetry() error

	// GetLicenseInfo returns the current license information
	// CE: Returns nil
	// ENT: Returns parsed JWT claims and features
	GetLicenseInfo() *LicenseInfo
}

// TelemetryCollector defines the interface for collecting usage statistics
type TelemetryCollector interface {
	// CollectLLMStats collects LLM usage statistics
	CollectLLMStats(db *gorm.DB) (map[string]interface{}, error)

	// CollectAppStats collects application statistics
	CollectAppStats(db *gorm.DB) (map[string]interface{}, error)

	// CollectUserStats collects user statistics
	CollectUserStats(db *gorm.DB) (map[string]interface{}, error)

	// CollectChatStats collects chat statistics
	CollectChatStats(db *gorm.DB) (map[string]interface{}, error)
}
