package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
)

// Plugin hosts (the host of a grpc:// plugin, the registry of an oci:// one)
// follow one rule on every path, Studio and gateway: unless
// ALLOW_INTERNAL_NETWORK_ACCESS=true, a host that is, or resolves to, an
// internal address is refused, and so is a hostname that does not resolve.
// Unlike the LLM upstream policy it is on by default, and
// LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS does not exempt a plugin host from it.

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
}

func (e *InternalPluginHostError) Error() string {
	return fmt.Sprintf("plugin host %q is an internal address, resolves to one, or does not resolve; set %s=true to allow internal plugin hosts and registries", e.Host, EnvAllowInternal)
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

// CheckPluginHost applies the plugin host rule to host (an IP literal or a
// hostname, without a port). No DNS lookup is made when internal hosts are
// allowed.
func CheckPluginHost(host string) error {
	if PluginInternalHostsAllowed() {
		return nil
	}
	if HostIsInternal(host, lookupIP) {
		return &InternalPluginHostError{Host: host}
	}
	return nil
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

// PluginHTTPTransport returns the transport for plugin registry requests: the
// upstream URL policy (scheme, LLM_UPSTREAM_ALLOWED_HOSTS) on every request,
// and every dial guarded by PluginDialControl. When internal plugin hosts are
// allowed, ALLOW_INTERNAL_NETWORK_ACCESS also lifts LLM_UPSTREAM_BLOCK_INTERNAL,
// so PluginDialControl is the only internal-range guard needed.
func PluginHTTPTransport() http.RoundTripper {
	dialer := NewDialer()
	dialer.Control = PluginDialControl
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, address)
	}
	return validatingTransport{inner: transport}
}
