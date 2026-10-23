//go:build !enterprise
// +build !enterprise

package plugin_security

import (
	"context"
	"log"
	"net"
	"os"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
)

// communityService is the Community Edition implementation.
// It provides basic internal-network blocking for plugin hosts (CIDR-based),
// while advanced controls (OCI signature verification, host whitelisting) are
// no-ops reserved for the Enterprise Edition.
type communityService struct {
	config        *Config
	warningLogged bool
}

// ValidateGRPCHost blocks hosts that resolve to internal/private networks.
// The check can be bypassed for local development via the
// AllowInternalNetworkAccess config option or the
// ALLOW_INTERNAL_NETWORK_ACCESS=true environment variable.
func (s *communityService) ValidateGRPCHost(host string) error {
	if s.IsInternalIP(host) && !s.allowInternalAccess() {
		return ErrInternalNetworkBlocked
	}
	return nil
}

// lookupIPFunc resolves hostnames to IPs. A variable so tests can stub DNS
// and stay hermetic.
var lookupIPFunc = net.LookupIP

// IsInternalIP reports whether a hostname or IP is internal (see
// netguard.HostIsInternal): hostnames are resolved and classified by every
// address they resolve to, and a resolution failure counts as internal.
func (s *communityService) IsInternalIP(host string) bool {
	return netguard.HostIsInternal(host, lookupIPFunc)
}

// allowInternalAccess reports whether the development bypass for internal
// network access is enabled via config or environment variable.
func (s *communityService) allowInternalAccess() bool {
	if s.config != nil && s.config.AllowInternalNetworkAccess {
		return true
	}
	return os.Getenv("ALLOW_INTERNAL_NETWORK_ACCESS") == "true"
}

// VerifySignature always returns nil in CE (no verification)
func (s *communityService) VerifySignature(ctx context.Context, ref *OCIReference, pubKeyID string) error {
	s.logSecurityWarning("OCI signature verification")
	return nil
}

// VerifiesSignatures reports that CE cannot verify signatures, so OCI
// clients record plugins as unverified (ociplugins.SignatureCapability).
func (s *communityService) VerifiesSignatures() bool {
	return false
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
