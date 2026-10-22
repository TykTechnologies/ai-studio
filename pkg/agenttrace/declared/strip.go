package declared

import (
	"net/http"
	"strings"
	"sync/atomic"
)

// TracingHeaders are the X-Tyk-* tracing headers (§4.3). Only these are
// stripped: other X-Tyk-* headers belong to other features.
var TracingHeaders = []string{
	"X-Tyk-Session-Id",
	"X-Tyk-Session-Name",
	"X-Tyk-Trace-Id",
	"X-Tyk-Span-Id",
	"X-Tyk-Parent-Span-Id",
	"X-Tyk-Agent-Name",
	"X-Tyk-Agent-Id",
	"X-Tyk-Agent-Path",
	"X-Tyk-User-Id",
	"X-Tyk-Trace-Capture",
}

var forwardTykHeaders atomic.Bool

// SetForwardTykHeaders sets propagation.forward_tyk_headers
// (AGENT_TRACING_FORWARD_TYK_HEADERS): when true the X-Tyk-* tracing headers
// reach the upstream. The default, false, strips them in every edition.
func SetForwardTykHeaders(v bool) { forwardTykHeaders.Store(v) }

// StripTracingHeaders removes the X-Tyk-* tracing headers from an upstream
// request's headers unless forwarding is on. Compat headers pass through
// unchanged. It is cheap enough for every request: no allocation.
func StripTracingHeaders(h http.Header) {
	if forwardTykHeaders.Load() || len(h) == 0 {
		return
	}
	// Keys are compared case-insensitively: a header set by direct map
	// assignment may not be canonical.
	for k := range h {
		if len(k) > 6 && strings.EqualFold(k[:6], "x-tyk-") && isTracingHeader(k) {
			delete(h, k)
		}
	}
}

func isTracingHeader(k string) bool {
	for _, t := range TracingHeaders {
		if strings.EqualFold(k, t) {
			return true
		}
	}
	return false
}
