package sso

import "errors"

var (
	// ErrSSONotAvailable is returned when SSO functionality is not available (CE edition)
	ErrSSONotAvailable = errors.New("SSO is an Enterprise feature")

	// ErrTIBNotInitialized is returned when TIB has not been initialized
	ErrTIBNotInitialized = errors.New("Tyk Identity Broker not initialized")

	// ErrInvalidProfile is returned when a profile cannot be found or is invalid
	ErrInvalidProfile = errors.New("Invalid SSO profile")

	// ErrNonceGenerationFailed is returned when nonce token generation fails
	ErrNonceGenerationFailed = errors.New("Failed to generate nonce token")

	// ErrNonceNotFound is returned when a nonce token cannot be found
	ErrNonceNotFound = errors.New("Nonce token not found")

	// ErrNonceExpired is returned when a nonce token has expired
	ErrNonceExpired = errors.New("Nonce token has expired")

	// ErrInvalidSection is returned when an invalid section is specified in nonce request
	ErrInvalidSection = errors.New("Invalid section in nonce request")

	// ErrUserCreationFailed is returned when SSO user creation fails
	ErrUserCreationFailed = errors.New("Failed to create user via SSO")

	// ErrSSOOnlyForRegistered is returned when SSO is restricted to registered users
	ErrSSOOnlyForRegistered = errors.New("SSO only enabled for registered users")
)
