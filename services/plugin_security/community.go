//go:build !enterprise
// +build !enterprise

package plugin_security

import (
	"context"
	"log"
	"net"
	"os"
	"strings"
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

// IsInternalIP checks if a hostname/IP is internal/private using CIDR
// validation. Hostnames are resolved via DNS and classified by their resolved
// addresses; resolution failures fail closed (treated as internal) so an
// unresolvable host cannot slip past the block.
func (s *communityService) IsInternalIP(host string) bool {
	// Handle localhost variations: the bare name and the reserved
	// *.localhost domain (RFC 6761), which resolve to loopback.
	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") || lowerHost == "::1" {
		return true
	}

	// IP literal: classify directly
	if ip := net.ParseIP(host); ip != nil {
		return ipIsPrivate(ip)
	}

	// Hostname: resolve and classify every returned address. Any internal
	// address taints the host, and a resolution failure fails closed —
	// otherwise a name that resolves to a private IP (or refuses to resolve
	// for the validator but resolves for the dialer) would bypass the block.
	ips, err := lookupIPFunc(host)
	if err != nil {
		return true
	}
	for _, ip := range ips {
		if ipIsPrivate(ip) {
			return true
		}
	}

	return false
}

// ipIsPrivate reports whether ip falls in a private or special-use range.
func ipIsPrivate(ip net.IP) bool {
	// Private and special-use IP CIDR ranges
	privateCIDRs := []string{
		"10.0.0.0/8",     // Private Class A
		"172.16.0.0/12",  // Private Class B
		"192.168.0.0/16", // Private Class C
		"127.0.0.0/8",    // IPv4 loopback
		"169.254.0.0/16", // IPv4 link-local (incl. cloud metadata endpoints)
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique local addresses
		"fe80::/10",      // IPv6 link-local
	}

	for _, cidrStr := range privateCIDRs {
		_, cidr, err := net.ParseCIDR(cidrStr)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}

	return false
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
