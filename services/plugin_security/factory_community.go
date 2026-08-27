//go:build !enterprise
// +build !enterprise

package plugin_security

import "log"

// newCommunityService creates a community service
// This is used when the enterprise build tag is not present
func newCommunityService(config *Config) Service {
	// OCI signature verification is not available in CE, so a configured
	// RequireSignature flag would otherwise silently do nothing. Make that
	// loud at startup so operators are not lulled into a false sense of
	// security.
	if config != nil && config.OCIConfig != nil && config.OCIConfig.RequireSignature {
		log.Printf("🔒 SECURITY WARNING: OCI RequireSignature is enabled, but signature verification is NOT available in Community Edition. Plugin signatures will NOT be verified. Upgrade to Enterprise Edition for signature enforcement.")
	}

	return &communityService{
		config: config,
	}
}
