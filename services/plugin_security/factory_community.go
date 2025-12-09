//go:build !enterprise
// +build !enterprise

package plugin_security

// newCommunityService creates a community stub service
// This is used when the enterprise build tag is not present
func newCommunityService(config *Config) Service {
	return &communityService{
		config: config,
	}
}
