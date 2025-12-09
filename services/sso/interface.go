package sso

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

// Service defines the SSO service interface
// Community Edition: Returns errors indicating enterprise-only feature
// Enterprise Edition: Full SSO functionality with Tyk Identity Broker
type Service interface {
	// InitInternalTIB initializes the embedded Tyk Identity Broker
	// CE: No-op
	// ENT: Initializes TIB with GORM backend and custom dispatcher
	InitInternalTIB() error

	// GetTapProfile retrieves a TAP (Tyk Access Profile) and provider instance
	// CE: Returns ErrSSONotAvailable
	// ENT: Returns provider and profile from TIB
	GetTapProfile(id string) (TAPProvider, *TAPProfile, error)

	// GenerateNonce creates a secure nonce token for SSO authentication flow
	// CE: Returns ErrSSONotAvailable
	// ENT: Creates 32-char random token with 60s TTL
	GenerateNonce(request NonceTokenRequest) (*string, error)

	// ValidateNonceRequest validates a nonce token request
	// CE: Returns ErrSSONotAvailable
	// ENT: Validates section and other request parameters
	ValidateNonceRequest(request *NonceTokenRequest) error

	// ResolveNonce validates and optionally consumes a nonce token
	// CE: Returns ErrSSONotAvailable
	// ENT: Validates token, checks expiry, optionally consumes (deletes)
	ResolveNonce(token string, consume bool) (*NonceTokenRequest, error)

	// HandleSSO processes SSO authentication and user provisioning
	// CE: Returns ErrSSONotAvailable
	// ENT: Creates/updates user, assigns groups, sends notifications
	HandleSSO(emailAddress, displayName, groupID string, groupsIDs []string, ssoOnlyForRegisteredUsers bool) (*models.User, error)
}

// ProfileService defines the profile management service interface
// This manages SSO profiles (OIDC, SAML, LDAP, etc.)
type ProfileService interface {
	// CreateProfile creates a new SSO profile
	CreateProfile(profile *models.Profile, userID uint) error

	// GetProfileByID retrieves a profile by ID
	GetProfileByID(profileID string) (*models.Profile, error)

	// UpdateProfile updates an existing profile
	UpdateProfile(profileID string, updated *models.Profile, userID uint) error

	// DeleteProfile removes a profile
	DeleteProfile(profileID string) error

	// ListProfiles returns paginated list of profiles
	ListProfiles(pageSize, pageNumber int, all bool, sort string) ([]models.Profile, int64, error)

	// SetProfileUseInLoginPage sets a profile as the default for login page
	SetProfileUseInLoginPage(profileID string) error

	// GetLoginPageProfile returns the default login page profile
	GetLoginPageProfile() (*models.Profile, error)

	// ValidateProfile validates profile configuration and permissions
	ValidateProfile(profile *models.Profile, userID uint, validateProfileID bool) error
}

// NotificationService is an interface for the notification service
// This avoids circular dependencies
type NotificationService interface {
	Notify(notificationID, title, template string, data interface{}, userFlags uint) error
}
