// internal/api/handlers/health.go
package handlers

import (
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/pkg/fipsinfo"
	"github.com/gin-gonic/gin"
)

// HealthCheck returns the health status of the service
func HealthCheck(services *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "microgateway",
			// FIPS 140-3 status: {enabled, version, enforced}
			"fips": fipsinfo.Current(),
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

		// An edge says which control endpoint it is on. It is ready either
		// way: an edge keeps serving through a control-plane outage.
		if services.ControlLink != nil {
			endpoint, connected := services.ControlLink()
			response["control_endpoint"] = endpoint
			response["control_connected"] = connected
		}
		// ... and where its configuration came from: a control plane, or
		// the snapshot it stored, when it started without one. Ready in
		// both cases.
		// A database with no room left: the gateway is ready (it serves
		// from memory and from what is stored), and says what it can no
		// longer store. Taking it out of service would turn a degraded
		// gateway into an outage.
		response["database_full"] = false
		if full, since, failing := database.StorageStatus(); full {
			response["database_full"] = true
			response["database_full_since"] = since.UTC().Format(time.RFC3339)
			response["database_full_not_stored"] = failing
		}
		if services.ConfigSource != nil {
			source, _ := services.ConfigSource()
			response["config_source"] = source
		}

		c.JSON(http.StatusOK, response)
	}
}