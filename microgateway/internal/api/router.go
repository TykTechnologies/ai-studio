// internal/api/router.go
package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
	"github.com/TykTechnologies/midsommar/microgateway/internal/api/handlers"
	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// RequestIDMiddleware generates a canonical request ID for every request
// This MUST be the first middleware in the chain to ensure all downstream code uses the same ID
// The request ID is stored in request context and available to all handlers, middleware, and plugins
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if request ID already exists (shouldn't, but defensive)
		if existingID := c.Request.Context().Value("request_id"); existingID != nil {
			log.Warn().Str("existing_id", existingID.(string)).Msg("⚠️ Request ID already in context - skipping regeneration")
			c.Next()
			return
		}

		// Generate canonical request ID - ignore any client-provided X-Request-ID header for security
		requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())

		// Store in request context so all middleware/handlers/plugins can access it
		ctx := context.WithValue(c.Request.Context(), "request_id", requestID)
		c.Request = c.Request.WithContext(ctx)

		// Set as response header for client observability and distributed tracing
		c.Header("X-Request-ID", requestID)

		log.Debug().Str("request_id", requestID).Str("path", c.Request.URL.Path).Msg("🆔 Generated canonical request ID for request")

		// Continue processing
		c.Next()
	}
}

// RouterConfig holds configuration for the API router
type RouterConfig struct {
	AuthProvider     auth.AuthProvider
	Services         *services.ServiceContainer
	Gateway          aigateway.Gateway
	PluginManager    PluginManagerInterface
	ReloadCoordinator *services.ReloadCoordinator
	EnableSwagger    bool
	EnableMetrics    bool
	Version          string
	BuildHash        string
	BuildTime        string
}

// SetupRouter configures and returns the main application router
func SetupRouter(config *RouterConfig) *gin.Engine {
	// Set Gin to release mode to reduce noise
	gin.SetMode(gin.ReleaseMode)

	// Use gin.New() instead of gin.Default() to control middleware
	// gin.Default() adds Logger and Recovery middleware automatically
	// We only want Recovery (handles panics) - no request logging
	router := gin.New()
	router.Use(gin.Recovery())

	// CRITICAL: Add request ID middleware FIRST (before all other routes)
	// This ensures ALL requests (gateway, API, health checks) get a canonical request ID
	router.Use(RequestIDMiddleware())

	// Health endpoints (no auth required)
	router.GET("/health", handlers.HealthCheck(config.Services))
	router.GET("/ready", handlers.ReadinessCheck(config.Services))
	router.GET("/health/detailed", handlers.GetDetailedHealthStatus(config.Services))

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

		// Budget management (Enterprise only)
		registerBudgetRoutes(protected, config)

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

			// Plugin health endpoints
			plugins.GET("/health", handlers.GetPluginHealth(config.Services))
			plugins.GET("/oci/status", handlers.GetOCIPluginStatus(config.Services))
			plugins.POST("/prewarm", handlers.TriggerPluginPreWarm(config.Services))
		}

		// System management endpoints
		system := protected.Group("/system")
		{
			system.POST("/reload", handlers.ReloadConfiguration(config.Gateway))
			system.GET("/info", handlers.GetSystemInfo(config.Services, config.Version, config.BuildHash, config.BuildTime))
		}

		// Hub-and-spoke management endpoints (TODO: Wire reload coordinator)
		if config.ReloadCoordinator != nil {
			namespace := protected.Group("/namespace")
			{
				namespace.POST("/reload", handlers.InitiateNamespaceReload(config.ReloadCoordinator))
				namespace.GET("/reload/operations", handlers.ListActiveReloadOperations(config.ReloadCoordinator))
				namespace.GET("/reload/:operation_id/status", handlers.GetReloadOperationStatus(config.ReloadCoordinator))
			}

			edge := protected.Group("/edge")
			{
				edge.POST("/reload", handlers.InitiateEdgeReload(config.ReloadCoordinator))
				edge.GET("/status", handlers.GetEdgeInstanceStatus(config.Services))
				edge.GET("/:edge_id/status", handlers.GetSingleEdgeStatus(config.Services))
				edge.GET("/reload/:operation_id/status", handlers.GetReloadOperationStatus(config.ReloadCoordinator))
			}
		}
	}

	// Gateway endpoints - mount the AI Gateway handler
	// Plugins are now integrated via hooks in the proxy layer, so router is a simple passthrough
	if config.Gateway != nil {
		gateway := router.Group("/")

		log.Debug().Msg("Mounting AI Gateway handler (plugins integrated via hooks)")
		gateway.Any("/llm/*path", gin.WrapH(config.Gateway.Handler()))
		gateway.Any("/tools/*path", gin.WrapH(config.Gateway.Handler()))
		gateway.Any("/datasource/*path", gin.WrapH(config.Gateway.Handler()))
		gateway.Any("/ai/*path", gin.WrapH(config.Gateway.Handler()))
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