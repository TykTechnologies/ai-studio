// Package corsutil centralizes the CORS policy for endpoints that
// historically sent `Access-Control-Allow-Origin: *` (OAuth, proxy metadata,
// plugin assets). The policy is configured via CORS_ALLOWED_ORIGINS:
//
//   - unset (default) or "*": wildcard, preserving existing behavior
//   - comma-separated origins: the request Origin is echoed back only when it
//     matches an entry (case-insensitive); otherwise no CORS header is sent
//
// Credentials are never enabled by this package.
package corsutil

import (
	"net/http"
	"os"
	"strings"
)

// EnvAllowedOrigins configures the allowed origins for the endpoints that
// previously used a wildcard CORS policy.
const EnvAllowedOrigins = "CORS_ALLOWED_ORIGINS"

// AllowOrigin returns the value to send as Access-Control-Allow-Origin for a
// request with the given Origin header, and whether an explicit origin list
// is configured (callers should send `Vary: Origin` when it is). An empty
// returned origin means the header must be omitted.
func AllowOrigin(requestOrigin string) (string, bool) {
	configured := os.Getenv(EnvAllowedOrigins)
	if configured == "" {
		return "*", false
	}

	for _, entry := range strings.Split(configured, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "*" {
			return "*", false
		}
		if entry != "" && requestOrigin != "" && strings.EqualFold(entry, requestOrigin) {
			return requestOrigin, true
		}
	}
	return "", true
}

// DefaultAllowHeaders is the Access-Control-Allow-Headers value used by
// SetCORSHeaders.
const DefaultAllowHeaders = "Origin, Content-Type, Accept, Authorization"

// SetCORSHeaders applies the CORS policy to a response header set.
// methods is the value for Access-Control-Allow-Methods.
func SetCORSHeaders(h http.Header, requestOrigin, methods string) {
	SetCORSHeadersFor(h, requestOrigin, methods, DefaultAllowHeaders, "")
}

// SetCORSHeadersFor applies the CORS policy with explicit allowed and exposed
// header lists. Protocols carried over HTTP need more than the default set:
// MCP's StreamableHTTP transport sends Mcp-Session-Id and MCP-Protocol-Version
// on requests and returns Mcp-Session-Id, which a browser client cannot read
// unless it is exposed. exposeHeaders may be empty.
func SetCORSHeadersFor(h http.Header, requestOrigin, methods, allowHeaders, exposeHeaders string) {
	origin, configured := AllowOrigin(requestOrigin)
	if origin != "" {
		h.Set("Access-Control-Allow-Origin", origin)
	}
	if configured {
		h.Add("Vary", "Origin")
	}
	h.Set("Access-Control-Allow-Methods", methods)
	if allowHeaders == "" {
		allowHeaders = DefaultAllowHeaders
	}
	h.Set("Access-Control-Allow-Headers", allowHeaders)
	if exposeHeaders != "" {
		h.Set("Access-Control-Expose-Headers", exposeHeaders)
	}
	h.Set("Access-Control-Max-Age", "43200") // 12 hours
}
