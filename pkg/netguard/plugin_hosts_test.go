package netguard

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func stubPluginLookup(t *testing.T, hosts map[string]string) {
	t.Helper()
	orig := LookupIP
	LookupIP = func(host string) ([]net.IP, error) {
		if ip, ok := hosts[host]; ok {
			return []net.IP{net.ParseIP(ip)}, nil
		}
		return nil, fmt.Errorf("lookup %s: no such host", host)
	}
	t.Cleanup(func() { LookupIP = orig })
}

// The plugin host rule refuses internal literals (legacy spellings and the
// "this network" and unspecified ranges included), names that resolve to an
// internal address and names that do not resolve, unless
// ALLOW_INTERNAL_NETWORK_ACCESS=true; the error names the host and the
// setting (midsommar-clean-c1g.32).
func TestCheckPluginHost(t *testing.T) {
	stubPluginLookup(t, map[string]string{
		"registry.corp.internal": "10.0.0.5",
		"registry.example.com":   "203.0.113.10",
	})
	t.Setenv(EnvAllowInternal, "")
	for host, refused := range map[string]bool{
		"10.0.0.5":               true,
		"0.0.0.0":                true,
		"::":                     true,
		"0x7f000001":             true, // 127.0.0.1
		"localhost":              true,
		"registry.corp.internal": true,
		"unresolvable.example":   true,
		"registry.example.com":   false,
		"203.0.113.10":           false,
	} {
		err := CheckPluginHost(host)
		if refused != (err != nil) {
			t.Errorf("CheckPluginHost(%q) = %v, want refused %v", host, err, refused)
			continue
		}
		if err == nil {
			continue
		}
		if !errors.Is(err, ErrInternalPluginHost) {
			t.Errorf("CheckPluginHost(%q) = %v, want ErrInternalPluginHost", host, err)
		}
		var hostErr *InternalPluginHostError
		if !errors.As(err, &hostErr) || hostErr.Host != host {
			t.Errorf("CheckPluginHost(%q) names host %v", host, hostErr)
		}
	}

	// The per-host opt-out exempts the hosts it names, and only those.
	t.Setenv(EnvAllowedInternalHosts, "registry.corp.internal,.svc.cluster.local,10.0.0.5")
	for host, refused := range map[string]bool{
		"registry.corp.internal":        false,
		"REGISTRY.corp.internal.":       false,
		"zot.plugins.svc.cluster.local": false,
		"10.0.0.5":                      false,
		"10.0.0.6":                      true,
		"other.corp.internal":           true,
	} {
		if err := CheckPluginHost(host); refused != (err != nil) {
			t.Errorf("with %s set, CheckPluginHost(%q) = %v, want refused %v", EnvAllowedInternalHosts, host, err, refused)
		}
	}
	t.Setenv(EnvAllowedInternalHosts, "")

	t.Setenv(EnvAllowInternal, "true")
	for _, host := range []string{"10.0.0.5", "registry.corp.internal", "unresolvable.example"} {
		if err := CheckPluginHost(host); err != nil {
			t.Errorf("with internal hosts allowed, CheckPluginHost(%q) = %v", host, err)
		}
	}
}

// PluginHTTPTransport refuses the dial itself when the address is internal,
// whatever name led there, and connects when internal hosts are allowed.
func TestPluginHTTPTransport_RefusesInternalDial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(server.Close)
	client := &http.Client{Transport: PluginHTTPTransport()}

	t.Setenv(EnvAllowInternal, "")
	_, err := client.Get(server.URL)
	if !errors.Is(err, ErrInternalPluginHost) {
		t.Fatalf("dial to %s = %v, want ErrInternalPluginHost", server.URL, err)
	}
	// A name that resolves to 127.0.0.1 is refused at dial time too (OCI
	// registry, signature and token-realm fetches use this transport).
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if _, err := client.Get("http://localhost:" + port); !errors.Is(err, ErrInternalPluginHost) {
		t.Fatalf("dial to localhost = %v, want ErrInternalPluginHost", err)
	}

	// Guarded whatever LLM_UPSTREAM_BLOCK_INTERNAL says.
	setBlockInternal(t, "false")
	if _, err := client.Get(server.URL); !errors.Is(err, ErrInternalPluginHost) {
		t.Fatalf("with %s=false, dial = %v, want ErrInternalPluginHost", EnvBlockInternal, err)
	}

	// A host named in LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS is dialled.
	t.Setenv(EnvAllowedInternalHosts, "127.0.0.1")
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("with the host exempt: %v", err)
	}
	resp.Body.Close()
	t.Setenv(EnvAllowedInternalHosts, "")
	client.CloseIdleConnections()

	t.Setenv(EnvAllowInternal, "true")
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("with internal hosts allowed: %v", err)
	}
	resp.Body.Close()
}
