// internal/api/handlers/llm_handlers.go
package handlers

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
)

// ListLLMs returns paginated list of LLMs
func ListLLMs(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse query parameters
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		vendor := c.Query("vendor")
		isActive := c.DefaultQuery("active", "true") == "true"

		// Validate pagination parameters
		if page < 1 {
			page = 1
		}
		if limit < 1 || limit > 100 {
			limit = 20
		}

		// Get LLMs from service
		llms, total, err := serviceContainer.Management.ListLLMs(page, limit, vendor, isActive)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to list LLMs",
				"message": err.Error(),
			})
			return
		}

		// Calculate pagination info
		totalPages := (total + int64(limit) - 1) / int64(limit)

		c.JSON(http.StatusOK, gin.H{
			"data": llms,
			"pagination": gin.H{
				"page":        page,
				"limit":       limit,
				"total":       total,
				"total_pages": totalPages,
			},
		})
	}
}

// CreateLLM creates a new LLM configuration
func CreateLLM(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req services.CreateLLMRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		// Create LLM
		llm, err := serviceContainer.Management.CreateLLM(&req)
		if err != nil {
			if isConflictError(err) {
				c.JSON(http.StatusConflict, gin.H{
					"error":   "Conflict",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to create LLM",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"data":    llm,
			"message": "LLM created successfully",
		})
	}
}

// GetLLM retrieves a specific LLM by ID
func GetLLM(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid LLM ID",
				"message": "ID must be a valid number",
			})
			return
		}

		llm, err := serviceContainer.Management.GetLLM(uint(id))
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "LLM not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to get LLM",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": llm,
		})
	}
}

// UpdateLLM updates an existing LLM
func UpdateLLM(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid LLM ID",
				"message": "ID must be a valid number",
			})
			return
		}

		var req services.UpdateLLMRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		llm, err := serviceContainer.Management.UpdateLLM(uint(id), &req)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "LLM not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to update LLM",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data":    llm,
			"message": "LLM updated successfully",
		})
	}
}

// DeleteLLM soft deletes an LLM
func DeleteLLM(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid LLM ID",
				"message": "ID must be a valid number",
			})
			return
		}

		err = serviceContainer.Management.DeleteLLM(uint(id))
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "LLM not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to delete LLM",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "LLM deleted successfully",
		})
	}
}

// GetLLMStats returns statistics for an LLM
func GetLLMStats(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid LLM ID",
				"message": "ID must be a valid number",
			})
			return
		}

		// Verify LLM exists
		_, err = serviceContainer.Management.GetLLM(uint(id))
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "LLM not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to verify LLM",
				"message": err.Error(),
			})
			return
		}

		// Get stats from gateway service
		if gatewayService, ok := serviceContainer.GatewayService.(*services.DatabaseGatewayService); ok {
			stats, err := gatewayService.GetLLMStats(uint(id))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "Failed to get LLM stats",
					"message": err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"data": stats,
			})
		} else {
			c.JSON(http.StatusNotImplemented, gin.H{
				"error": "LLM stats not available",
			})
		}
	}
}

// Helper functions

// isNotFoundError checks if an error indicates a resource was not found
func isNotFoundError(err error) bool {
	return err != nil && (err.Error() == "LLM not found" || 
		err.Error() == "app not found" ||
		err.Error() == "credential not found")
}

// isConflictError checks if an error indicates a conflict (duplicate resource)
func isConflictError(err error) bool {
	return err != nil && (err.Error() == "LLM with this name already exists" ||
		err.Error() == "app with this name already exists")
}