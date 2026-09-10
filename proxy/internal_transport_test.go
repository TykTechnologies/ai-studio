package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// The /ai/ and unified-router handlers re-enter the gateway with a loopback
// call to /llm/call/{slug} on the proxy's own port. When the listener serves
// TLS that hop must be HTTPS, otherwise Go's server rejects it with "client
// sent an HTTP request to an HTTPS server" and every OpenAI-compatible request
// fails (seen on a microgateway with TLS_ENABLED=true).
func TestInternalLLMBaseURL_SchemeFollowsListenerTLS(t *testing.T) {
	tests := []struct {
		name   string
		tls    bool
		vendor models.Vendor
		want   string
	}{
		{"plain listener, openai", false, models.OPENAI, "http://127.0.0.1:8443/llm/call/my-llm/v1"},
		{"plain listener, anthropic", false, models.ANTHROPIC, "http://127.0.0.1:8443/llm/call/my-llm"},
		{"tls listener, openai", true, models.OPENAI, "https://127.0.0.1:8443/llm/call/my-llm/v1"},
		{"tls listener, anthropic", true, models.ANTHROPIC, "https://127.0.0.1:8443/llm/call/my-llm"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Proxy{config: &Config{Port: 8443, TLSEnabled: tc.tls}}
			if got := p.getInternalLLMBaseURL("my-llm", tc.vendor); got != tc.want {
				t.Fatalf("getInternalLLMBaseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInternalRoutingTransport_PlainListenerKeepsDefaultTLSConfig(t *testing.T) {
	tr := NewInternalRoutingTransport("Bearer abc", false)
	underlying := tr.underlying.(*http.Transport)
	if underlying.TLSClientConfig != nil && underlying.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("plain-HTTP loopback must not disable certificate verification")
	}
	if !underlying.DisableCompression {
		t.Fatal("loopback transport must keep compression disabled")
	}
}

// The per-request client must share one connection pool so keep-alives are
// reused and file descriptors are not exhausted under load.
func TestInternalRoutingClient_SharesConnectionPool(t *testing.T) {
	p := &Proxy{config: &Config{Port: 8443, TLSEnabled: true}}
	first := p.newInternalRoutingClient("Bearer a").Transport.(*InternalRoutingTransport)
	second := p.newInternalRoutingClient("Bearer b").Transport.(*InternalRoutingTransport)
	if first.underlying != second.underlying {
		t.Fatal("each request built its own transport; the connection pool must be shared")
	}
	if first.originalAuth != "Bearer a" || second.originalAuth != "Bearer b" {
		t.Fatal("per-request auth must stay with its own wrapper")
	}
	pool := first.underlying.(*http.Transport)
	if pool.TLSClientConfig == nil || !pool.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("shared pool must follow the listener's TLS setting")
	}
}

// End to end: the loopback transport can call an HTTPS listener whose
// certificate is not trusted (self-signed, issued for another name), and still
// swaps the SDK's vendor auth for the caller's Authorization header.
func TestInternalRoutingTransport_ReachesTLSListener(t *testing.T) {
	var gotAuth, gotAPIKey string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	client := &http.Client{Transport: NewInternalRoutingTransport("Bearer client-token", true)}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/llm/call/my-llm/v1/messages", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	// What an SDK would set from the dummy internal API key.
	req.Header.Set("x-api-key", "internal-routing-dummy-key")
	req.Header.Set("Authorization", "Bearer internal-routing-dummy-key")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("loopback over TLS failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if gotAuth != "Bearer client-token" {
		t.Fatalf("Authorization forwarded = %q, want the caller's token", gotAuth)
	}
	if gotAPIKey != "" {
		t.Fatalf("x-api-key = %q, want SDK vendor auth stripped", gotAPIKey)
	}

	// Without the TLS flag the same server is unreachable: this is the bug.
	plain := &http.Client{Transport: NewInternalRoutingTransport("Bearer client-token", false)}
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/llm/call/my-llm/v1/messages", strings.NewReader(`{}`))
	if resp2, err := plain.Do(req2); err == nil {
		resp2.Body.Close()
		t.Fatal("expected certificate verification to fail without the loopback TLS flag")
	}
}
