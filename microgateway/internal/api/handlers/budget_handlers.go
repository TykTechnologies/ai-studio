//go:build enterprise
// +build enterprise

// internal/api/handlers/budget_handlers.go
package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
)

// GetBudgetUsage returns current budget usage for an app
func GetBudgetUsage(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		appID, err := strconv.ParseUint(c.Param("appId"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid app ID",
				"message": "app_id must be a valid number",
			})
			return
		}

		// Parse optional LLM ID
		var llmID *uint
		if llmIDParam := c.Query("llm_id"); llmIDParam != "" {
			id, err := strconv.ParseUint(llmIDParam, 10, 32)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Invalid LLM ID",
					"message": "llm_id must be a valid number",
				})
				return
			}
			llmIDVal := uint(id)
			llmID = &llmIDVal
		}

		// Get budget status
		status, err := serviceContainer.BudgetService.GetBudgetStatus(uint(appID), llmID)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "App not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to get budget usage",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": status,
		})
	}
}

// GetBudgetHistory returns budget usage history
func GetBudgetHistory(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		appID, err := strconv.ParseUint(c.Param("appId"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid app ID",
				"message": "app_id must be a valid number",
			})
			return
		}

		// Parse time range
		startTimeStr := c.DefaultQuery("start_time", time.Now().AddDate(0, -1, 0).Format(time.RFC3339))
		endTimeStr := c.DefaultQuery("end_time", time.Now().Format(time.RFC3339))

		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid start_time",
				"message": "start_time must be in RFC3339 format",
			})
			return
		}

		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid end_time",
				"message": "end_time must be in RFC3339 format",
			})
			return
		}

		// Parse optional LLM ID
		var llmID *uint
		if llmIDParam := c.Query("llm_id"); llmIDParam != "" {
			id, err := strconv.ParseUint(llmIDParam, 10, 32)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Invalid LLM ID",
					"message": "llm_id must be a valid number",
				})
				return
			}
			llmIDVal := uint(id)
			llmID = &llmIDVal
		}

		// Get budget history
		history, err := serviceContainer.BudgetService.GetBudgetHistory(uint(appID), llmID, startTime, endTime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to get budget history",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": history,
			"time_range": gin.H{
				"start_time": startTime,
				"end_time":   endTime,
			},
		})
	}
}

// UpdateBudget updates budget settings for an app
func UpdateBudget(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		appID, err := strconv.ParseUint(c.Param("appId"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid app ID",
				"message": "app_id must be a valid number",
			})
			return
		}

		var req struct {
			MonthlyBudget  float64 `json:"monthly_budget" binding:"required,min=0"`
			BudgetResetDay int     `json:"budget_reset_day,omitempty"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		// Validate reset day
		if req.BudgetResetDay == 0 {
			req.BudgetResetDay = 1
		}
		if req.BudgetResetDay < 1 || req.BudgetResetDay > 28 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid budget reset day",
				"message": "reset_day must be between 1 and 28",
			})
			return
		}

		// Update budget
		err = serviceContainer.BudgetService.UpdateBudget(uint(appID), req.MonthlyBudget, req.BudgetResetDay)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "App not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to update budget",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Budget updated successfully",
		})
	}
}

// ListBudgets lists budget information for all apps (admin only)
func ListBudgets(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get budget summary using budget service
		if budgetService, ok := serviceContainer.BudgetService.(*services.DatabaseBudgetService); ok {
			summary, err := budgetService.GetBudgetSummary()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "Failed to get budget summary",
					"message": err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"data": summary,
			})
		} else {
			c.JSON(http.StatusNotImplemented, gin.H{
				"error": "Budget summary not available",
			})
		}
	}
}