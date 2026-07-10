package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
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

// registerMetricsRoute mounts /metrics with secure defaults:
//   - METRICS_AUTH_TOKEN set: endpoint requires a matching bearer token
//   - METRICS_ALLOW_UNAUTHENTICATED=true: endpoint is served without auth (e.g. for
//     in-cluster Prometheus scraping behind a network boundary)
//   - neither: endpoint is not registered at all
//
// Returns true if the endpoint was registered.
func registerMetricsRoute(router gin.IRoutes, config *RouterConfig) bool {
	if config.MetricsHandler == nil {
		return false
	}

	switch {
	case config.MetricsAuthToken != "":
		router.GET("/metrics", metricsBearerAuth(config.MetricsAuthToken), gin.WrapH(config.MetricsHandler))
		return true
	case config.MetricsAllowUnauthenticated:
		log.Warn().Msg("Serving /metrics without authentication (METRICS_ALLOW_UNAUTHENTICATED=true); ensure it is not reachable from untrusted networks")
		router.GET("/metrics", gin.WrapH(config.MetricsHandler))
		return true
	default:
		log.Warn().Msg("/metrics endpoint disabled: set METRICS_AUTH_TOKEN to protect it with a bearer token, or METRICS_ALLOW_UNAUTHENTICATED=true to serve it without auth")
		return false
	}
}
