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

// SetCORSHeaders applies the CORS policy to a response header set.
// methods is the value for Access-Control-Allow-Methods.
func SetCORSHeaders(h http.Header, requestOrigin, methods string) {
	origin, configured := AllowOrigin(requestOrigin)
	if origin != "" {
		h.Set("Access-Control-Allow-Origin", origin)
	}
	if configured {
		h.Add("Vary", "Origin")
	}
	h.Set("Access-Control-Allow-Methods", methods)
	h.Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
	h.Set("Access-Control-Max-Age", "43200") // 12 hours
}
