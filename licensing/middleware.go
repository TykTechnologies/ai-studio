package licensing

import (
	"github.com/gin-gonic/gin"
)

func (l *Licenser) TelemetryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.TelemetryEnabled() {
			c.Next()
			return
		}

		c.Next()

		action, exists := c.Get(CtxActionKey)

		if exists {
			if actionStr, ok := action.(string); ok && actionStr != "" {
				accessType := "ui"
				if c.GetHeader("Authorization") != "" {
					accessType = "api"
				}

				l.SendHTTPTelemetry(actionStr, c.Writer.Status(), accessType)
			}
		}
	}
}

func SetAction(c *gin.Context, action string) {
	c.Set(CtxActionKey, action)
}

func ActionHandler(handler gin.HandlerFunc, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		SetAction(c, action)
		handler(c)
	}
}
