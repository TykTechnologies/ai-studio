package plugin_security

import "errors"

var (
	// ErrSecurityNotAvailable indicates advanced security features require Enterprise Edition
	ErrSecurityNotAvailable = errors.New("advanced plugin security features require Enterprise Edition")

	// ErrInternalNetworkBlocked indicates GRPC host targets an internal network address
	ErrInternalNetworkBlocked = errors.New("plugin command targets internal network address")

	// ErrSignatureVerificationFailed indicates the signature verification failed
	ErrSignatureVerificationFailed = errors.New("plugin signature verification failed")

	// ErrInvalidHost indicates the host is invalid or malformed
	ErrInvalidHost = errors.New("invalid or malformed host address")

	// ErrNoPublicKey indicates no public key was provided or configured
	ErrNoPublicKey = errors.New("no public key specified for signature verification")

	// ErrPublicKeyNotFound indicates the requested public key could not be found
	ErrPublicKeyNotFound = errors.New("public key not found")

	// ErrInvalidSignature indicates the signature is invalid or corrupted
	ErrInvalidSignature = errors.New("invalid or corrupted signature")
)

// IsSecurityError returns true if the error is a security-related error
func IsSecurityError(err error) bool {
	return errors.Is(err, ErrSecurityNotAvailable) ||
		errors.Is(err, ErrInternalNetworkBlocked) ||
		errors.Is(err, ErrSignatureVerificationFailed) ||
		errors.Is(err, ErrInvalidHost) ||
		errors.Is(err, ErrNoPublicKey) ||
		errors.Is(err, ErrPublicKeyNotFound) ||
		errors.Is(err, ErrInvalidSignature)
}

// IsEnterpriseRequired returns true if the error indicates Enterprise Edition is required
func IsEnterpriseRequired(err error) bool {
	return errors.Is(err, ErrSecurityNotAvailable)
}
