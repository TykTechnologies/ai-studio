package netguard

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing %q: %v", raw, err)
	}
	return u
}

func TestInternalLiteralBlockedWithoutExemption(t *testing.T) {
	t.Setenv(EnvBlockInternal, "true")

	err := ValidateUpstreamURL(mustParseURL(t, "http://10.42.1.7:8000/v1"))
	if err == nil {
		t.Fatal("an internal address must be refused when blocking is on and nothing is exempted")
	}
}

func TestInternalHostExemptionPermitsClusterUpstream(t *testing.T) {
	t.Setenv(EnvBlockInternal, "true")
	t.Setenv(EnvAllowedInternalHosts, ".svc.cluster.local")

	for _, raw := range []string{
		"http://vllm-pool.inference.svc.cluster.local:8000/v1",
		"http://svc.cluster.local:8000/v1",
	} {
		if err := ValidateUpstreamURL(mustParseURL(t, raw)); err != nil {
			t.Errorf("exempted host %q should be permitted: %v", raw, err)
		}
	}
}

func TestInternalHostExemptionDoesNotMatchLookalikes(t *testing.T) {
	t.Setenv(EnvBlockInternal, "true")
	t.Setenv(EnvAllowedInternalHosts, ".svc.cluster.local")

	// The suffix must match on a label boundary, not as a bare string suffix.
	if err := ValidateUpstreamURL(mustParseURL(t, "http://10.0.0.1/v1")); err == nil {
		t.Error("an unrelated internal literal must still be refused")
	}
	if internalHostExempt("evilsvc.cluster.local") {
		t.Error("evilsvc.cluster.local must not match .svc.cluster.local")
	}
}

func TestInternalExemptionLeavesExternalEgressAlone(t *testing.T) {
	// The whole point of the separate variable: naming a cluster host must not
	// silently restrict external providers the way EnvAllowedHosts would.
	t.Setenv(EnvBlockInternal, "true")
	t.Setenv(EnvAllowedInternalHosts, ".svc.cluster.local")

	if err := ValidateUpstreamURL(mustParseURL(t, "https://api.anthropic.com/v1/messages")); err != nil {
		t.Errorf("external provider must remain reachable: %v", err)
	}
	if err := ValidateUpstreamURL(mustParseURL(t, "https://api.openai.com/v1/chat/completions")); err != nil {
		t.Errorf("external provider must remain reachable: %v", err)
	}
}

func TestInternalExemptionOverridesGlobalAllowlist(t *testing.T) {
	t.Setenv(EnvBlockInternal, "true")
	t.Setenv(EnvAllowedHosts, ".anthropic.com")
	t.Setenv(EnvAllowedInternalHosts, ".svc.cluster.local")

	if err := ValidateUpstreamURL(mustParseURL(t, "http://vllm.inference.svc.cluster.local:8000/v1")); err != nil {
		t.Errorf("the internal exemption must apply even with a global allowlist set: %v", err)
	}
	// The global allowlist still governs everything not exempted.
	if err := ValidateUpstreamURL(mustParseURL(t, "https://api.openai.com/v1")); err == nil {
		t.Error("a host outside the global allowlist must still be refused")
	}
}

// TestDialGuardAgreesWithValidate is the important one: ValidateUpstreamURL and
// the dial-time guard must reach the same verdict, or an upstream would pass
// validation and then fail to connect.
func TestDialGuardAgreesWithValidate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// httptest binds loopback, which is an internal range.
	_, port, err := net.SplitHostPort(mustParseURL(t, upstream.URL).Host)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("blocked without exemption", func(t *testing.T) {
		t.Setenv(EnvBlockInternal, "true")

		client := &http.Client{Transport: HTTPTransport()}
		_, err := client.Get(upstream.URL)
		if err == nil {
			t.Fatal("dial to an internal address must be refused when blocking is on")
		}
	})

	t.Run("permitted when the host is exempted", func(t *testing.T) {
		t.Setenv(EnvBlockInternal, "true")
		t.Setenv(EnvAllowedInternalHosts, "127.0.0.1")

		target := "http://127.0.0.1:" + port
		if err := ValidateUpstreamURL(mustParseURL(t, target)); err != nil {
			t.Fatalf("validation refused an exempted host: %v", err)
		}

		client := &http.Client{Transport: HTTPTransport()}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("dial to an exempted host must succeed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("got %d, want 200", resp.StatusCode)
		}
	})
}

func TestNoExemptionConfiguredIsInert(t *testing.T) {
	if internalHostExempt("anything.svc.cluster.local") {
		t.Error("nothing may be exempt when the variable is unset")
	}
}
