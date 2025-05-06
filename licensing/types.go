package licensing

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/golang-jwt/jwt/v5"
)

const (
	TrackLicenseUsage = "track"

	FEATUREPortal  = "feature_portal"
	FEATUREChat    = "feature_chat"
	FEATUREGateway = "feature_gateway"

	claimVersion = "v"
	claimExp     = "exp"
	claimScope   = "scope"
)

type LicenseInfo struct {
	Key       string
	IsValid   bool
	ExpiresAt time.Time
	Version   string
	Features  map[string]*Feature
	claims    jwt.MapClaims
}

type LicenseConfig struct {
	LicenseKey          string
	ValidityCheckPeriod time.Duration
	TelemetryPeriod     time.Duration
	DisableTelemetry    bool
	TelemetryURL        string
	Version             string
	Component           string
	TelemetryService    *services.TelemetryService
}
