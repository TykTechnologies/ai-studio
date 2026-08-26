//go:build !enterprise
// +build !enterprise

package plugin_security

import (
	"errors"
	"testing"
)

func TestCommunityIsInternalIP(t *testing.T) {
	svc := NewService(&Config{})

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

	for _, host := range []string{"8.8.8.8", "registry.example.com"} {
		if err := svc.ValidateGRPCHost(host); err != nil {
			t.Errorf("expected external host %q to be allowed, got: %v", host, err)
		}
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
