package licensing

import "errors"

var (
	// ErrInvalidLicense indicates the license is invalid or malformed
	ErrInvalidLicense = errors.New("invalid license")

	// ErrExpiredLicense indicates the license has expired
	ErrExpiredLicense = errors.New("license expired")

	// ErrNoLicense indicates no license was provided
	ErrNoLicense = errors.New("no license provided")

	// ErrInvalidSignature indicates the JWT signature is invalid
	ErrInvalidSignature = errors.New("invalid license signature")

	// ErrFeatureNotAvailable indicates a feature is not included in the license
	ErrFeatureNotAvailable = errors.New("feature not available in license")

	// ErrTelemetryFailed indicates telemetry transmission failed
	ErrTelemetryFailed = errors.New("telemetry transmission failed")
)
