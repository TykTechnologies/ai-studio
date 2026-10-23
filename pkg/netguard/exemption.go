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

type exemptionKey struct{}

// WithContext returns ctx carrying the exemption, for a client that serves
// many assets through one RoundTripper (the gateway's upstream pool): only
// PooledByExemption reads it, so a connection dialled under one asset's
// exemption is never reused for another asset's request.
func (e Exemption) WithContext(ctx context.Context) context.Context {
	if len(e.hosts) == 0 {
		return ctx
	}
	return context.WithValue(ctx, exemptionKey{}, e)
}

// ExemptionFromContext is the exemption WithContext stored in ctx, or the
// zero Exemption.
func ExemptionFromContext(ctx context.Context) Exemption {
	if ctx == nil {
		return Exemption{}
	}
	e, _ := ctx.Value(exemptionKey{}).(Exemption)
	return e
}

// Transport is HTTPTransport without the URL check: the dial-time check
// with the exemption, as an *http.Transport whose pool settings a caller
// may tune.
func (e Exemption) Transport() *http.Transport { return httpTransport(e) }

// pooledTransport sends each request through the pool of the exemption in
// its context.
type pooledTransport struct {
	build func(Exemption) http.RoundTripper
	pools sync.Map // host-set key -> http.RoundTripper
}

// PooledByExemption returns a RoundTripper for a client shared by many
// assets: each request goes through a transport of its own for the
// exemption its context carries (Exemption.WithContext), built by build and
// kept for reuse. Connections are pooled per exemption, so one dialled under
// an asset's exemption is never reused for a request without it, which a
// single pool would allow (the dial-time check runs once per connection).
func PooledByExemption(build func(Exemption) http.RoundTripper) http.RoundTripper {
	return &pooledTransport{build: build}
}

func (p *pooledTransport) pool(e Exemption) http.RoundTripper {
	key := strings.Join(e.hosts, ",")
	if rt, ok := p.pools.Load(key); ok {
		return rt.(http.RoundTripper)
	}
	rt, _ := p.pools.LoadOrStore(key, p.build(e))
	return rt.(http.RoundTripper)
}

func (p *pooledTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return p.pool(ExemptionFromContext(req.Context())).RoundTrip(req)
}

// CloseIdleConnections closes the idle connections of every pool.
func (p *pooledTransport) CloseIdleConnections() {
	p.pools.Range(func(_, v any) bool {
		if c, ok := v.(interface{ CloseIdleConnections() }); ok {
			c.CloseIdleConnections()
		}
		return true
	})
}

// CommandExemption is a plugin's "Allow internal host" exemption: the hosts
// of the URLs in its command (oci:// registry, grpc:// host), when allow.
func CommandExemption(allow bool, command string) Exemption {
	if !allow {
		return Exemption{}
	}
	var urls []string
	for _, part := range strings.Fields(command) {
		if strings.Contains(part, "://") && !strings.HasPrefix(part, "file://") {
			urls = append(urls, part)
		}
	}
	return ExemptionFor(true, urls...)
}
