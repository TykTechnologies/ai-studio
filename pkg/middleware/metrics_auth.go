// Package middleware provides HTTP middleware shared between AI Studio and
// the Microgateway.
package middleware

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// MetricsBearerAuth returns middleware that requires "Authorization: Bearer <token>"
// on the metrics endpoint. Token comparison is constant-time.
func MetricsBearerAuth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		provided, ok := strings.CutPrefix(authHeader, "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			c.Header("WWW-Authenticate", "Bearer")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

// MetricsEndpointConfig configures RegisterMetricsEndpoint.
type MetricsEndpointConfig struct {
	// Path is the route path the handler is mounted on, e.g. "/metrics".
	Path string
	// Handler serves the metrics (typically a Prometheus handler). If nil,
	// the endpoint is not registered.
	Handler http.Handler
	// AuthToken, when non-empty, protects the endpoint with bearer auth.
	AuthToken string
	// AllowUnauthenticated serves the endpoint without auth when no token is
	// set (e.g. for in-cluster Prometheus scraping behind a network boundary).
	AllowUnauthenticated bool
	// Warn, if non-nil, is called with security-relevant warnings so each
	// service can route them through its own logger.
	Warn func(msg string)
}

// RegisterMetricsEndpoint mounts the metrics handler with secure defaults:
//   - AuthToken set: endpoint requires a matching bearer token
//   - AllowUnauthenticated true: endpoint is served without auth
//   - neither: endpoint is not registered at all
//
// Returns true if the endpoint was registered.
func RegisterMetricsEndpoint(router gin.IRoutes, cfg MetricsEndpointConfig) bool {
	if cfg.Handler == nil {
		return false
	}

	warn := cfg.Warn
	if warn == nil {
		warn = func(string) {}
	}

	switch {
	case cfg.AuthToken != "":
		router.GET(cfg.Path, MetricsBearerAuth(cfg.AuthToken), gin.WrapH(cfg.Handler))
		return true
	case cfg.AllowUnauthenticated:
		warn(fmt.Sprintf("Serving %s without authentication (METRICS_ALLOW_UNAUTHENTICATED=true); ensure it is not reachable from untrusted networks", cfg.Path))
		router.GET(cfg.Path, gin.WrapH(cfg.Handler))
		return true
	default:
		warn(fmt.Sprintf("%s endpoint disabled: set METRICS_AUTH_TOKEN to protect it with a bearer token, or METRICS_ALLOW_UNAUTHENTICATED=true to serve it without auth", cfg.Path))
		return false
	}
}
