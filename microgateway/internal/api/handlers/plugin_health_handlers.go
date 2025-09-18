// internal/api/handlers/plugin_health_handlers.go
package handlers

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
)

// GetPluginHealth returns detailed plugin health status
func GetPluginHealth(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if serviceContainer.PluginManager == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "Plugin manager not available",
			})
			return
		}

		// Get detailed plugin health
		pluginHealth := serviceContainer.PluginManager.GetPluginHealth()
		healthSummary := serviceContainer.PluginManager.GetPluginHealthSummary()

		c.JSON(http.StatusOK, gin.H{
			"summary": healthSummary,
			"plugins": pluginHealth,
		})
	}
}

// GetOCIPluginStatus returns OCI plugin system status
func GetOCIPluginStatus(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if serviceContainer.PluginManager == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "Plugin manager not available",
			})
			return
		}

		ociClient := serviceContainer.PluginManager.GetOCIClient()
		if ociClient == nil {
			c.JSON(http.StatusOK, gin.H{
				"enabled": false,
				"message": "OCI plugin support not enabled",
			})
			return
		}

		// Get OCI-specific stats
		ociStats := serviceContainer.PluginManager.GetOCIStats()

		// Get cached plugins
		cachedPlugins, err := serviceContainer.PluginManager.ListCachedOCIPlugins()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to list cached OCI plugins",
				"message": err.Error(),
			})
			return
		}

		// Get cache size
		cacheSize, err := ociClient.GetCacheSize()
		if err != nil {
			cacheSize = -1 // Indicate error
		}

		c.JSON(http.StatusOK, gin.H{
			"enabled":        true,
			"stats":          ociStats,
			"cached_plugins": cachedPlugins,
			"cache_size":     cacheSize,
		})
	}
}

// TriggerPluginPreWarm manually triggers plugin pre-warming
func TriggerPluginPreWarm(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if serviceContainer.PluginManager == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "Plugin manager not available",
			})
			return
		}

		var req struct {
			Command string `json:"command"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		// Trigger pre-fetch for the specific plugin
		if err := serviceContainer.PluginManager.PreFetchOCIPlugin(req.Command); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to pre-warm plugin",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Plugin pre-warming triggered successfully",
			"command": req.Command,
		})
	}
}

// GetDetailedHealthStatus returns comprehensive health status including plugins
func GetDetailedHealthStatus(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := gin.H{
			"service": "microgateway",
			"status":  "healthy",
		}

		// Check overall health
		if err := serviceContainer.Health(); err != nil {
			status["status"] = "unhealthy"
			status["error"] = err.Error()
		}

		// Add database health
		if err := serviceContainer.DB.Exec("SELECT 1").Error; err != nil {
			status["database"] = gin.H{
				"status": "unhealthy",
				"error":  err.Error(),
			}
		} else {
			status["database"] = gin.H{
				"status": "healthy",
			}
		}

		// Add plugin health details
		if serviceContainer.PluginManager != nil {
			status["plugins"] = gin.H{
				"summary":     serviceContainer.PluginManager.GetPluginHealthSummary(),
				"all_ready":   serviceContainer.PluginManager.IsAllPluginsReady(),
				"oci_stats":   serviceContainer.PluginManager.GetOCIStats(),
			}
		} else {
			status["plugins"] = gin.H{
				"status": "not available",
			}
		}

		// Set HTTP status based on overall health
		httpStatus := http.StatusOK
		if status["status"] == "unhealthy" {
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, status)
	}
}