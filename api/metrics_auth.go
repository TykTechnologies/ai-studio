package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/gin-gonic/gin"
)

// metricsBearerAuth returns middleware that requires "Authorization: Bearer <token>"
// on the metrics endpoint. Token comparison is constant-time.
func metricsBearerAuth(token string) gin.HandlerFunc {
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

// registerMetricsEndpoint mounts the Prometheus metrics handler with secure defaults:
//   - METRICS_AUTH_TOKEN set: endpoint requires a matching bearer token
//   - METRICS_ALLOW_UNAUTHENTICATED=true: endpoint is served without auth (e.g. for
//     in-cluster Prometheus scraping behind a network boundary)
//   - neither: endpoint is not registered at all
//
// Returns true if the endpoint was registered.
func registerMetricsEndpoint(router gin.IRoutes, path string, handler http.Handler, authToken string, allowUnauthenticated bool) bool {
	if handler == nil {
		return false
	}

	switch {
	case authToken != "":
		router.GET(path, metricsBearerAuth(authToken), gin.WrapH(handler))
		return true
	case allowUnauthenticated:
		logger.Warn("Serving metrics endpoint without authentication (METRICS_ALLOW_UNAUTHENTICATED=true); ensure it is not reachable from untrusted networks")
		router.GET(path, gin.WrapH(handler))
		return true
	default:
		logger.Warn("Metrics endpoint disabled: set METRICS_AUTH_TOKEN to protect it with a bearer token, or METRICS_ALLOW_UNAUTHENTICATED=true to serve it without auth")
		return false
	}
}
