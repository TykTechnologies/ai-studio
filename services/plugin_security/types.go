// Package plugin_security provides security validation for plugins in both CE and ENT editions
package plugin_security

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
)

// Config holds configuration for plugin security service
type Config struct {
	// OCI configuration for signature verification
	OCIConfig *ociplugins.OCIConfig

	// Development mode settings
	AllowInternalNetworkAccess bool
}

// OCIReference is re-exported from ociplugins for convenience
type OCIReference = ociplugins.OCIReference

// VerificationResult contains the result of a signature verification
type VerificationResult struct {
	Verified  bool
	Method    string // "cosign", "bundle", "policy"
	PublicKey string
	Error     error
}

// GRPCValidationResult contains the result of GRPC host validation
type GRPCValidationResult struct {
	Allowed bool
	Host    string
	Reason  string
}

// SecurityContext provides context for security operations
type SecurityContext struct {
	Context        context.Context
	AllowOverrides bool // Allow development overrides
}
