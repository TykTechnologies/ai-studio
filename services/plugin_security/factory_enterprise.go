//go:build enterprise
// +build enterprise

package plugin_security

// newCommunityService is not used in enterprise builds
// The enterprise implementation is registered via RegisterEnterpriseFactory
// This file exists to satisfy the build system
func newCommunityService(config *Config) Service {
	// This should never be called in enterprise builds
	// The enterprise factory is registered and used instead
	panic("community service should not be created in enterprise build")
}
