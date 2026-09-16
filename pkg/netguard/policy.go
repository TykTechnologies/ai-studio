package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ErrURLPolicy is the default sentinel wrapped by every URLPolicy error.
// Features that need their own sentinel set PolicyOptions.Err.
var ErrURLPolicy = errors.New("url is not allowed by the outbound URL policy")

// ErrPolicyDial is returned from a policy-built HTTP client when the address
// a hostname resolved to is blocked. It is permanent: retrying will not
// change the answer.
var ErrPolicyDial = errors.New("destination resolved to a blocked internal address")

// PolicyOptions configures a URLPolicy.
type PolicyOptions struct {
	// AllowInternal permits loopback, RFC1918, link-local and other internal
	// destinations for every URL the policy checks.
	AllowInternal bool
	// AllowedHosts, when non-empty, restricts destinations to these hosts
	// (exact, or ".suffix" for a domain and its subdomains). A host named
	// exactly here may also be internal.
	AllowedHosts []string
	// DeniedHosts always rejects and wins over AllowedHosts.
	DeniedHosts []string
	// ExemptHosts are exact hosts permitted to be internal without opening
	// the policy for anything else (a per-object exemption, e.g. one
	// administrator-approved Dashboard on a private address).
	ExemptHosts []string
	// Err is the sentinel wrapped in every validation error. Defaults to
	// ErrURLPolicy.
	Err error
	// InternalHint is appended to "internal address" rejections so the
	// message names the switch that would allow it.
	InternalHint string
}

// URLPolicy decides where an outbound request derived from stored
// configuration may go. It is always on, independent of the LLM_UPSTREAM_*
// environment gating in this package, because the URLs it guards are typed
// in by users and are an exfiltration vector.
//
// Two layers: Validate checks the URL as written (scheme, userinfo, host, IP
// literals, allow/deny lists) whenever a URL is stored and again before every
// use; the client built by NewHTTPClient re-checks the resolved IP at connect
// time so a hostname that resolves to an internal address (or is re-pointed
// there later) is still blocked.
type URLPolicy struct {
	allowInternal bool
	allowed       []string
	denied        []string
	exempt        []string
	err           error
	hint          string
}

// NewURLPolicy builds a policy from options.
func NewURLPolicy(o PolicyOptions) *URLPolicy {
	p := &URLPolicy{
		allowInternal: o.AllowInternal,
		allowed:       lowerAll(o.AllowedHosts),
		denied:        lowerAll(o.DeniedHosts),
		exempt:        lowerAll(o.ExemptHosts),
		err:           o.Err,
		hint:          o.InternalHint,
	}
	if p.err == nil {
		p.err = ErrURLPolicy
	}
	return p
}

// WithExemptHosts returns a copy of the policy that additionally permits the
// named hosts to be internal.
func (p *URLPolicy) WithExemptHosts(hosts ...string) *URLPolicy {
	c := *p
	c.exempt = append(append([]string{}, p.exempt...), lowerAll(hosts)...)
	return &c
}

func lowerAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.ToLower(strings.TrimSpace(v))
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (p *URLPolicy) fail(format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s", p.err, fmt.Sprintf(format, args...))
}

func (p *URLPolicy) internalMsg(host string, what string) error {
	if p.hint != "" {
		return p.fail("%s is an internal %s; %s", host, what, p.hint)
	}
	return p.fail("%s is an internal %s and internal destinations are not allowed", host, what)
}

// Validate parses and checks a URL. The returned URL is normalised.
func (p *URLPolicy) Validate(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, p.fail("url is required")
	}
	if len(raw) > 2048 {
		return nil, p.fail("url is longer than 2048 characters")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, p.fail("url is not valid: %v", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, p.fail("scheme must be http or https")
	}
	if u.User != nil {
		return nil, p.fail("credentials in the url are not allowed; use a custom header")
	}
	if u.Fragment != "" {
		return nil, p.fail("url must not contain a fragment")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return nil, p.fail("url has no host")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, p.fail("invalid port %q", port)
		}
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		if IsInternalIP(ip) && !p.internalPermitted(host) {
			return nil, p.internalMsg(host, "address")
		}
	} else if IsInternalHostname(host) && !p.internalPermitted(host) {
		return nil, p.internalMsg(host, "hostname")
	}
	if err := p.HostAllowed(host); err != nil {
		return nil, err
	}
	u.Scheme = scheme
	return u, nil
}

// IsInternalHostname reports whether a hostname names a local or
// cluster-internal destination by convention.
func IsInternalHostname(host string) bool {
	host = strings.ToLower(host)
	return host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal")
}

// HostAllowed applies the deny list, then the allow list.
func (p *URLPolicy) HostAllowed(host string) error {
	host = strings.ToLower(host)
	for _, d := range p.denied {
		if HostMatches(host, d) {
			return p.fail("%s is on the denied hosts list", host)
		}
	}
	if len(p.allowed) == 0 {
		return nil
	}
	for _, a := range p.allowed {
		if HostMatches(host, a) {
			return nil
		}
	}
	return p.fail("%s is not on the allowed hosts list", host)
}

// ExplicitlyAllowed reports whether the operator named this exact host in the
// allow list or the exemptions; such hosts may be internal.
func (p *URLPolicy) ExplicitlyAllowed(host string) bool {
	host = strings.ToLower(host)
	for _, a := range p.allowed {
		if a == host {
			return true
		}
	}
	for _, e := range p.exempt {
		if e == host {
			return true
		}
	}
	return false
}

func (p *URLPolicy) internalPermitted(host string) bool {
	return p.allowInternal || p.ExplicitlyAllowed(host)
}

// HostMatches compares a lower-cased host with a pattern: ".example.com"
// matches example.com and any subdomain; anything else is an exact match.
func HostMatches(host, pattern string) bool {
	if strings.HasPrefix(pattern, ".") {
		return host == strings.TrimPrefix(pattern, ".") || strings.HasSuffix(host, pattern)
	}
	return host == pattern
}

// NewHTTPClient returns a client whose dialer rejects internal addresses at
// connect time (unless permitted), which never uses a proxy (a proxy would
// bypass the dial guard) and never follows redirects: a 3xx would send the
// request somewhere the administrator did not approve.
func (p *URLPolicy) NewHTTPClient(timeout time.Duration) *http.Client {
	base, _ := http.DefaultTransport.(*http.Transport)
	transport := base.Clone()
	transport.Proxy = nil
	transport.MaxIdleConnsPerHost = 8
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		exempt := p.internalPermitted(strings.ToLower(host))
		d := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
			Control: func(network, address string, _ syscall.RawConn) error {
				if exempt {
					return nil
				}
				ipStr, _, err := net.SplitHostPort(address)
				if err != nil {
					ipStr = address
				}
				if ip := net.ParseIP(ipStr); ip != nil && IsInternalIP(ip) {
					return ErrPolicyDial
				}
				return nil
			},
		}
		return d.DialContext(ctx, network, addr)
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
