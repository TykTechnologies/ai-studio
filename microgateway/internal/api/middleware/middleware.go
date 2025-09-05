// internal/api/middleware/middleware.go
package middleware

import (
	"time"

	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// RequestLogger logs HTTP requests with structured logging
func RequestLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		log.Info().
			Str("method", param.Method).
			Str("path", param.Path).
			Int("status", param.StatusCode).
			Dur("latency", param.Latency).
			Str("client_ip", param.ClientIP).
			Str("user_agent", param.Request.UserAgent()).
			Msg("HTTP Request")
		return ""
	})
}

// RequestID adds a unique request ID to each request
func RequestID() gin.HandlerFunc {
	return requestid.New()
}

// CORS adds CORS headers to responses
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Request-ID")
		c.Header("Access-Control-Expose-Headers", "X-Request-ID")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// RateLimit implements basic rate limiting (placeholder)
func RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement rate limiting logic
		// This would integrate with Redis or in-memory store
		c.Next()
	}
}

// Security adds basic security headers
func Security() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

// Timeout implements request timeout middleware
func Timeout(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement timeout logic
		// gin.TimeoutWithHandler is not available, would need custom implementation
		c.Next()
	}
}