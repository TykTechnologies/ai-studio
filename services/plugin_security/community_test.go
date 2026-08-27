//go:build !enterprise
// +build !enterprise

package plugin_security

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

// stubResolver replaces DNS resolution for the duration of a test so
// hostname-based checks are hermetic (no live lookups, no flakes).
func stubResolver(t *testing.T, table map[string][]string) {
	t.Helper()
	orig := lookupIPFunc
	lookupIPFunc = func(host string) ([]net.IP, error) {
		addrs, ok := table[host]
		if !ok {
			return nil, fmt.Errorf("stub resolver: no such host %q", host)
		}
		ips := make([]net.IP, 0, len(addrs))
		for _, a := range addrs {
			ips = append(ips, net.ParseIP(a))
		}
		return ips, nil
	}
	t.Cleanup(func() { lookupIPFunc = orig })
}

func TestCommunityIsInternalIP(t *testing.T) {
	svc := NewService(&Config{})
	stubResolver(t, map[string][]string{
		"registry.example.com": {"93.184.216.34"},
	})

	internal := []string{
		"127.0.0.1",
		"127.255.255.255",
		"localhost",
		"LOCALHOST",
		"10.0.0.5",
		"172.16.0.1",
		"172.31.255.254",
		"192.168.1.1",
		"169.254.169.254", // cloud metadata endpoint
		"::1",
		"fe80::1",
		"fc00::1",
	}
	for _, host := range internal {
		if !svc.IsInternalIP(host) {
			t.Errorf("expected %q to be detected as internal", host)
		}
	}

	external := []string{
		"8.8.8.8",
		"1.1.1.1",
		"172.32.0.1", // just outside 172.16.0.0/12
		"registry.example.com",
		"2001:4860:4860::8888",
	}
	for _, host := range external {
		if svc.IsInternalIP(host) {
			t.Errorf("expected %q to be treated as external", host)
		}
	}
}

func TestCommunityValidateGRPCHost_BlocksInternal(t *testing.T) {
	svc := NewService(&Config{AllowInternalNetworkAccess: false})

	for _, host := range []string{"127.0.0.1", "localhost", "10.1.2.3", "192.168.0.10", "169.254.169.254"} {
		err := svc.ValidateGRPCHost(host)
		if err == nil {
			t.Errorf("expected internal host %q to be blocked in CE", host)
			continue
		}
		if !errors.Is(err, ErrInternalNetworkBlocked) {
			t.Errorf("expected ErrInternalNetworkBlocked for %q, got: %v", host, err)
		}
	}
}

func TestCommunityValidateGRPCHost_AllowsExternal(t *testing.T) {
	svc := NewService(&Config{AllowInternalNetworkAccess: false})
	stubResolver(t, map[string][]string{
		"registry.example.com": {"93.184.216.34"},
	})

	for _, host := range []string{"8.8.8.8", "registry.example.com"} {
		if err := svc.ValidateGRPCHost(host); err != nil {
			t.Errorf("expected external host %q to be allowed, got: %v", host, err)
		}
	}
}

// TestCommunityIsInternalIP_HostnameResolution covers the SSRF bypass from
// review: a hostname resolving to a private IP must be classified internal,
// and DNS failures must fail closed.
func TestCommunityIsInternalIP_HostnameResolution(t *testing.T) {
	svc := NewService(&Config{})
	stubResolver(t, map[string][]string{
		"internal.service":           {"10.0.0.1"},
		"metadata.internal":          {"169.254.169.254"},
		"dual.example.com":           {"93.184.216.34", "192.168.1.5"}, // any internal IP taints the host
		"external.example.com":       {"93.184.216.34"},
		"a-service-at-localhost.com": {"93.184.216.34"}, // name contains 'localhost' but is external
	})

	internal := []string{
		"internal.service",
		"metadata.internal",
		"dual.example.com",
		"unresolvable.example.com", // resolution failure fails closed
		"foo.localhost",            // *.localhost is reserved for loopback
	}
	for _, host := range internal {
		if !svc.IsInternalIP(host) {
			t.Errorf("expected %q to be classified internal", host)
		}
	}

	external := []string{
		"external.example.com",
		"a-service-at-localhost.com",
	}
	for _, host := range external {
		if svc.IsInternalIP(host) {
			t.Errorf("expected %q to be classified external", host)
		}
	}
}

func TestCommunityValidateGRPCHost_BlocksResolvedInternalHostname(t *testing.T) {
	svc := NewService(&Config{AllowInternalNetworkAccess: false})
	stubResolver(t, map[string][]string{
		"internal.service": {"10.0.0.1"},
	})

	err := svc.ValidateGRPCHost("internal.service")
	if !errors.Is(err, ErrInternalNetworkBlocked) {
		t.Errorf("expected hostname resolving to a private IP to be blocked, got: %v", err)
	}
}

func TestCommunityValidateGRPCHost_DevelopmentBypass(t *testing.T) {
	svc := NewService(&Config{AllowInternalNetworkAccess: true})

	for _, host := range []string{"127.0.0.1", "localhost", "10.1.2.3"} {
		if err := svc.ValidateGRPCHost(host); err != nil {
			t.Errorf("expected internal host %q to be allowed with bypass enabled, got: %v", host, err)
		}
	}
}

func TestCommunityValidateGRPCHost_EnvBypass(t *testing.T) {
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	svc := NewService(&Config{AllowInternalNetworkAccess: false})

	if err := svc.ValidateGRPCHost("127.0.0.1"); err != nil {
		t.Errorf("expected env bypass to allow internal host, got: %v", err)
	}
}

func TestCommunityValidateGRPCHost_NilConfig(t *testing.T) {
	// A service created without config must fail closed on internal hosts,
	// not panic.
	svc := NewService(nil)

	if err := svc.ValidateGRPCHost("10.0.0.1"); err == nil {
		t.Error("expected internal host to be blocked with nil config")
	}
	if err := svc.ValidateGRPCHost("8.8.8.8"); err != nil {
		t.Errorf("expected external host to be allowed with nil config, got: %v", err)
	}
}
