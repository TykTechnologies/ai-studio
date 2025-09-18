// internal/api/handlers/plugin_handlers.go
package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
)

// ListPlugins returns paginated list of plugins
func ListPlugins(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse query parameters
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		hookType := c.Query("hook_type")
		isActive := c.DefaultQuery("active", "true") == "true"

		// Validate pagination parameters
		if page < 1 {
			page = 1
		}
		if limit < 1 || limit > 100 {
			limit = 20
		}

		// Get plugins from service
		plugins, total, err := serviceContainer.PluginService.ListPlugins(page, limit, hookType, isActive)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to list plugins",
				"message": err.Error(),
			})
			return
		}

		// Calculate pagination info
		totalPages := (total + int64(limit) - 1) / int64(limit)

		c.JSON(http.StatusOK, gin.H{
			"data": plugins,
			"pagination": gin.H{
				"page":        page,
				"limit":       limit,
				"total":       total,
				"total_pages": totalPages,
			},
		})
	}
}

// CreatePlugin creates a new plugin configuration
func CreatePlugin(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req services.CreatePluginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		// Perform API-level security validation on the command field
		if err := validatePluginCommand(req.Command); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Security validation failed",
				"message": err.Error(),
			})
			return
		}

		// Create plugin
		plugin, err := serviceContainer.PluginService.CreatePlugin(&req)
		if err != nil {
			if isConflictError(err) {
				c.JSON(http.StatusConflict, gin.H{
					"error":   "Conflict",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to create plugin",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"data":    plugin,
			"message": "Plugin created successfully",
		})
	}
}

// GetPlugin retrieves a specific plugin by ID
func GetPlugin(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid plugin ID",
				"message": "ID must be a valid number",
			})
			return
		}

		plugin, err := serviceContainer.PluginService.GetPlugin(uint(id))
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "Plugin not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to get plugin",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": plugin,
		})
	}
}

// UpdatePlugin updates an existing plugin
func UpdatePlugin(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid plugin ID",
				"message": "ID must be a valid number",
			})
			return
		}

		var req services.UpdatePluginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		// Perform API-level security validation on the command field if it's being updated
		if req.Command != nil {
			if err := validatePluginCommand(*req.Command); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Security validation failed",
					"message": err.Error(),
				})
				return
			}
		}

		plugin, err := serviceContainer.PluginService.UpdatePlugin(uint(id), &req)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "Plugin not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to update plugin",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data":    plugin,
			"message": "Plugin updated successfully",
		})
	}
}

// DeletePlugin soft deletes a plugin
func DeletePlugin(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid plugin ID",
				"message": "ID must be a valid number",
			})
			return
		}

		err = serviceContainer.PluginService.DeletePlugin(uint(id))
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "Plugin not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to delete plugin",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Plugin deleted successfully",
		})
	}
}

// TestPlugin tests a plugin with test data
func TestPlugin(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid plugin ID",
				"message": "ID must be a valid number",
			})
			return
		}

		var req map[string]interface{}
		if err := c.ShouldBindJSON(&req); err != nil {
			// Empty body is okay for testing
			req = make(map[string]interface{})
		}

		result, err := serviceContainer.PluginService.TestPlugin(uint(id), req)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "Plugin not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to test plugin",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data":    result,
			"message": "Plugin test completed successfully",
		})
	}
}

// GetLLMPlugins returns plugins associated with an LLM
func GetLLMPlugins(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid LLM ID",
				"message": "ID must be a valid number",
			})
			return
		}

		plugins, err := serviceContainer.PluginService.GetPluginsForLLM(uint(id))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to get LLM plugins",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": plugins,
		})
	}
}

// UpdateLLMPlugins updates plugin associations for an LLM
func UpdateLLMPlugins(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid LLM ID",
				"message": "ID must be a valid number",
			})
			return
		}

		var req struct {
			PluginIDs []uint `json:"plugin_ids"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		err = serviceContainer.PluginService.UpdateLLMPlugins(uint(id), req.PluginIDs)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "LLM or plugin not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to update LLM plugins",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "LLM plugins updated successfully",
		})
	}
}

// Helper functions

// isNotFoundError checks if an error indicates a resource was not found
func isNotFoundPluginError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "plugin not found") ||
		   strings.Contains(errMsg, "not found") ||
		   strings.Contains(errMsg, "record not found")
}

// isConflictError checks if an error indicates a conflict (duplicate resource)
func isConflictPluginError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "already exists") ||
		strings.Contains(err.Error(), "slug") && strings.Contains(err.Error(), "exists"))
}