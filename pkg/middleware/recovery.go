package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/TykTechnologies/midsommar/v2/pkg/requestid"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// Recovery answers 500 when a handler panics. Unlike gin.Recovery, which
// dumps the request headers (masking only Authorization), it logs the method,
// route template, request id, panic value and stack only: headers carry
// cookies, API keys and end-user identity (X-Tyk-User-Id, baggage).
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		route := c.FullPath()
		if route == "" {
			route = "(unmatched)"
		}
		log.Error().
			Str("method", c.Request.Method).
			Str("route", route).
			Str("request_id", requestid.FromContext(c.Request.Context())).
			Interface("panic", recovered).
			Bytes("stack", debug.Stack()).
			Msg("Recovered from panic in HTTP handler")
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}
