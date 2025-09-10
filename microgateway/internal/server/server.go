// internal/server/server.go
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/microgateway/internal/api"
	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/gosimple/slug"
	"github.com/rs/zerolog/log"
)

// Server represents the HTTP server
type Server struct {
	config        *config.Config
	services      *services.ServiceContainer
	gateway       aigateway.Gateway
	pluginManager *plugins.PluginManager
	router        *gin.Engine
	server        *http.Server
}

// New creates a new server instance
func New(cfg *config.Config, serviceContainer *services.ServiceContainer, version, buildHash, buildTime string) (*Server, error) {
	// Set gin mode based on log level
	if cfg.IsDevelopment() {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create service adapters for AI Gateway integration
	gatewayServiceAdapter := services.NewGatewayServiceAdapter(
		serviceContainer.GatewayService,
		serviceContainer.Management,
		serviceContainer.AnalyticsService,
		serviceContainer.Crypto,
		serviceContainer.FilterService,
		serviceContainer.PluginService,
	)

	budgetServiceAdapter := services.NewBudgetServiceAdapter(
		serviceContainer.BudgetService,
		serviceContainer.GatewayService,
	)

	// Create analytics handler for microgateway with configuration
	analyticsHandler := services.NewMicrogatewaAnalyticsHandler(serviceContainer.DB, &cfg.Analytics)
	analyticsHandler.SetAsGlobalHandler()

	// Create plugin manager first
	pluginManager := plugins.NewPluginManager(serviceContainer.PluginService)
	log.Info().Msg("Plugin manager created")

	// Create response hooks for plugin processing
	responseHooks := createResponseHooks(pluginManager)

	// Create AI Gateway instance for mounting (not standalone)
	log.Info().Msg("Creating AI Gateway for mounting in management server")
	gateway := aigateway.NewWithAnalytics(
		gatewayServiceAdapter,
		budgetServiceAdapter,
		analyticsHandler, // Use microgateway analytics handler
		&aigateway.Config{
			Port:          cfg.Server.Port, // Same port as management API
			ResponseHooks: responseHooks,
		},
	)
	
	// Manually trigger resource loading since we're mounting, not calling Start()
	log.Info().Msg("Loading AI Gateway resources...")
	if err := gateway.Reload(); err != nil {
		return nil, fmt.Errorf("failed to load AI Gateway resources: %w", err)
	}
	log.Info().Msg("AI Gateway resources loaded successfully")

	// Setup API router with mounted gateway
	routerConfig := &api.RouterConfig{
		AuthProvider:  serviceContainer.AuthProvider,
		Services:      serviceContainer,
		Gateway:       gateway, // Mount gateway back in router
		PluginManager: api.NewPluginManagerAdapter(pluginManager),
		EnableSwagger: cfg.IsDevelopment(),
		EnableMetrics: cfg.Observability.EnableMetrics,
		Version:       version,
		BuildHash:     buildHash,
		BuildTime:     buildTime,
	}

	router := api.SetupRouter(routerConfig)

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	return &Server{
		config:        cfg,
		services:      serviceContainer,
		gateway:       gateway,
		pluginManager: pluginManager,
		router:        router,
		server:        server,
	}, nil
}

// Start starts the unified HTTP server with mounted AI Gateway
func (s *Server) Start() error {
	log.Info().
		Int("port", s.config.Server.Port).
		Msg("Starting unified server with management API and mounted AI Gateway")
	log.Info().
		Str("management_endpoints", "/api/v1/*").
		Str("gateway_endpoints", "/llm/* /tools/* /datasource/*").
		Msg("Available endpoints on single port")

	if s.config.Server.TLSEnabled {
		log.Info().Msg("Starting server with TLS")
		return s.server.ListenAndServeTLS(
			s.config.Server.TLSCertPath,
			s.config.Server.TLSKeyPath,
		)
	}

	log.Info().Msg("Starting server without TLS")
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	log.Info().Msg("Shutting down unified server...")

	// Shutdown plugin manager first
	if s.pluginManager != nil {
		log.Info().Msg("Shutting down plugin manager...")
		if err := s.pluginManager.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("Failed to shutdown plugin manager")
		} else {
			log.Info().Msg("Plugin manager shutdown completed")
		}
	}

	// The AI Gateway is mounted, so shutting down the main server handles everything
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	log.Info().Msg("Server stopped successfully")
	return nil
}

// GetRouter returns the gin router (useful for testing)
func (s *Server) GetRouter() *gin.Engine {
	return s.router
}

// Health returns the health status of the server
func (s *Server) Health() error {
	// Check if services are healthy
	return s.services.Health()
}

// Reload reloads the AI Gateway configuration
func (s *Server) Reload() error {
	log.Info().Msg("Reloading AI Gateway configuration...")
	
	if err := s.gateway.Reload(); err != nil {
		log.Error().Err(err).Msg("Failed to reload AI Gateway configuration")
		return fmt.Errorf("failed to reload AI Gateway: %w", err)
	}
	
	log.Info().Msg("AI Gateway configuration reloaded successfully")
	return nil
}

// createResponseHooks creates response hooks that integrate with the plugin system
func createResponseHooks(pluginManager *plugins.PluginManager) *proxy.ResponseHooks {
	return &proxy.ResponseHooks{
		OnBeforeWriteHeaders: func(headers http.Header, llm *models.LLM, app *models.App) http.Header {
			// Fast path: Execute header-only response plugins for this LLM
			return executeResponseHeadersViaHook(pluginManager, headers, llm, app)
		},
		OnBeforeWrite: func(body []byte, headers http.Header, isStreamChunk bool, llm *models.LLM, app *models.App) ([]byte, http.Header) {
			// Full path: Execute complete response plugins for this LLM
			return executeResponsePluginsViaHook(pluginManager, body, headers, isStreamChunk, llm, app)
		},
	}
}

// executeResponseHeadersViaHook executes header-only response plugin modifications
func executeResponseHeadersViaHook(pluginManager *plugins.PluginManager, headers http.Header, llm *models.LLM, app *models.App) http.Header {
	// Fast path: Check if LLM has response plugins, return quickly if not
	if !hasResponsePluginsForLLM(pluginManager, llm.ID) {
		return nil // No plugins = use original headers
	}
	
	log.Debug().Uint("llm_id", llm.ID).Msg("Header hook: LLM has response plugins, processing headers")
	
	// Convert headers to plugin format
	headerMap := make(map[string]string)
	for key, values := range headers {
		if len(values) > 0 {
			headerMap[key] = values[0]
		}
	}
	
	// Create response data for header processing
	respData := &interfaces.ResponseData{
		RequestID:  generateHookRequestID(),
		StatusCode: 200, // Default, actual status set elsewhere
		Headers:    headerMap,
		Body:       nil, // Headers-only processing
		Context: &interfaces.PluginContext{
			LLMID:   llm.ID,
			LLMSlug: slug.Make(llm.Name), // Generate slug from name same as AI Gateway library
			Vendor:  string(llm.Vendor),
			AppID:   app.ID,
			UserID:  app.UserID,
		},
	}
	
	// Execute response plugins for this LLM
	result, err := pluginManager.ExecutePluginChain(llm.ID, interfaces.HookTypeOnResponse, respData, respData.Context)
	if err != nil {
		log.Error().Err(err).Uint("llm_id", llm.ID).Msg("Header hook: Response plugin chain failed")
		return nil // Use original headers on error
	}
	
	// Extract modified headers from plugin result
	if modifiedResp, ok := result.(*interfaces.ResponseData); ok && modifiedResp.Headers != nil {
		modifiedHeaders := make(http.Header)
		for key, value := range modifiedResp.Headers {
			modifiedHeaders.Set(key, value)
		}
		log.Debug().Int("modified_headers", len(modifiedHeaders)).Uint("llm_id", llm.ID).Msg("Header hook: Response plugins modified headers")
		return modifiedHeaders
	}
	
	return nil // Use original headers
}

// executeResponsePluginsViaHook executes complete response plugins via the AI Gateway hook system
func executeResponsePluginsViaHook(pluginManager *plugins.PluginManager, body []byte, headers http.Header, isStreamChunk bool, llm *models.LLM, app *models.App) ([]byte, http.Header) {
	// Fast path: Check if LLM has response plugins, return quickly if not
	if !hasResponsePluginsForLLM(pluginManager, llm.ID) {
		return nil, nil // No plugins = use originals
	}
	
	log.Debug().Int("body_len", len(body)).Bool("is_stream_chunk", isStreamChunk).Uint("llm_id", llm.ID).Msg("Response hook: LLM has response plugins, processing complete response")
	
	// Convert headers to plugin format
	headerMap := make(map[string]string)
	for key, values := range headers {
		if len(values) > 0 {
			headerMap[key] = values[0]
		}
	}
	
	// Create response data structure for plugins
	respData := &interfaces.ResponseData{
		RequestID:  generateHookRequestID(),
		StatusCode: 200, // Default, actual status set elsewhere
		Headers:    headerMap,
		Body:       body,
		Context: &interfaces.PluginContext{
			LLMID:   llm.ID,
			LLMSlug: slug.Make(llm.Name), // Generate slug from name same as AI Gateway library
			Vendor:  string(llm.Vendor),
			AppID:   app.ID,
			UserID:  app.UserID,
		},
	}
	
	// Execute response plugins for this LLM
	result, err := pluginManager.ExecutePluginChain(llm.ID, interfaces.HookTypeOnResponse, respData, respData.Context)
	if err != nil {
		log.Error().Err(err).Uint("llm_id", llm.ID).Msg("Response hook: Response plugin chain failed")
		return nil, nil // Use originals on error
	}
	
	// Extract modifications from plugin result
	if modifiedResp, ok := result.(*interfaces.ResponseData); ok {
		var modifiedBody []byte
		var modifiedHeaders http.Header
		
		// Extract modified body
		if modifiedResp.Body != nil {
			modifiedBody = modifiedResp.Body
		}
		
		// Extract modified headers
		if modifiedResp.Headers != nil {
			modifiedHeaders = make(http.Header)
			for key, value := range modifiedResp.Headers {
				modifiedHeaders.Set(key, value)
			}
		}
		
		log.Debug().
			Int("original_body_len", len(body)).
			Int("modified_body_len", len(modifiedBody)).
			Int("modified_headers", len(modifiedHeaders)).
			Uint("llm_id", llm.ID).
			Bool("is_stream_chunk", isStreamChunk).
			Msg("Response hook: Response plugins completed, returning modifications")
		
		return modifiedBody, modifiedHeaders
	}
	
	log.Debug().Uint("llm_id", llm.ID).Msg("Response hook: No modifications from plugins")
	return nil, nil // Use originals
}

// hasResponsePluginsForLLM checks if the LLM has any active response plugins (fast path optimization)
func hasResponsePluginsForLLM(pluginManager *plugins.PluginManager, llmID uint) bool {
	loadedPlugins, err := pluginManager.GetPluginsForLLM(llmID, interfaces.HookTypeOnResponse)
	if err != nil {
		log.Debug().Err(err).Uint("llm_id", llmID).Msg("Failed to check for response plugins")
		return false
	}
	return len(loadedPlugins) > 0
}

func generateHookRequestID() string {
	return fmt.Sprintf("hook_%d", time.Now().UnixNano())
}