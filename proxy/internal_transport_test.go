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
		{"plain listener, anthropic", false, models.ANTHROPIC, "http://127.0.0.1:8443/llm/call/my-llm/v1"},
		{"plain listener, google", false, models.GOOGLEAI, "http://127.0.0.1:8443/llm/call/my-llm"},
		{"tls listener, openai", true, models.OPENAI, "https://127.0.0.1:8443/llm/call/my-llm/v1"},
		{"tls listener, anthropic", true, models.ANTHROPIC, "https://127.0.0.1:8443/llm/call/my-llm/v1"},
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

// The langchaingo Anthropic client appends "/messages" to its base URL. The
// inner /llm/call/ hop forwards whatever follows the slug to the configured
// upstream, joining it with the endpoint's own path only when the two do not
// already overlap. So the loopback base URL must end in /v1: with it, the
// inner path is /v1/messages and both endpoint shapes operators actually
// configure resolve to https://api.anthropic.com/v1/messages, exactly as a
// native /llm/call/{slug}/v1/messages caller does. Without it the inner path
// is /messages and the no-/v1 endpoint shape is forwarded to
// https://api.anthropic.com/messages, which is a 404 (seen on staging).
func TestInternalLLMBaseURL_AnthropicLoopbackYieldsNativeMessagesPath(t *testing.T) {
	p := &Proxy{config: &Config{Port: 8080}}
	base := p.getInternalLLMBaseURL("claude", models.ANTHROPIC)
	innerPath := strings.TrimPrefix(base, "http://127.0.0.1:8080/llm/call/claude") + "/messages"
	if innerPath != "/v1/messages" {
		t.Fatalf("Anthropic SDK loopback path = %q, want /v1/messages", innerPath)
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

// Keep-Alive is an HTTP/1 connection header. Sending it on an HTTP/2 response
// violates RFC 9113 §8.2.2, and strict clients (curl 8.10+) fail the whole
// response over it, so it must only be set when the request arrived on HTTP/1.
func TestCloudflareHeadersMiddleware_NoKeepAliveOnHTTP2(t *testing.T) {
	p := &Proxy{}
	h := p.cloudflareHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, tc := range []struct {
		proto      string
		major      int
		wantHeader bool
	}{
		{"HTTP/1.1", 1, true},
		{"HTTP/2.0", 2, false},
	} {
		t.Run(tc.proto, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			req.Proto, req.ProtoMajor = tc.proto, tc.major
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if got := rec.Header().Get("Keep-Alive") != ""; got != tc.wantHeader {
				t.Fatalf("Keep-Alive present = %v on %s, want %v", got, tc.proto, tc.wantHeader)
			}
			if rec.Header().Get("X-Accel-Buffering") != "no" {
				t.Fatal("X-Accel-Buffering must be set regardless of protocol")
			}
		})
	}
}
