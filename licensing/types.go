package licensing

import (
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/golang-jwt/jwt/v5"
)

const (
	TrackLicenseUsage              = "track"
	FEATUREPortal                  = "feature_portal"
	FEATUREChat                    = "feature_chat"
	FEATUREGateway                 = "feature_gateway"
	claimVersion                   = "v"
	claimExp                       = "exp"
	claimScope                     = "scope"
	DefaultTelemetryPeriod         = 1 * time.Hour
	DefaultValidityCheckPeriod     = 10 * time.Minute
	MaxConcurrentTelemetryRequests = 20
	telemetryAPIURL                = "https://telemetry.tyk.technologies"
	CtxActionKey                   = "telemetry_action"
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

type Client struct {
	http *http.Client
	URL  string
}

type Event struct {
	Identity   string                 `json:"identity"`
	Event      string                 `json:"event"`
	Timestamp  int64                  `json:"timestamp"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}
type Feature struct {
	tp        string
	valBool   bool
	valString string
	valInt    int
}
