package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
)

// Plugin hosts (the host of a grpc:// plugin, the registry of an oci:// one)
// follow one rule on every path, Studio and gateway: unless
// ALLOW_INTERNAL_NETWORK_ACCESS=true, a host that is, or resolves to, an
// internal address is refused, and so is a hostname that does not resolve.
// Unlike the LLM upstream policy it is on by default. A host named in
// LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS is exempt, the same per-host opt-out
// LLM upstreams use, so one internal registry does not need the
// platform-wide flag.

// LookupIP resolves hostnames for the plugin host checks. A variable so tests
// in any package can stub DNS (the plugin security services resolve through
// it too).
var LookupIP = net.LookupIP

// ErrInternalPluginHost matches every InternalPluginHostError (errors.Is).
var ErrInternalPluginHost = errors.New("plugin host is an internal address")

// InternalPluginHostError names the refused host and the setting that allows
// it, so an operator can act on the log line or API error alone.
type InternalPluginHostError struct {
	Host string
	// Unresolved is set when Host is a name whose lookup failed, rather
	// than one that resolved to an internal address.
	Unresolved bool
}

func (e *InternalPluginHostError) Error() string {
	return fmt.Sprintf("plugin host %q is an internal address, resolves to one, or does not resolve; add it to %s to allow this host, or set %s=true to allow every internal host", e.Host, EnvAllowedInternalHosts, EnvAllowInternal)
}

// Is makes errors.Is(err, ErrInternalPluginHost) true.
func (e *InternalPluginHostError) Is(target error) bool {
	return target == ErrInternalPluginHost
}

// PluginInternalHostsAllowed reports whether ALLOW_INTERNAL_NETWORK_ACCESS
// lifts the plugin host rule.
func PluginInternalHostsAllowed() bool {
	return os.Getenv(EnvAllowInternal) == "true"
}

// PluginHostExempt reports whether host is named in
// LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS (exact, or a ".suffix" entry), which
// exempts it from the plugin host rule.
func PluginHostExempt(host string) bool {
	return internalHostExempt(strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), "."))
}

// CheckPluginHost applies the plugin host rule to host (an IP literal or a
// hostname, without a port). No DNS lookup is made when internal hosts are
// allowed or the host is exempt.
func CheckPluginHost(host string) error {
	return CheckPluginHostWith(Exemption{}, host)
}

// CheckPluginHostWith is CheckPluginHost with a plugin's own "Allow internal
// host" exemption (CommandExemption): its hosts pass too.
func CheckPluginHostWith(exempt Exemption, host string) error {
	if PluginInternalHostsAllowed() || PluginHostExempt(host) || exempt.Has(host) {
		return nil
	}
	lower := strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
	if IsInternalHost(lower) {
		return &InternalPluginHostError{Host: host}
	}
	if ip := net.ParseIP(lower); ip != nil {
		if IsInternalIP(ip) {
			return &InternalPluginHostError{Host: host}
		}
		return nil
	}
	ips, err := lookupIP(lower)
	if err != nil {
		return &InternalPluginHostError{Host: host, Unresolved: true}
	}
	for _, ip := range ips {
		if IsInternalIP(ip) {
			return &InternalPluginHostError{Host: host}
		}
	}
	return nil
}

// IsUnresolvedPluginHost reports whether err refuses a plugin host only
// because its name did not resolve (as opposed to resolving internally).
func IsUnresolvedPluginHost(err error) bool {
	var hostErr *InternalPluginHostError
	return errors.As(err, &hostErr) && hostErr.Unresolved
}

// lookupIP calls LookupIP at call time, so a stub installed after a caller
// captured the function still applies.
func lookupIP(host string) ([]net.IP, error) {
	return LookupIP(host)
}

// PluginDialControl is a net.Dialer Control hook applying the plugin host
// rule to the exact address being dialled, after DNS resolution, so a DNS
// answer that changes after CheckPluginHost cannot reach an internal address.
func PluginDialControl(network, address string, _ syscall.RawConn) error {
	if PluginInternalHostsAllowed() {
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
		return &InternalPluginHostError{Host: ip.String()}
	}
	return nil
}

// proxyFromEnvironment picks the proxy for a plugin transport request; a
// variable so tests can set a proxy (http.ProxyFromEnvironment reads the
// environment once per process).
var proxyFromEnvironment = http.ProxyFromEnvironment

// PluginHTTPTransport returns the transport for OCI client requests
// (registry, signature and token-server connections): the upstream URL policy
// (scheme, LLM_UPSTREAM_ALLOWED_HOSTS) on every request, and every dial
// guarded by PluginDialControl whatever LLM_UPSTREAM_BLOCK_INTERNAL says. A
// host named in LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS is dialled unguarded, as
// HTTPTransport does, keyed on the name the request set out to reach. When
// internal plugin hosts are allowed, ALLOW_INTERNAL_NETWORK_ACCESS also lifts
// LLM_UPSTREAM_BLOCK_INTERNAL, so PluginDialControl is the only internal-range
// guard needed.
//
// Behind a proxy (HTTPS_PROXY / HTTP_PROXY, NO_PROXY honoured) the dial goes
// to the proxy, which is operator configuration, not a plugin host: the proxy
// endpoint is dialled unguarded, and the request's target is checked by name
// (CheckPluginHost) before it is handed to the proxy. Connect-time rebinding
// protection does not apply to the target then; the proxy is responsible for
// egress. A target that bypasses the proxy (NO_PROXY) is dial-guarded.
func PluginHTTPTransport() http.RoundTripper {
	return PluginHTTPTransportFor(Exemption{})
}

// PluginHTTPTransportFor is PluginHTTPTransport with a plugin's own "Allow
// internal host" exemption (CommandExemption): its registry and signature
// hosts are dialled unguarded too. Use one per exemption, so connections
// are never shared across plugins with different switches.
func PluginHTTPTransportFor(exempt Exemption) http.RoundTripper {
	guarded := NewDialer()
	guarded.Control = PluginDialControl
	unguarded := newUnguardedDialer()
	var proxies sync.Map // "host:port" of each proxy endpoint in use
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		proxyURL, err := proxyFromEnvironment(req)
		if err != nil || proxyURL == nil {
			return proxyURL, err
		}
		if err := CheckPluginHostWith(exempt, req.URL.Hostname()); err != nil {
			return nil, err
		}
		proxies.Store(proxyAddress(proxyURL), true)
		return proxyURL, nil
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		if _, isProxy := proxies.Load(address); isProxy || PluginHostExempt(host) || exempt.Has(host) {
			return unguarded.DialContext(ctx, network, address)
		}
		return guarded.DialContext(ctx, network, address)
	}
	return validatingTransport{inner: transport, pluginHosts: true}
}

// proxyAddress is the "host:port" the transport dials for proxyURL.
func proxyAddress(proxyURL *url.URL) string {
	port := proxyURL.Port()
	if port == "" {
		switch proxyURL.Scheme {
		case "https":
			port = "443"
		case "socks5", "socks5h":
			port = "1080"
		default:
			port = "80"
		}
	}
	return net.JoinHostPort(proxyURL.Hostname(), port)
}
