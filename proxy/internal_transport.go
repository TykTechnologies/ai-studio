package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// InternalRoutingTransport intercepts SDK HTTP calls for internal routing.
// When the /ai/ (OpenAI compatibility) endpoint routes requests through /llm/,
// this transport:
// 1. Strips vendor-specific auth headers set by the SDK
// 2. Passes through the original client's Authorization header
//
// This allows /llm/ to authenticate the request using the client's credentials
// and then set the correct vendor auth (from stored LLM config) before forwarding.
type InternalRoutingTransport struct {
	underlying   http.RoundTripper
	originalAuth string // From the /ai/ request's Authorization header
	// extra headers to add to every loopback request; the failover marker
	// rides here so the inner hop can grant inherited access to a fallback.
	extra http.Header
}

// NewInternalRoutingTransport creates a transport that passes through the original
// client auth header while stripping any vendor-specific auth headers set by the SDK.
//
// serverTLS must be true when the listener being called back terminates TLS: the
// loopback then dials 127.0.0.1 over HTTPS without verifying the certificate,
// which is issued for the public hostname rather than the loopback address.
//
// This builds a private connection pool; the proxy's request path uses
// newInternalRoutingTransport with a shared pool instead.
func NewInternalRoutingTransport(originalAuth string, serverTLS bool) *InternalRoutingTransport {
	return newInternalRoutingTransport(newLoopbackTransport(serverTLS), originalAuth)
}

// newInternalRoutingTransport wraps an existing pool with the per-request auth
// handling. The wrapper is cheap and holds only the caller's Authorization
// header; the underlying pool is what must be shared across requests.
func newInternalRoutingTransport(underlying http.RoundTripper, originalAuth string) *InternalRoutingTransport {
	return &InternalRoutingTransport{
		underlying:   underlying,
		originalAuth: originalAuth,
	}
}

// newLoopbackTransport builds the connection pool for calls back into our own
// listener on 127.0.0.1.
func newLoopbackTransport(serverTLS bool) *http.Transport {
	// Create a custom transport that disables automatic gzip handling
	// This prevents double-decompression issues when the SDK also tries to handle gzip
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true // Don't add Accept-Encoding or auto-decompress
	// Every /ai/ and unified-router request makes this hop to one host, so
	// the default of 2 idle connections per host churned connections under
	// any concurrency (see upstreamMaxIdleConnsPerHost).
	transport.MaxIdleConns = upstreamMaxIdleConns
	transport.MaxIdleConnsPerHost = upstreamMaxIdleConnsPerHost
	if serverTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // loopback to our own listener on 127.0.0.1
	}
	return transport
}

func (t *InternalRoutingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// DEBUG: Log the outgoing request details. Measuring the body means
	// reading and re-buffering it, so do it only when the line is logged.
	if ev := log.Debug(); ev.Enabled() {
		bodySize := int64(0)
		if req.Body != nil {
			// Read body to get size, then restore it
			bodyBytes, err := io.ReadAll(req.Body)
			if err == nil {
				bodySize = int64(len(bodyBytes))
				req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}
		}
		ev.Str("url", req.URL.String()).
			Str("method", req.Method).
			Int64("body_size", bodySize).
			Int64("content_length", req.ContentLength).
			Msg("InternalRoutingTransport.RoundTrip")
	}

	// Strip SDK-set vendor auth headers
	// The SDK may set these, but /llm/ will set the correct vendor auth
	// from stored LLM credentials via vendor.ProxySetAuthHeader()
	req.Header.Del("x-api-key")     // Anthropic
	req.Header.Del("Authorization") // OpenAI/others (SDK may set this)

	// Pass through the original client auth so /llm/ can authenticate
	if t.originalAuth != "" {
		req.Header.Set("Authorization", t.originalAuth)
	}
	for k, v := range t.extra {
		req.Header[k] = v
	}
	// Mark the hop so the inner handler answers a policy block in the
	// OpenAI error envelope the driver on this side can read (see
	// respondPolicyBlock). The marker changes only the error body's shape,
	// never what is enforced, so it needs no trust.
	req.Header.Set(hdrInternalHop, "1")

	resp, err := loopbackRoundTrip(t.underlying, req)
	if err == nil && resp.StatusCode < http.StatusMultipleChoices {
		if relay := loopbackRelayFrom(req.Context()); relay != nil {
			relay.capture(resp.Header)
		}
	}
	return resp, err
}

// loopbackRelay carries the inner hop's plugin headers back to the /ai/
// handler. The SDK driver consumes the loopback response itself, so without
// it headers a plugin set on /llm/call/ (a cache hit's X-Cache-Status, for
// example) never reach the client of the OpenAI-compatible endpoint.
type loopbackRelay struct {
	mu sync.Mutex
	h  http.Header
}

type loopbackRelayKey struct{}

func withLoopbackRelay(ctx context.Context, relay *loopbackRelay) context.Context {
	return context.WithValue(ctx, loopbackRelayKey{}, relay)
}

func loopbackRelayFrom(ctx context.Context) *loopbackRelay {
	relay, _ := ctx.Value(loopbackRelayKey{}).(*loopbackRelay)
	return relay
}

// isRelayedLoopbackHeader reports whether an inner-hop response header is
// passed on to the /ai/ client. Only the cache headers are: vendor and
// transport headers describe the loopback response, not the translated one.
func isRelayedLoopbackHeader(key string) bool {
	return strings.EqualFold(key, "X-Cache") ||
		len(key) > len("X-Cache-") && strings.EqualFold(key[:len("X-Cache-")], "X-Cache-")
}

// Relayed headers can come from a vendor (or a CDN in front of it) as well as
// a plugin, so how much is copied onto the client response is capped. The
// cache plugin sets at most seven short values.
const (
	maxRelayedHeaderValues = 16
	maxRelayedHeaderBytes  = 4 << 10
)

// capture keeps the relayed headers of the latest successful loopback
// response. A driver that retries replaces the earlier response's headers.
// Keys are taken in sorted order so the cap drops the same ones every time.
func (l *loopbackRelay) capture(h http.Header) {
	keys := make([]string, 0, len(h))
	for k := range h {
		if isRelayedLoopbackHeader(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	kept := make(http.Header)
	values, size := 0, 0
	for _, k := range keys {
		for _, v := range h[k] {
			if values == maxRelayedHeaderValues || size+len(k)+len(v) > maxRelayedHeaderBytes {
				break
			}
			kept[k] = append(kept[k], v)
			values++
			size += len(k) + len(v)
		}
	}
	l.mu.Lock()
	l.h = kept
	l.mu.Unlock()
}

// applyTo replaces any relayed headers in dst with the captured ones. It must
// run before the response is committed.
func (l *loopbackRelay) applyTo(dst http.Header) {
	for k := range dst {
		if isRelayedLoopbackHeader(k) {
			delete(dst, k)
		}
	}
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, v := range l.h {
		dst[k] = v
	}
}
