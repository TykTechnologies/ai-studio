// internal/api/router.go
package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
	"github.com/TykTechnologies/midsommar/microgateway/internal/api/handlers"
	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// RouterConfig holds configuration for the API router
type RouterConfig struct {
	AuthProvider  auth.AuthProvider
	Services      *services.ServiceContainer
	Gateway       aigateway.Gateway
	PluginManager PluginManagerInterface
	EnableSwagger bool
	EnableMetrics bool
	Version       string
	BuildHash     string
	BuildTime     string
}

// SetupRouter configures and returns the main application router
func SetupRouter(config *RouterConfig) *gin.Engine {
	// Use gin.Default() which includes logging and recovery middleware
	router := gin.Default()

	// Health endpoints (no auth required)
	router.GET("/health", handlers.HealthCheck(config.Services))
	router.GET("/ready", handlers.ReadinessCheck(config.Services))

	// Root endpoint
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service":    "microgateway",
			"version":    config.Version,
			"build_hash": config.BuildHash,
			"build_time": config.BuildTime,
			"status":     "running",
		})
	})

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Public endpoints
		v1.POST("/auth/token", handlers.GenerateToken(config.Services))
		v1.POST("/auth/validate", handlers.ValidateTokenEndpoint(config.Services))

		// Protected management endpoints
		protected := v1.Group("/")
		protected.Use(auth.RequireAuth(config.AuthProvider))
		protected.Use(auth.RequireScopes("admin"))

		// LLM management
		llms := protected.Group("/llms")
		{
			llms.GET("", handlers.ListLLMs(config.Services))
			llms.POST("", handlers.CreateLLM(config.Services))
			llms.GET("/:id", handlers.GetLLM(config.Services))
			llms.PUT("/:id", handlers.UpdateLLM(config.Services))
			llms.DELETE("/:id", handlers.DeleteLLM(config.Services))
			llms.GET("/:id/stats", handlers.GetLLMStats(config.Services))
		}

		// App management
		apps := protected.Group("/apps")
		{
			apps.GET("", handlers.ListApps(config.Services))
			apps.POST("", handlers.CreateApp(config.Services))
			apps.GET("/:id", handlers.GetApp(config.Services))
			apps.PUT("/:id", handlers.UpdateApp(config.Services))
			apps.DELETE("/:id", handlers.DeleteApp(config.Services))

			// LLM associations
			apps.GET("/:id/llms", handlers.GetAppLLMs(config.Services))
			apps.PUT("/:id/llms", handlers.UpdateAppLLMs(config.Services))
		}

		// Budget management
		budgets := protected.Group("/budgets")
		{
			budgets.GET("", handlers.ListBudgets(config.Services))
			budgets.GET("/:appId/usage", handlers.GetBudgetUsage(config.Services))
			budgets.PUT("/:appId", handlers.UpdateBudget(config.Services))
			budgets.GET("/:appId/history", handlers.GetBudgetHistory(config.Services))
		}

		// Token management
		tokens := protected.Group("/tokens")
		{
			tokens.GET("", handlers.ListTokens(config.Services))
			tokens.POST("", handlers.CreateToken(config.Services))
			tokens.DELETE("/:token", handlers.RevokeToken(config.Services))
			tokens.GET("/:token", handlers.GetTokenInfo(config.Services))
		}

		// Analytics
		analytics := protected.Group("/analytics")
		{
			analytics.GET("/events", handlers.GetAnalyticsEvents(config.Services))
			analytics.GET("/events/:id/request", handlers.GetAnalyticsEventRequest(config.Services))
			analytics.GET("/events/:id/response", handlers.GetAnalyticsEventResponse(config.Services))
			analytics.GET("/summary", handlers.GetAnalyticsSummary(config.Services))
			analytics.GET("/costs", handlers.GetCostAnalysis(config.Services))
		}

		// Model Pricing management
		pricing := protected.Group("/pricing")
		{
			pricing.GET("", handlers.ListModelPrices(config.Services))
			pricing.POST("", handlers.CreateModelPrice(config.Services))
			pricing.GET("/:id", handlers.GetModelPrice(config.Services))
			pricing.PUT("/:id", handlers.UpdateModelPrice(config.Services))
			pricing.DELETE("/:id", handlers.DeleteModelPrice(config.Services))
		}

		// Filter management
		filters := protected.Group("/filters")
		{
			filters.GET("", handlers.ListFilters(config.Services))
			filters.POST("", handlers.CreateFilter(config.Services))
			filters.GET("/:id", handlers.GetFilter(config.Services))
			filters.PUT("/:id", handlers.UpdateFilter(config.Services))
			filters.DELETE("/:id", handlers.DeleteFilter(config.Services))
		}

		// LLM-Filter associations (extend existing LLM routes)
		llms.GET("/:id/filters", handlers.GetLLMFilters(config.Services))
		llms.PUT("/:id/filters", handlers.UpdateLLMFilters(config.Services))

		// LLM-Plugin associations (extend existing LLM routes)
		llms.GET("/:id/plugins", handlers.GetLLMPlugins(config.Services))
		llms.PUT("/:id/plugins", handlers.UpdateLLMPlugins(config.Services))

		// Plugin management
		plugins := protected.Group("/plugins")
		{
			plugins.GET("", handlers.ListPlugins(config.Services))
			plugins.POST("", handlers.CreatePlugin(config.Services))
			plugins.GET("/:id", handlers.GetPlugin(config.Services))
			plugins.PUT("/:id", handlers.UpdatePlugin(config.Services))
			plugins.DELETE("/:id", handlers.DeletePlugin(config.Services))
			plugins.POST("/:id/test", handlers.TestPlugin(config.Services))
		}

		// System management endpoints
		system := protected.Group("/system")
		{
			system.POST("/reload", handlers.ReloadConfiguration(config.Gateway))
			system.GET("/info", handlers.GetSystemInfo(config.Services, config.Version, config.BuildHash, config.BuildTime))
		}
	}

	// Gateway endpoints - mount the AI Gateway handler with middleware
	if config.Gateway != nil {
		gateway := router.Group("/")
		
		log.Debug().Bool("has_plugin_manager", config.PluginManager != nil).Msg("Router setup - checking plugin manager availability")
		
		// Create plugin-aware handlers for LLM endpoints
		if config.PluginManager != nil {
			log.Info().Msg("Adding plugin-aware handlers for gateway endpoints")
			log.Debug().Msg("Plugin manager is available for router setup")
			pluginMiddlewareConfig := &PluginMiddlewareConfig{
				PluginManager: config.PluginManager,
				Services:      config.Services,
			}
			
			// Create plugin-aware LLM handler that bypasses AI Gateway auth when auth plugins are configured
			gateway.Any("/llm/*path", CreatePluginAwareLLMHandler(config.Gateway.Handler(), pluginMiddlewareConfig))
			
			// Tools and datasources don't need plugin processing
			gateway.Any("/tools/*path", gin.WrapH(config.Gateway.Handler()))
			gateway.Any("/datasource/*path", gin.WrapH(config.Gateway.Handler()))
		} else {
			// No plugin manager, use standard handlers
			gateway.Any("/llm/*path", gin.WrapH(config.Gateway.Handler()))
			gateway.Any("/tools/*path", gin.WrapH(config.Gateway.Handler()))
			gateway.Any("/datasource/*path", gin.WrapH(config.Gateway.Handler()))
		}
	}

	// Metrics endpoint if enabled
	if config.EnableMetrics {
		router.GET("/metrics", handlers.PrometheusMetrics())
	}

	// Swagger documentation if enabled
	if config.EnableSwagger {
		router.GET("/swagger/*any", handlers.SwaggerHandler())
	}

	return router
}