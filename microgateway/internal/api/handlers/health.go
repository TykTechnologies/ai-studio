// internal/api/handlers/health.go
package handlers

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
)

// HealthCheck returns the health status of the service
func HealthCheck(services *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"service": "microgateway",
		})
	}
}

// ReadinessCheck returns the readiness status of the service
func ReadinessCheck(services *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if all critical services are ready
		if err := services.Health(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"error":  err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
			"service": "microgateway",
		})
	}
}