package proxy

import (
	"bytes"
	"io"
	"net/http"

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
}

// NewInternalRoutingTransport creates a transport that passes through the original
// client auth header while stripping any vendor-specific auth headers set by the SDK.
func NewInternalRoutingTransport(originalAuth string) *InternalRoutingTransport {
	// Create a custom transport that disables automatic gzip handling
	// This prevents double-decompression issues when the SDK also tries to handle gzip
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true // Don't add Accept-Encoding or auto-decompress

	return &InternalRoutingTransport{
		underlying:   transport,
		originalAuth: originalAuth,
	}
}

func (t *InternalRoutingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// DEBUG: Log the outgoing request details
	bodySize := int64(0)
	if req.Body != nil {
		// Read body to get size, then restore it
		bodyBytes, err := io.ReadAll(req.Body)
		if err == nil {
			bodySize = int64(len(bodyBytes))
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}
	log.Debug().
		Str("url", req.URL.String()).
		Str("method", req.Method).
		Int64("body_size", bodySize).
		Int64("content_length", req.ContentLength).
		Msg("InternalRoutingTransport.RoundTrip")

	// Strip SDK-set vendor auth headers
	// The SDK may set these, but /llm/ will set the correct vendor auth
	// from stored LLM credentials via vendor.ProxySetAuthHeader()
	req.Header.Del("x-api-key")    // Anthropic
	req.Header.Del("Authorization") // OpenAI/others (SDK may set this)

	// Pass through the original client auth so /llm/ can authenticate
	if t.originalAuth != "" {
		req.Header.Set("Authorization", t.originalAuth)
	}

	return t.underlying.RoundTrip(req)
}
