// internal/api/router.go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/api/handlers"
	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
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
		log.Debug().Str("body length is", fmt.Sprintf("%d", c.Request.ContentLength)).Msg("📦 Request body length")

		// Continue processing
		c.Next()
	}
}

// RouterConfig holds configuration for the API router
type RouterConfig struct {
	AuthProvider       auth.AuthProvider
	Services           *services.ServiceContainer
	Gateway            aigateway.Gateway
	PluginManager      PluginManagerInterface
	ReloadCoordinator  *services.ReloadCoordinator
	ModelRouterService *services.ModelRouterService // Enterprise: Model router service
	EnableSwagger      bool
	EnableMetrics      bool
	Version            string
	BuildHash          string
	BuildTime          string
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

		// Model Router endpoints (Enterprise)
		// Routes requests to LLM vendors based on model name patterns
		if config.ModelRouterService != nil {
			log.Debug().Msg("Mounting Model Router handler (Enterprise)")
			modelRouterHandler := services.NewModelRouterHandler(
				config.ModelRouterService,
				func(w http.ResponseWriter, r *http.Request) {
					// Forward to the gateway handler - the mux.SetURLVars has already set routeId
					config.Gateway.Handler().ServeHTTP(w, r)
				},
			)
			// Mount router endpoints - OpenAI-compatible format (uses GinHandler for proper param extraction)
			gateway.POST("/router/:routerSlug/v1/chat/completions", modelRouterHandler.GinHandler())
			gateway.POST("/router/:routerSlug/v1/completions", modelRouterHandler.GinHandler())
		}
	}

	// Custom plugin endpoints - dispatches /plugins/{pluginName}/... to the owning plugin
	if config.PluginManager != nil {
		log.Debug().Msg("Mounting custom plugin endpoint handler at /plugins/*path")
		router.Any("/plugins/*path", handlePluginEndpoint(config))
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

// handlePluginEndpoint dispatches HTTP requests to the appropriate plugin's custom endpoint handler.
// URL format: /plugins/{pluginName}/{subPath...}
func handlePluginEndpoint(config *RouterConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		fullPath := c.Request.URL.Path

		// Parse the path into plugin name + sub-path
		pluginName, subPath, ok := parsePluginPath(fullPath)
		if !ok || pluginName == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "invalid plugin endpoint path"})
			return
		}

		method := c.Request.Method

		// Look up route
		route := config.PluginManager.GetEndpointRoute(method, pluginName, subPath)
		if route == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("no endpoint registered for %s /plugins/%s%s", method, pluginName, subPath)})
			return
		}

		// Get request ID from context
		requestID := ""
		if rid := c.Request.Context().Value("request_id"); rid != nil {
			requestID, _ = rid.(string)
		}

		// Read request body
		var body []byte
		if c.Request.Body != nil {
			var err error
			body, err = io.ReadAll(c.Request.Body)
			if err != nil {
				log.Error().Err(err).Msg("Failed to read plugin endpoint request body")
				c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
				return
			}
		}

		// Collect headers
		hdrs := make(map[string]string)
		for k, vals := range c.Request.Header {
			if len(vals) > 0 {
				hdrs[k] = vals[0]
			}
		}

		// Build path segments from the relative sub-path
		segments := splitPluginPathSegments(subPath)

		// Build the endpoint request
		endpointReq := &pb.EndpointRequest{
			Method:       method,
			Path:         fullPath,
			RelativePath: subPath,
			PathSegments: segments,
			Headers:      hdrs,
			Body:         body,
			QueryString:  c.Request.URL.RawQuery,
			RemoteAddr:   c.ClientIP(),
			Host:         c.Request.Host,
			Protocol:     "http",
			Context: &pb.PluginContext{
				RequestId: requestID,
			},
		}

		// Handle authentication if required
		if route.RequireAuth {
			token := extractBearerToken(c)
			if token == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
				return
			}

			authResult, err := config.AuthProvider.ValidateToken(token)
			if err != nil || !authResult.Valid {
				errMsg := "invalid token"
				if err != nil {
					errMsg = err.Error()
				} else if authResult.Error != "" {
					errMsg = authResult.Error
				}
				c.JSON(http.StatusUnauthorized, gin.H{"error": errMsg})
				return
			}

			endpointReq.Authenticated = true
			endpointReq.Scopes = authResult.Scopes

			// Fetch the full App object for the plugin
			if config.Services != nil && config.Services.Management != nil && authResult.AppID > 0 {
				dbApp, err := config.Services.Management.GetApp(authResult.AppID)
				if err != nil {
					log.Warn().Uint("app_id", authResult.AppID).Err(err).Msg("Failed to fetch app for plugin endpoint auth context")
				} else if dbApp != nil {
					// Convert metadata JSON to map
					metadataMap := make(map[string]string)
					if dbApp.Metadata != nil {
						var rawMeta map[string]interface{}
						if err := json.Unmarshal(dbApp.Metadata, &rawMeta); err == nil {
							for k, v := range rawMeta {
								if s, ok := v.(string); ok {
									metadataMap[k] = s
								}
							}
						}
					}

					endpointReq.App = &pb.App{
						Id:            uint32(dbApp.ID),
						Name:          dbApp.Name,
						Description:   dbApp.Description,
						OwnerEmail:    dbApp.OwnerEmail,
						IsActive:      dbApp.IsActive,
						MonthlyBudget: dbApp.MonthlyBudget,
						RateLimit:     int32(dbApp.RateLimitRPM),
						Metadata:      metadataMap,
					}
				}
			}
		}

		log.Debug().
			Str("plugin_name", pluginName).
			Uint("plugin_id", route.PluginID).
			Str("method", method).
			Str("sub_path", subPath).
			Bool("stream", route.StreamResponse).
			Bool("require_auth", route.RequireAuth).
			Str("request_id", requestID).
			Msg("Dispatching custom plugin endpoint request")

		// Dispatch to plugin
		if route.StreamResponse {
			handlePluginEndpointStream(c, config, route.PluginID, endpointReq)
		} else {
			handlePluginEndpointUnary(c, config, route.PluginID, endpointReq)
		}
	}
}

// handlePluginEndpointUnary handles a non-streaming plugin endpoint request
func handlePluginEndpointUnary(c *gin.Context, config *RouterConfig, pluginID uint, req *pb.EndpointRequest) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	resp, err := config.PluginManager.HandleEndpointRequest(ctx, pluginID, req)
	if err != nil {
		log.Error().Uint("plugin_id", pluginID).Err(err).Msg("Plugin endpoint request failed")
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("plugin endpoint error: %v", err)})
		return
	}

	if resp.ErrorMessage != "" {
		statusCode := int(resp.StatusCode)
		if statusCode == 0 {
			statusCode = http.StatusInternalServerError
		}
		c.JSON(statusCode, gin.H{"error": resp.ErrorMessage})
		return
	}

	// Write response headers
	for k, v := range resp.Headers {
		c.Header(k, v)
	}

	statusCode := int(resp.StatusCode)
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	c.Data(statusCode, c.Writer.Header().Get("Content-Type"), resp.Body)
}

// handlePluginEndpointStream handles a streaming plugin endpoint request (SSE)
func handlePluginEndpointStream(c *gin.Context, config *RouterConfig, pluginID uint, req *pb.EndpointRequest) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	stream, err := config.PluginManager.HandleEndpointRequestStream(ctx, pluginID, req)
	if err != nil {
		log.Error().Uint("plugin_id", pluginID).Err(err).Msg("Plugin streaming endpoint request failed")
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("plugin streaming endpoint error: %v", err)})
		return
	}

	// Get the http.Flusher for streaming
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	headersWritten := false

	for {
		chunk, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			if headersWritten {
				// Already started writing, best effort log
				log.Error().Uint("plugin_id", pluginID).Err(err).Msg("Error during plugin streaming endpoint")
			} else {
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("stream error: %v", err)})
			}
			return
		}

		switch chunk.Type {
		case pb.EndpointResponseChunk_HEADERS:
			// Write status code and headers
			for k, v := range chunk.Headers {
				c.Header(k, v)
			}
			statusCode := int(chunk.StatusCode)
			if statusCode == 0 {
				statusCode = http.StatusOK
			}
			c.Status(statusCode)
			flusher.Flush()
			headersWritten = true

		case pb.EndpointResponseChunk_BODY:
			if !headersWritten {
				c.Status(http.StatusOK)
				headersWritten = true
			}
			if len(chunk.Data) > 0 {
				_, writeErr := c.Writer.Write(chunk.Data)
				if writeErr != nil {
					log.Debug().Err(writeErr).Msg("Client disconnected during plugin stream")
					return
				}
				flusher.Flush()
			}

		case pb.EndpointResponseChunk_DONE:
			// Stream complete
			return

		case pb.EndpointResponseChunk_ERROR:
			if !headersWritten {
				c.JSON(http.StatusInternalServerError, gin.H{"error": chunk.ErrorMessage})
			} else {
				log.Error().Str("error", chunk.ErrorMessage).Msg("Error chunk received during plugin stream")
			}
			return
		}
	}
}

// --- Router-local helper functions ---

// parsePluginPath splits /plugins/{pluginName}/{subPath...} into (pluginName, subPath).
func parsePluginPath(fullPath string) (pluginName string, subPath string, ok bool) {
	trimmed := strings.TrimPrefix(fullPath, "/plugins/")
	if trimmed == fullPath {
		return "", "", false
	}

	slashIdx := strings.Index(trimmed, "/")
	if slashIdx < 0 {
		// /plugins/{pluginName} with no trailing sub-path
		if trimmed == "" {
			return "", "", false
		}
		return trimmed, "/", true
	}

	pluginName = trimmed[:slashIdx]
	subPath = trimmed[slashIdx:]

	if pluginName == "" {
		return "", "", false
	}
	return pluginName, subPath, true
}

// splitPluginPathSegments splits "/users/123/profile" into ["users", "123", "profile"].
func splitPluginPathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// extractBearerToken extracts a Bearer token from the Authorization header or query param.
func extractBearerToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return c.Query("token")
	}
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return authHeader
}
