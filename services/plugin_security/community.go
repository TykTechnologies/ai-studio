//go:build !enterprise
// +build !enterprise

package plugin_security

import (
	"context"
	"log"
)

// communityService is the Community Edition stub implementation
// All security checks are no-ops, allowing plugin operations to proceed
// This maintains plugin functionality while reserving advanced security for ENT
type communityService struct {
	config        *Config
	warningLogged bool
}

// ValidateGRPCHost always returns nil in CE (allows all hosts)
func (s *communityService) ValidateGRPCHost(host string) error {
	s.logSecurityWarning("GRPC host whitelisting")
	return nil
}

// IsInternalIP always returns false in CE
func (s *communityService) IsInternalIP(host string) bool {
	return false
}

// VerifySignature always returns nil in CE (no verification)
func (s *communityService) VerifySignature(ctx context.Context, ref *OCIReference, pubKeyID string) error {
	s.logSecurityWarning("OCI signature verification")
	return nil
}

// VerifyBundle always returns nil in CE (no verification)
func (s *communityService) VerifyBundle(ctx context.Context, ref *OCIReference, issuer, subject string) error {
	s.logSecurityWarning("OCI bundle verification")
	return nil
}

// VerifyWithPolicy always returns nil in CE (no verification)
func (s *communityService) VerifyWithPolicy(ctx context.Context, ref *OCIReference, policyPath string) error {
	s.logSecurityWarning("OCI policy verification")
	return nil
}

// GetPublicKeyPath always returns empty string in CE
func (s *communityService) GetPublicKeyPath(pubKeyID string) (string, error) {
	return "", nil
}

// ValidatePublicKey always returns nil in CE
func (s *communityService) ValidatePublicKey(keyPath string) error {
	return nil
}

// LoadPublicKeysFromDirectory always returns empty slice in CE
func (s *communityService) LoadPublicKeysFromDirectory(dir string) ([]string, error) {
	return []string{}, nil
}

// logSecurityWarning logs a warning about reduced security in CE (only once)
func (s *communityService) logSecurityWarning(feature string) {
	if !s.warningLogged {
		log.Printf("⚠️  Plugin Security: %s is disabled in Community Edition. Upgrade to Enterprise Edition for advanced plugin security features.", feature)
		s.warningLogged = true
	}
}
