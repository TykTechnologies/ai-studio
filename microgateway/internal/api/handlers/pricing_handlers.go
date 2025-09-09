// internal/api/handlers/pricing_handlers.go
package handlers

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
)

// ListModelPrices returns paginated list of model prices
func ListModelPrices(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		vendor := c.Query("vendor")

		prices, err := serviceContainer.Management.ListModelPrices(vendor)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to list model prices",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": prices,
		})
	}
}

// CreateModelPrice creates a new model price configuration
func CreateModelPrice(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req services.CreateModelPriceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		price, err := serviceContainer.Management.CreateModelPrice(&req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to create model price",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"data":    price,
			"message": "Model price created successfully",
		})
	}
}

// GetModelPrice retrieves a specific model price by ID
func GetModelPrice(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid price ID",
				"message": "ID must be a valid number",
			})
			return
		}

		var price database.ModelPrice
		if err := serviceContainer.DB.First(&price, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Model price not found",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": price,
		})
	}
}

// UpdateModelPrice updates an existing model price
func UpdateModelPrice(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid price ID",
				"message": "ID must be a valid number",
			})
			return
		}

		var req services.UpdateModelPriceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		price, err := serviceContainer.Management.UpdateModelPrice(uint(id), &req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to update model price",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data":    price,
			"message": "Model price updated successfully",
		})
	}
}

// DeleteModelPrice deletes a model price
func DeleteModelPrice(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid price ID",
				"message": "ID must be a valid number",
			})
			return
		}

		err = serviceContainer.Management.DeleteModelPrice(uint(id))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to delete model price",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Model price deleted successfully",
		})
	}
}