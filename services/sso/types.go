package sso

import (
	"time"

	"github.com/TykTechnologies/tyk-identity-broker/tap"
)

const (
	DashboardSection = "dashboard"
	NonceLength      = 32
	NonceTTL         = 60 * time.Second
	DefaultGroupID   = "1"

	// ProfileIDHeader carries the identity provider profile that started a
	// login. The embedded broker's dispatcher sets it on the nonce request
	// so provisioning can apply that profile's defaults; an external broker
	// never sends it and new users then get the NewUser defaults.
	ProfileIDHeader = "X-Tyk-AI-Profile-ID"
)

// Config holds SSO service configuration
type Config struct {
	APISecret string
	LogLevel  string
}

// NonceTokenRequest represents a request to create a nonce token for SSO authentication
type NonceTokenRequest struct {
	ForSection                string
	OrgID                     string
	EmailAddress              string
	GroupID                   string
	GroupsIDs                 []string
	DisplayName               string
	SSOOnlyForRegisteredUsers bool
	// ProfileID is the identity provider profile that produced this login,
	// taken from ProfileIDHeader; empty when the broker did not identify one.
	ProfileID string
	ExpiresAt time.Time
}

// NonceTokenResponse represents the response from a nonce token creation request
type NonceTokenResponse struct {
	Meta    *string `json:"Meta"`
	Status  string  `json:"Status"`
	Message string  `json:"Message"`
}

// TAPProfile represents a Tyk Access Profile from TIB
// This is an alias to avoid exposing TIB internals in the interface
type TAPProfile = tap.Profile

// TAPProvider represents a Tyk Access Provider from TIB
// This is an alias to avoid exposing TIB internals in the interface
type TAPProvider = tap.TAProvider
