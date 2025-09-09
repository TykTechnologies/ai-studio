// internal/api/handlers/filter_handlers.go
package handlers

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
)

// ListFilters returns paginated list of filters
func ListFilters(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse query parameters
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		isActive := c.DefaultQuery("active", "true") == "true"

		// Validate pagination
		if page < 1 {
			page = 1
		}
		if limit < 1 || limit > 100 {
			limit = 20
		}

		// Get filters from service
		filters, total, err := serviceContainer.FilterService.ListFilters(page, limit, isActive)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to list filters",
				"message": err.Error(),
			})
			return
		}

		// Calculate pagination info
		totalPages := (total + int64(limit) - 1) / int64(limit)

		c.JSON(http.StatusOK, gin.H{
			"data": filters,
			"pagination": gin.H{
				"page":        page,
				"limit":       limit,
				"total":       total,
				"total_pages": totalPages,
			},
		})
	}
}

// CreateFilter creates a new filter
func CreateFilter(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req services.CreateFilterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		// Create filter
		filter, err := serviceContainer.FilterService.CreateFilter(&req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Failed to create filter",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"data":    filter,
			"message": "Filter created successfully",
		})
	}
}

// GetFilter retrieves a specific filter by ID
func GetFilter(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid filter ID",
				"message": "ID must be a valid number",
			})
			return
		}

		filter, err := serviceContainer.FilterService.GetFilter(uint(id))
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "Filter not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to get filter",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": filter,
		})
	}
}

// UpdateFilter updates an existing filter
func UpdateFilter(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid filter ID",
				"message": "ID must be a valid number",
			})
			return
		}

		var req services.UpdateFilterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		filter, err := serviceContainer.FilterService.UpdateFilter(uint(id), &req)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "Filter not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Failed to update filter",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data":    filter,
			"message": "Filter updated successfully",
		})
	}
}

// DeleteFilter soft deletes a filter
func DeleteFilter(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid filter ID",
				"message": "ID must be a valid number",
			})
			return
		}

		err = serviceContainer.FilterService.DeleteFilter(uint(id))
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "Filter not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to delete filter",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Filter deleted successfully",
		})
	}
}

// GetLLMFilters returns filters associated with an LLM
func GetLLMFilters(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid LLM ID",
				"message": "ID must be a valid number",
			})
			return
		}

		filters, err := serviceContainer.FilterService.GetFiltersForLLM(uint(id))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to get LLM filters",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": filters,
		})
	}
}

// UpdateLLMFilters updates filter associations for an LLM
func UpdateLLMFilters(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
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
			FilterIDs []uint `json:"filter_ids" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		err = serviceContainer.FilterService.UpdateLLMFilters(uint(id), req.FilterIDs)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "LLM or filter not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to update LLM filters",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "LLM filters updated successfully",
		})
	}
}