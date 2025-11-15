//go:build !enterprise
// +build !enterprise

package api

import (
	"github.com/gin-gonic/gin"
)

// registerBudgetRoutes is a no-op in Community Edition
func registerBudgetRoutes(protected *gin.RouterGroup, config *RouterConfig) {
	// CE: Budget management routes not available
	// Routes return 402 Payment Required if called
}
