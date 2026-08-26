// Package netguard provides SSRF safeguards for outbound requests whose
// destination is derived from stored configuration (LLM upstream endpoints,
// OCI registries). Enforcement is opt-in via environment variables so that
// deployments proxying to internal LLMs (a first-class use case) keep working
// unchanged.
package netguard

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

const (
	// EnvBlockInternal ("LLM_UPSTREAM_BLOCK_INTERNAL") enables blocking of
	// upstream hosts that resolve to internal/reserved IP ranges.
	EnvBlockInternal = "LLM_UPSTREAM_BLOCK_INTERNAL"

	// EnvAllowedHosts ("LLM_UPSTREAM_ALLOWED_HOSTS") is a comma-separated
	// allowlist of upstream hostnames. Entries starting with '.' match any
	// subdomain (".anthropic.com" matches "api.anthropic.com"). When set,
	// upstream hosts not on the list are refused.
	EnvAllowedHosts = "LLM_UPSTREAM_ALLOWED_HOSTS"

	// EnvAllowInternal is the existing development flag that disables
	// internal-network restrictions platform-wide.
	EnvAllowInternal = "ALLOW_INTERNAL_NETWORK_ACCESS"
)

// internalCIDRs covers loopback, RFC1918, link-local (incl. cloud metadata
// endpoints), unique-local and unspecified addresses.
var internalCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"0.0.0.0/32",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
		"::/128",
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}()

// IsInternalIP reports whether ip belongs to a private, loopback, link-local,
// unique-local, or unspecified range.
func IsInternalIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, n := range internalCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateUpstreamURL validates an outbound upstream URL against the
// configured policy:
//
//  1. Scheme must be http or https.
//  2. If LLM_UPSTREAM_ALLOWED_HOSTS is set, the hostname must match an entry
//     (exact, case-insensitive; ".suffix" entries match subdomains).
//  3. If LLM_UPSTREAM_BLOCK_INTERNAL=true (and ALLOW_INTERNAL_NETWORK_ACCESS
//     is not "true"), the host must not resolve to an internal IP range.
//
// With neither variable set only the scheme check applies, preserving
// existing behavior for deployments that proxy to internal LLMs.
func ValidateUpstreamURL(u *url.URL) error {
	if u == nil {
		return fmt.Errorf("upstream URL is nil")
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("upstream URL scheme %q is not allowed (must be http or https)", u.Scheme)
	}

	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("upstream URL %q has no host", u.String())
	}

	if allowed := os.Getenv(EnvAllowedHosts); allowed != "" {
		if !hostMatchesAllowlist(host, allowed) {
			return fmt.Errorf("upstream host %q is not in %s", host, EnvAllowedHosts)
		}
		return nil
	}

	if os.Getenv(EnvBlockInternal) == "true" && os.Getenv(EnvAllowInternal) != "true" {
		ips, err := resolveHost(host)
		if err != nil {
			return fmt.Errorf("cannot resolve upstream host %q: %w", host, err)
		}
		for _, ip := range ips {
			if IsInternalIP(ip) {
				return fmt.Errorf("upstream host %q resolves to internal address %s — blocked by %s", host, ip, EnvBlockInternal)
			}
		}
	}

	return nil
}

// hostMatchesAllowlist checks host against a comma-separated allowlist.
// Entries beginning with '.' match any subdomain on a label boundary.
func hostMatchesAllowlist(host, allowlist string) bool {
	for _, entry := range strings.Split(allowlist, ",") {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if strings.HasPrefix(entry, ".") {
			if strings.HasSuffix(host, entry) || host == strings.TrimPrefix(entry, ".") {
				return true
			}
			continue
		}
		if host == entry {
			return true
		}
	}
	return false
}

// resolveHost returns the IPs for a hostname; IP literals are returned
// directly without a DNS lookup.
func resolveHost(host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	return net.LookupIP(host)
}
