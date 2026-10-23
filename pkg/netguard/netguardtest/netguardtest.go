// Package netguardtest stubs the DNS lookups of the plugin host checks
// (netguard.LookupIP), so tests that name registries or plugin hosts stay
// hermetic.
package netguardtest

import (
	"fmt"
	"net"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
)

// PublicIP is the address unlisted hostnames resolve to: TEST-NET-3, which
// the plugin host rule treats as public.
const PublicIP = "203.0.113.10"

// ResolveAllPublic makes every hostname resolve to PublicIP until restore is
// called. For a TestMain, where tests use made-up registry names that must
// pass the plugin host rule; tests that need a name to resolve internally use
// StubLookupIP.
func ResolveAllPublic() (restore func()) {
	orig := netguard.LookupIP
	netguard.LookupIP = func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP(PublicIP)}, nil
	}
	return func() { netguard.LookupIP = orig }
}

// StubLookupIP makes each hostname in hosts resolve to its address ("" for a
// name that does not resolve) and every other hostname to PublicIP, for the
// rest of the test.
func StubLookupIP(t testing.TB, hosts map[string]string) {
	t.Helper()
	orig := netguard.LookupIP
	netguard.LookupIP = func(host string) ([]net.IP, error) {
		ip, ok := hosts[host]
		if !ok {
			return []net.IP{net.ParseIP(PublicIP)}, nil
		}
		if ip == "" {
			return nil, fmt.Errorf("lookup %s: no such host", host)
		}
		return []net.IP{net.ParseIP(ip)}, nil
	}
	t.Cleanup(func() { netguard.LookupIP = orig })
}

// AllowInternal turns internal-range blocking off (LLM_UPSTREAM_BLOCK_INTERNAL
// =false) for the rest of the test, for tests whose upstreams are httptest
// servers on 127.0.0.1. Blocking is on by default.
func AllowInternal(t testing.TB) {
	t.Helper()
	prev := netguard.SetBlockInternal(false)
	t.Cleanup(func() { netguard.SetBlockInternal(prev) })
}

// AllowInternalForPackage turns internal-range blocking off until restore is
// called. For a TestMain whose package runs upstreams on 127.0.0.1.
func AllowInternalForPackage() (restore func()) {
	prev := netguard.SetBlockInternal(false)
	return func() { netguard.SetBlockInternal(prev) }
}

// BlockInternal turns internal-range blocking on for the rest of the test,
// for a test of the default in a package whose other tests turn it off.
func BlockInternal(t testing.TB) {
	t.Helper()
	prev := netguard.SetBlockInternal(true)
	t.Cleanup(func() { netguard.SetBlockInternal(prev) })
}
