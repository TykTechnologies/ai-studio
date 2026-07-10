package api

import (
	"github.com/TykTechnologies/midsommar/v2/pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// registerMetricsRoute mounts /metrics with secure defaults via the shared
// middleware package: METRICS_AUTH_TOKEN enables bearer auth,
// METRICS_ALLOW_UNAUTHENTICATED=true serves without auth, otherwise the
// endpoint is not registered. Returns true if the endpoint was registered.
func registerMetricsRoute(router gin.IRoutes, config *RouterConfig) bool {
	return middleware.RegisterMetricsEndpoint(router, middleware.MetricsEndpointConfig{
		Path:                 "/metrics",
		Handler:              config.MetricsHandler,
		AuthToken:            config.MetricsAuthToken,
		AllowUnauthenticated: config.MetricsAllowUnauthenticated,
		Warn:                 func(msg string) { log.Warn().Msg(msg) },
	})
}
