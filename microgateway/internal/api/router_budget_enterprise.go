//go:build enterprise
// +build enterprise

package api

import (
	"github.com/TykTechnologies/midsommar/microgateway/internal/api/handlers"
	"github.com/gin-gonic/gin"
)

// registerBudgetRoutes registers budget management routes (Enterprise only)
func registerBudgetRoutes(protected *gin.RouterGroup, config *RouterConfig) {
	budgets := protected.Group("/budgets")
	{
		budgets.GET("", handlers.ListBudgets(config.Services))
		budgets.GET("/:appId/usage", handlers.GetBudgetUsage(config.Services))
		budgets.PUT("/:appId", handlers.UpdateBudget(config.Services))
		budgets.GET("/:appId/history", handlers.GetBudgetHistory(config.Services))
	}
}
