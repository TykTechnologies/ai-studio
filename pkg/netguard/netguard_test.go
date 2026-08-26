package netguard

import (
	"net"
	"net/url"
	"testing"
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
