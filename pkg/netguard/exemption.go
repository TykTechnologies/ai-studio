package netguard

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// Exemption is one managed asset's "Allow internal host" switch (an LLM, an
// embedder, a tool, a datasource, a guardrail, a plugin): with it on, the
// asset's own hosts may be internal addresses while blocking stays on for
// everything else, including another asset on the same host. It lifts only
// the internal-address check: LLM_UPSTREAM_ALLOWED_HOSTS still applies, and
// the environment settings (LLM_UPSTREAM_BLOCK_INTERNAL,
// LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS, ALLOW_INTERNAL_NETWORK_ACCESS) stay
// global overrides. The zero value exempts nothing.
//
// Every client an asset uses goes through one of its methods, so the rule
// is the same wherever the asset is called from, Studio or an edge.
type Exemption struct {
	hosts []string // lower-cased, sorted, unique
}

// ExemptionFor is the exemption of an asset whose switch is allow, for the
// hosts of its own targets: URLs ("https://ollama:11434/v1"), "host:port" or
// bare hosts. A target without a host adds nothing. allow false, or no
// targets, is the zero Exemption.
func ExemptionFor(allow bool, targets ...string) Exemption {
	if !allow {
		return Exemption{}
	}
	var hosts []string
	for _, t := range targets {
		if h := targetHost(t); h != "" {
			hosts = append(hosts, h)
		}
	}
	slices.Sort(hosts)
	return Exemption{hosts: slices.Compact(hosts)}
}

// targetHost is the lower-cased host of a URL, "host:port" or bare host.
func targetHost(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	if strings.Contains(t, "://") {
		u, err := url.Parse(t)
		if err != nil {
			return ""
		}
		return strings.ToLower(u.Hostname())
	}
	if h, _, err := net.SplitHostPort(t); err == nil {
		return strings.ToLower(strings.Trim(h, "[]"))
	}
	if strings.ContainsAny(t, "/?#@ ") {
		return ""
	}
	return strings.ToLower(strings.Trim(t, "[]"))
}

// Has reports whether host is one of the exemption's hosts.
func (e Exemption) Has(host string) bool {
	if len(e.hosts) == 0 {
		return false
	}
	_, found := slices.BinarySearch(e.hosts, strings.ToLower(strings.Trim(host, "[]")))
	return found
}

// Hosts returns the exempted hosts.
func (e Exemption) Hosts() []string { return slices.Clone(e.hosts) }

// ValidateURL is ValidateUpstreamURL with the exemption.
func (e Exemption) ValidateURL(u *url.URL) error {
	return validateUpstreamURL(u, true, e)
}

// exemptTransports holds one transport per exempted host set, so an asset's
// connections are pooled across calls.
var exemptTransports sync.Map // string -> http.RoundTripper

// HTTPTransport is ValidatingHTTPTransport with the exemption: the URL
// policy on every request (redirects included) and the dial-time check on
// the resolved address, except for the exemption's hosts. The zero
// Exemption shares one transport.
func (e Exemption) HTTPTransport() http.RoundTripper {
	key := strings.Join(e.hosts, ",")
	if rt, ok := exemptTransports.Load(key); ok {
		return rt.(http.RoundTripper)
	}
	rt, _ := exemptTransports.LoadOrStore(key, validatingTransport{inner: httpTransport(e), exempt: e})
	return rt.(http.RoundTripper)
}

// HTTPClient is an *http.Client on HTTPTransport.
func (e Exemption) HTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Transport: e.HTTPTransport(), Timeout: timeout}
}

// DialContext dials address with the dial-time check, except for the
// exemption's hosts and LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS. For clients
// that are not HTTP (a pgx pool, a Redis client).
func (e Exemption) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if internalHostExempt(host) || e.Has(host) {
		return newUnguardedDialer().DialContext(ctx, network, address)
	}
	return NewDialer().DialContext(ctx, network, address)
}
