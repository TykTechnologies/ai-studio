// Package netguard provides SSRF safeguards for outbound requests whose
// destination is derived from stored configuration (LLM upstream endpoints,
// OCI registries). Enforcement is opt-in via environment variables so that
// deployments proxying to internal LLMs (a first-class use case) keep working
// unchanged.
//
// Internal-range blocking is enforced at dial time, on the exact IP address
// being connected (via net.Dialer.Control). This closes the DNS-rebinding
// TOCTOU window: there is no separate validate-then-resolve step for an
// attacker-controlled DNS server to exploit — the address that is checked is
// the address that is dialed.
package netguard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

const (
	// EnvBlockInternal ("LLM_UPSTREAM_BLOCK_INTERNAL") enables blocking of
	// connections to internal/reserved IP ranges at dial time.
	EnvBlockInternal = "LLM_UPSTREAM_BLOCK_INTERNAL"

	// EnvAllowedHosts ("LLM_UPSTREAM_ALLOWED_HOSTS") is a comma-separated
	// allowlist of upstream hostnames. Entries starting with '.' match the
	// apex domain and any subdomain on a label boundary (".anthropic.com"
	// matches "anthropic.com" and "api.anthropic.com", never
	// "evil-anthropic.com"). When set, upstream hosts not on the list are
	// refused.
	EnvAllowedHosts = "LLM_UPSTREAM_ALLOWED_HOSTS"

	// EnvAllowInternal is the existing development flag that disables
	// internal-network restrictions platform-wide.
	EnvAllowInternal = "ALLOW_INTERNAL_NETWORK_ACCESS"

	// EnvAllowedInternalHosts ("LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS") names
	// upstream hosts that are exempt from internal-range blocking, using the
	// same matching as EnvAllowedHosts (".svc.cluster.local" matches every
	// in-cluster Service). It exists because the two existing controls cannot
	// express "reach this in-cluster model server, keep blocking everything
	// else internal": EnvAllowInternal disables the policy platform-wide, and
	// EnvAllowedHosts is a global allowlist, so using it to permit a cluster
	// host silently restricts every external provider not also listed.
	//
	// This is an additive exemption. It is consulted before EnvAllowedHosts and
	// widens the policy only for the hosts named, so an operator can run an
	// InferencePool-backed upstream alongside SaaS providers with egress
	// blocking left on.
	EnvAllowedInternalHosts = "LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS"
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

// blockInternalEnabled reports whether internal-range blocking is active.
func blockInternalEnabled() bool {
	return os.Getenv(EnvBlockInternal) == "true" && os.Getenv(EnvAllowInternal) != "true"
}

// ValidateUpstreamURL validates an outbound upstream URL against the
// configured policy:
//
//  1. Scheme must be http or https.
//  2. If LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS names the host, it is permitted
//     outright — this is the in-cluster exemption and it short-circuits the
//     rules below.
//  3. If LLM_UPSTREAM_ALLOWED_HOSTS is set, the hostname must match an entry
//     (exact, case-insensitive; ".suffix" entries match the apex and
//     subdomains on a label boundary).
//  4. If internal blocking is enabled and the host is an IP literal, it must
//     not be an internal address (an early, clear error; hostnames are
//     enforced at dial time by DialControl/HTTPTransport, which also covers
//     DNS-rebinding).
//
// This function performs no DNS resolution, so it is cheap on the request
// path and cannot be raced against a later lookup.
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

	// An explicitly exempted internal host is permitted whatever else is
	// configured, so an in-cluster upstream does not force the operator to
	// choose between blocking nothing and allowlisting every SaaS provider.
	if internalHostExempt(host) {
		return nil
	}

	if allowed := os.Getenv(EnvAllowedHosts); allowed != "" {
		if !hostMatchesAllowlist(host, allowed) {
			return fmt.Errorf("upstream host %q is not in %s", host, EnvAllowedHosts)
		}
		return nil
	}

	if blockInternalEnabled() {
		if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil && IsInternalIP(ip) {
			return fmt.Errorf("upstream host %q is an internal address — blocked by %s", host, EnvBlockInternal)
		}
	}

	return nil
}

// hostMatchesAllowlist checks host against a comma-separated allowlist.
// Entries beginning with '.' match the apex domain and any subdomain on a
// label boundary: ".example.com" matches "example.com" and "a.b.example.com",
// but never "evil-example.com" or "fooexample.com".
func hostMatchesAllowlist(host, allowlist string) bool {
	for _, entry := range strings.Split(allowlist, ",") {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if strings.HasPrefix(entry, ".") {
			apex := strings.TrimPrefix(entry, ".")
			if host == apex {
				return true
			}
			// Require the character before the suffix to be part of the
			// suffix's own leading dot — i.e. the host ends in ".apex" —
			// which is exactly a label boundary.
			if len(host) > len(entry) && strings.HasSuffix(host, entry) {
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

// DialControl is a net.Dialer Control hook that rejects connections to
// internal IP ranges when blocking is enabled. It runs after DNS resolution,
// once per connection attempt, on the exact address being dialed — so a DNS
// answer that changes between validation and connection cannot bypass it.
func DialControl(network, address string, _ syscall.RawConn) error {
	if !blockInternalEnabled() {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("refusing to dial unparseable address %q", address)
	}
	if IsInternalIP(ip) {
		return fmt.Errorf("connection to internal address %s blocked by %s", ip, EnvBlockInternal)
	}
	return nil
}

// internalHostExempt reports whether host is named by EnvAllowedInternalHosts.
func internalHostExempt(host string) bool {
	allowed := os.Getenv(EnvAllowedInternalHosts)
	if allowed == "" {
		return false
	}
	return hostMatchesAllowlist(strings.ToLower(host), allowed)
}

// NewDialer returns a net.Dialer whose connections are guarded by
// DialControl.
func NewDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   DialControl,
	}
}

// newUnguardedDialer returns a dialer with the same timeouts as NewDialer but
// without the internal-range Control hook. It is used only for hosts the
// operator has explicitly exempted via EnvAllowedInternalHosts.
func newUnguardedDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
}

// HTTPTransport returns an *http.Transport (based on
// http.DefaultTransport) whose dials are guarded by DialControl. Use it for
// any HTTP client whose destination comes from stored configuration.
//
// The exemption is applied here rather than in DialControl because Control runs
// after DNS resolution and sees only an IP, whereas the transport hands
// DialContext the hostname it set out to reach — the same name
// ValidateUpstreamURL checked. Keying the exemption on that name is what makes
// a host allowlist expressible at all, and it does not reopen the
// DNS-rebinding window the Control hook closes: an exempted name is one the
// operator has declared may resolve internally, not one an attacker chose.
func HTTPTransport() *http.Transport {
	guarded := NewDialer()
	unguarded := newUnguardedDialer()

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		if internalHostExempt(host) {
			return unguarded.DialContext(ctx, network, address)
		}
		return guarded.DialContext(ctx, network, address)
	}
	return transport
}

// validatingTransport applies ValidateUpstreamURL to every request before
// delegating to an inner (dial-guarded) transport.
type validatingTransport struct {
	inner http.RoundTripper
}

func (t validatingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := ValidateUpstreamURL(req.URL); err != nil {
		return nil, err
	}
	return t.inner.RoundTrip(req)
}

// ValidatingHTTPTransport returns a RoundTripper that applies the full URL
// policy (scheme + host allowlist) to every request — including redirects and
// any request issued by wrapping clients — on top of HTTPTransport's
// dial-time internal-IP guard. Use it for clients where per-request URL
// validation cannot be guaranteed at the call sites.
func ValidatingHTTPTransport() http.RoundTripper {
	return validatingTransport{inner: HTTPTransport()}
}
