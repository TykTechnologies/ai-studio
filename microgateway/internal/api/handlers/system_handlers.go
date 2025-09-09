// internal/api/handlers/system_handlers.go
package handlers

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
)

// ReloadConfiguration reloads the AI Gateway configuration
func ReloadConfiguration(gateway aigateway.Gateway) gin.HandlerFunc {
	return func(c *gin.Context) {
		if gateway == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Gateway not available",
				"message": "AI Gateway is not initialized",
			})
			return
		}

		if err := gateway.Reload(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to reload configuration",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "AI Gateway configuration reloaded successfully",
		})
	}
}

// GetSystemInfo returns system information and status
func GetSystemInfo(serviceContainer *services.ServiceContainer, version, buildHash, buildTime string) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats := serviceContainer.GetStats()
		
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"service":    "microgateway",
				"version":    version,
				"build_hash": buildHash,
				"build_time": buildTime,
				"status":     "running",
				"stats":      stats,
			},
		})
	}
}