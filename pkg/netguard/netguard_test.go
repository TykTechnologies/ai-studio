package netguard

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("bad test URL %q: %v", raw, err)
	}
	return u
}

func TestIsInternalIP(t *testing.T) {
	internal := []string{
		"10.0.0.1",
		"10.255.255.255",
		"172.16.0.1",
		"172.31.255.254",
		"192.168.1.1",
		"127.0.0.1",
		"127.8.8.8",
		"169.254.169.254", // cloud metadata endpoint
		"::1",
		"fc00::1",
		"fd12:3456::1",
		"fe80::1",
		"0.0.0.0",
		"0.1.2.3",         // "this network"
		"100.64.0.1",      // shared address space (CGNAT)
		"100.100.100.200", // Alibaba Cloud metadata endpoint
		"100.127.255.254",
		"::ffff:100.100.100.200",
	}
	for _, s := range internal {
		if !IsInternalIP(net.ParseIP(s)) {
			t.Errorf("expected %s to be classified internal", s)
		}
	}

	external := []string{
		"8.8.8.8",
		"1.1.1.1",
		"172.32.0.1", // just outside 172.16/12
		"11.0.0.1",
		"100.63.255.255", // just below 100.64/10
		"100.128.0.1",    // just above 100.64/10
		"2606:4700:4700::1111",
	}
	for _, s := range external {
		if IsInternalIP(net.ParseIP(s)) {
			t.Errorf("expected %s to be classified external", s)
		}
	}
}

func TestHostIsInternal(t *testing.T) {
	lookup := func(host string) ([]net.IP, error) {
		switch host {
		case "public.example.com":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		case "mixed.example.com":
			return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("10.0.0.7")}, nil
		case "cgnat.example.com":
			return []net.IP{net.ParseIP("100.100.100.200")}, nil
		}
		return nil, fmt.Errorf("no such host %q", host)
	}
	for host, want := range map[string]bool{
		"localhost":            true,
		"Plugin.LOCALHOST.":    true,
		"10.1.2.3":             true,
		"[::1]":                true,
		"100.100.100.200":      true,
		"mixed.example.com":    true, // any internal address taints the host
		"cgnat.example.com":    true,
		"unresolvable.invalid": true, // resolution failure counts as internal
		"public.example.com":   false,
		"8.8.8.8":              false,
		"2606:4700:4700::1111": false,
	} {
		if got := HostIsInternal(host, lookup); got != want {
			t.Errorf("HostIsInternal(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestValidateUpstreamURLSchemes(t *testing.T) {
	setBlockInternal(t, "")
	t.Setenv(EnvAllowedHosts, "")

	if err := ValidateUpstreamURL(mustURL(t, "https://api.openai.com/v1")); err != nil {
		t.Errorf("https should be allowed: %v", err)
	}
	if err := ValidateUpstreamURL(mustURL(t, "http://internal-llm:11434")); err != nil {
		t.Errorf("http should be allowed: %v", err)
	}
	if err := ValidateUpstreamURL(mustURL(t, "file:///etc/passwd")); err == nil {
		t.Error("file scheme must be rejected")
	}
	if err := ValidateUpstreamURL(mustURL(t, "gopher://x")); err == nil {
		t.Error("gopher scheme must be rejected")
	}
}

func TestValidateUpstreamURLAllowedHosts(t *testing.T) {
	setBlockInternal(t, "")
	t.Setenv(EnvAllowedHosts, "api.openai.com, .anthropic.com")

	if err := ValidateUpstreamURL(mustURL(t, "https://api.openai.com/v1")); err != nil {
		t.Errorf("exact allowlisted host should pass: %v", err)
	}
	if err := ValidateUpstreamURL(mustURL(t, "https://api.anthropic.com/v1")); err != nil {
		t.Errorf("suffix allowlisted host should pass: %v", err)
	}
	if err := ValidateUpstreamURL(mustURL(t, "https://API.OPENAI.COM/v1")); err != nil {
		t.Errorf("host matching should be case-insensitive: %v", err)
	}
	if err := ValidateUpstreamURL(mustURL(t, "https://evil.example.com/v1")); err == nil {
		t.Error("host outside allowlist must be rejected")
	}
	if err := ValidateUpstreamURL(mustURL(t, "https://notanthropic.com/v1")); err == nil {
		t.Error("suffix must match on label boundary, not substring")
	}
	if err := ValidateUpstreamURL(mustURL(t, "https://evil-anthropic.com/v1")); err == nil {
		t.Error("dash-joined lookalike domain must be rejected")
	}
	if err := ValidateUpstreamURL(mustURL(t, "https://anthropic.com/v1")); err != nil {
		t.Errorf("a '.suffix' entry should match the bare apex domain: %v", err)
	}
	if err := ValidateUpstreamURL(mustURL(t, "https://sub.api.anthropic.com/v1")); err != nil {
		t.Errorf("nested subdomains should match a '.suffix' entry: %v", err)
	}
}

func TestValidateUpstreamURLBlockInternal(t *testing.T) {
	t.Setenv(EnvAllowedHosts, "")
	setBlockInternal(t, "true")
	t.Setenv(EnvAllowInternal, "")

	blocked := []string{
		"http://127.0.0.1:11434",
		"http://10.1.2.3/api",
		"http://192.168.0.10:8080",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]:8080",
	}
	for _, raw := range blocked {
		if err := ValidateUpstreamURL(mustURL(t, raw)); err == nil {
			t.Errorf("expected %s to be blocked when %s=true", raw, EnvBlockInternal)
		}
	}

	// ALLOW_INTERNAL_NETWORK_ACCESS overrides the block for local development
	t.Setenv(EnvAllowInternal, "true")
	if err := ValidateUpstreamURL(mustURL(t, "http://127.0.0.1:11434")); err != nil {
		t.Errorf("%s=true should override internal blocking: %v", EnvAllowInternal, err)
	}
}

// Since 2.3 internal upstreams are blocked unless the setting says false
// (sf5); a local Ollama needs LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS.
func TestValidateUpstreamURLBlockInternalEnabledByDefault(t *testing.T) {
	t.Setenv(EnvAllowedHosts, "")
	t.Setenv(EnvAllowInternal, "")

	setBlockInternal(t, "")
	if err := ValidateUpstreamURL(mustURL(t, "http://127.0.0.1:11434")); err == nil {
		t.Error("internal upstreams must be blocked by default")
	}
	t.Setenv(EnvAllowedInternalHosts, "127.0.0.1")
	if err := ValidateUpstreamURL(mustURL(t, "http://127.0.0.1:11434")); err != nil {
		t.Errorf("an allow-listed internal host must pass: %v", err)
	}
	t.Setenv(EnvAllowedInternalHosts, "")
	setBlockInternal(t, "false")
	if err := ValidateUpstreamURL(mustURL(t, "http://127.0.0.1:11434")); err != nil {
		t.Errorf("%s=false must allow internal upstreams: %v", EnvBlockInternal, err)
	}
}

// setBlockInternal sets LLM_UPSTREAM_BLOCK_INTERNAL and applies it as startup
// does (Configure), restoring the stored setting after the test.
func setBlockInternal(t *testing.T, v string) {
	t.Helper()
	t.Setenv(EnvBlockInternal, v)
	prev := blockInternal.Load()
	t.Cleanup(func() { blockInternal.Store(prev) })
	if err := Configure(os.Getenv); err != nil {
		t.Fatal(err)
	}
}

// An invalid value is an error, never a silent default.
func TestParseBlockInternal(t *testing.T) {
	for v, want := range map[string]bool{"": true, "  ": true, "true": true, "TRUE": true, "1": true, " false ": false, "0": false, "False": false} {
		got, err := ParseBlockInternal(v)
		if err != nil || got != want {
			t.Errorf("ParseBlockInternal(%q) = %v, %v; want %v", v, got, err, want)
		}
	}
	for _, v := range []string{"ture", "yes", "off", "no"} {
		if _, err := ParseBlockInternal(v); err == nil {
			t.Errorf("ParseBlockInternal(%q) accepted an invalid value", v)
		}
	}
	prev := blockInternal.Load()
	t.Cleanup(func() { blockInternal.Store(prev) })
	blockInternal.Store(true)
	if err := Configure(func(string) string { return "yes" }); err == nil {
		t.Error("Configure accepted an invalid value")
	}
	if !blockInternal.Load() {
		t.Error("an invalid value changed the stored setting")
	}
}

func TestHTTPTransportBlocksInternalDialsPostDNS(t *testing.T) {
	t.Setenv(EnvAllowedHosts, "")
	t.Setenv(EnvAllowInternal, "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Transport: HTTPTransport(), Timeout: 5 * time.Second}

	t.Run("blocking disabled allows internal", func(t *testing.T) {
		setBlockInternal(t, "false")
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("expected success with blocking disabled: %v", err)
		}
		resp.Body.Close()
	})

	t.Run("blocking enabled rejects internal IP literal", func(t *testing.T) {
		setBlockInternal(t, "true")
		client.CloseIdleConnections() // force a fresh dial
		if _, err := client.Get(srv.URL); err == nil {
			t.Fatal("expected dial to an internal IP to be blocked")
		}
	})

	t.Run("blocking enabled rejects hostname resolving to internal IP", func(t *testing.T) {
		// The decisive DNS-rebinding case: validating the hostname up front is
		// not enough — the IP actually dialed (post-resolution) must be checked.
		setBlockInternal(t, "true")
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Get("http://localhost:" + u.Port()); err == nil {
			t.Fatal("expected dial to a hostname resolving to an internal IP to be blocked")
		}
	})

	t.Run("dev override allows internal", func(t *testing.T) {
		setBlockInternal(t, "true")
		t.Setenv(EnvAllowInternal, "true")
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("expected success with dev override: %v", err)
		}
		resp.Body.Close()
	})
}

func TestValidatingHTTPTransportAppliesURLPolicyPerRequest(t *testing.T) {
	setBlockInternal(t, "false")
	t.Setenv(EnvAllowInternal, "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Transport: ValidatingHTTPTransport(), Timeout: 5 * time.Second}

	t.Run("allowlisted request succeeds", func(t *testing.T) {
		u := mustURL(t, srv.URL)
		t.Setenv(EnvAllowedHosts, u.Hostname())
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("allowlisted host should succeed: %v", err)
		}
		resp.Body.Close()
	})

	t.Run("non-allowlisted request rejected before connecting", func(t *testing.T) {
		t.Setenv(EnvAllowedHosts, "registry.example.com")
		if _, err := client.Get(srv.URL); err == nil {
			t.Fatal("expected non-allowlisted host to be rejected")
		}
	})

	t.Run("no allowlist preserves default behavior", func(t *testing.T) {
		t.Setenv(EnvAllowedHosts, "")
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("expected success without an allowlist: %v", err)
		}
		resp.Body.Close()
	})
}

// The ranges added with blocking on by default (sf5), and IPv4-mapped IPv6,
// which IPNet.Contains matches against the IPv4 ranges.
func TestIsInternalIP_AddedRanges(t *testing.T) {
	for _, s := range []string{
		"64:ff9b::a00:1", "64:ff9b:1::1", "2002:a00:1::1", // NAT64, 6to4
		"192.0.0.8", "198.18.0.1", "198.19.255.255", // IETF, benchmarking
		"224.0.0.1", "239.255.255.250", "ff02::1", // multicast
		"240.0.0.1", "255.255.255.255", // reserved, broadcast
		"fec0::1",                             // site-local
		"::ffff:10.0.0.1", "::ffff:127.0.0.1", // IPv4-mapped
	} {
		if !IsInternalIP(net.ParseIP(s)) {
			t.Errorf("IsInternalIP(%s) = false", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "2606:4700::1111", "::ffff:8.8.8.8", "203.0.113.10"} {
		if IsInternalIP(net.ParseIP(s)) {
			t.Errorf("IsInternalIP(%s) = true", s)
		}
	}
}

// A redirect to an internal address is refused on the hop that reaches it,
// even when the first hop was allowed: every dial is checked.
func TestValidatingHTTPTransport_RefusesRedirectToInternal(t *testing.T) {
	t.Setenv(EnvAllowedHosts, "")
	t.Setenv(EnvAllowInternal, "")
	setBlockInternal(t, "true")

	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer internal.Close()
	// The first hop is reached as "localhost", exempted by name; it
	// redirects to 127.0.0.1, which is not.
	first := httptest.NewServer(http.RedirectHandler(internal.URL, http.StatusFound))
	defer first.Close()
	t.Setenv(EnvAllowedInternalHosts, "localhost")

	client := &http.Client{Transport: ValidatingHTTPTransport(), Timeout: 5 * time.Second}
	firstURL := mustURL(t, first.URL)
	resp, err := client.Get("http://localhost:" + firstURL.Port())
	if err == nil {
		resp.Body.Close()
		t.Fatal("the redirect to an internal address was followed")
	}
	if !strings.Contains(err.Error(), "internal address") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A policy refusal is recognisable through the wrapping of a dial error, so
// callers can pass its reason on (w4e.4); other errors are not refusals.
func TestRefusal(t *testing.T) {
	setBlockInternal(t, "")
	t.Setenv(EnvAllowedHosts, "")
	t.Setenv(EnvAllowedInternalHosts, "")

	reason, ok := Refusal(ValidateUpstreamURL(mustURL(t, "http://127.0.0.1:1")))
	if !ok || !strings.Contains(reason, EnvBlockInternal) {
		t.Fatalf("ValidateUpstreamURL refusal = %q, %v", reason, ok)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	_, err := (&http.Client{Transport: HTTPTransport()}).Get(srv.URL)
	if err == nil {
		t.Fatal("a dial to a loopback address succeeded with blocking on")
	}
	reason, ok = Refusal(err)
	if !ok || !strings.Contains(reason, EnvBlockInternal) || strings.Contains(reason, "dial tcp") {
		t.Fatalf("dial refusal = %q, %v (from %v)", reason, ok, err)
	}

	if _, ok := Refusal(fmt.Errorf("connection refused")); ok {
		t.Fatal("a plain error is not a refusal")
	}
}
