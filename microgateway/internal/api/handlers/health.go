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
			response := gin.H{
				"status":  "not ready",
				"service": "microgateway",
				"error":   err.Error(),
			}

			// Add plugin health details if available
			if services.PluginManager != nil {
				response["plugin_health"] = services.PluginManager.GetPluginHealthSummary()
			}

			c.JSON(http.StatusServiceUnavailable, response)
			return
		}

		response := gin.H{
			"status":  "ready",
			"service": "microgateway",
		}

		// Add plugin health summary to readiness response
		if services.PluginManager != nil {
			response["plugin_health"] = services.PluginManager.GetPluginHealthSummary()
		}

		c.JSON(http.StatusOK, response)
	}
}