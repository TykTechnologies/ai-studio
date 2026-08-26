package netguard

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		"2606:4700:4700::1111",
	}
	for _, s := range external {
		if IsInternalIP(net.ParseIP(s)) {
			t.Errorf("expected %s to be classified external", s)
		}
	}
}

func TestValidateUpstreamURLSchemes(t *testing.T) {
	t.Setenv(EnvBlockInternal, "")
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
	t.Setenv(EnvBlockInternal, "")
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
	t.Setenv(EnvBlockInternal, "true")
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

func TestValidateUpstreamURLBlockInternalDisabledByDefault(t *testing.T) {
	t.Setenv(EnvAllowedHosts, "")
	t.Setenv(EnvBlockInternal, "")
	t.Setenv(EnvAllowInternal, "")

	// Backwards compatible: internal upstreams (e.g. local Ollama) work unless
	// blocking is explicitly enabled.
	if err := ValidateUpstreamURL(mustURL(t, "http://127.0.0.1:11434")); err != nil {
		t.Errorf("internal upstreams must be allowed by default: %v", err)
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
		t.Setenv(EnvBlockInternal, "")
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("expected success with blocking disabled: %v", err)
		}
		resp.Body.Close()
	})

	t.Run("blocking enabled rejects internal IP literal", func(t *testing.T) {
		t.Setenv(EnvBlockInternal, "true")
		client.CloseIdleConnections() // force a fresh dial
		if _, err := client.Get(srv.URL); err == nil {
			t.Fatal("expected dial to an internal IP to be blocked")
		}
	})

	t.Run("blocking enabled rejects hostname resolving to internal IP", func(t *testing.T) {
		// The decisive DNS-rebinding case: validating the hostname up front is
		// not enough — the IP actually dialed (post-resolution) must be checked.
		t.Setenv(EnvBlockInternal, "true")
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Get("http://localhost:" + u.Port()); err == nil {
			t.Fatal("expected dial to a hostname resolving to an internal IP to be blocked")
		}
	})

	t.Run("dev override allows internal", func(t *testing.T) {
		t.Setenv(EnvBlockInternal, "true")
		t.Setenv(EnvAllowInternal, "true")
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("expected success with dev override: %v", err)
		}
		resp.Body.Close()
	})
}

func TestValidatingHTTPTransportAppliesURLPolicyPerRequest(t *testing.T) {
	t.Setenv(EnvBlockInternal, "")
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
