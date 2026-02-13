// internal/server/server.go
package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
	"github.com/TykTechnologies/midsommar/microgateway/internal/api"
	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/gin-gonic/gin"
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
	
	// Build information
	version   string
	buildHash string
	buildTime string
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
		serviceContainer.PluginManager,
		serviceContainer.DB,
	)

	budgetServiceAdapter := services.NewBudgetServiceAdapter(
		serviceContainer.BudgetService,
		serviceContainer.GatewayService,
	)

	// Use plugin manager from service container (already loaded with global plugins)
	pluginManager := serviceContainer.PluginManager

	// Create analytics handler for microgateway with plugin manager that has loaded plugins
	analyticsHandler := services.NewMicrogatewaAnalyticsHandler(serviceContainer.DB, &cfg.Analytics, pluginManager, serviceContainer.BudgetService)
	analyticsHandler.SetAsGlobalHandler()

	// Debug: Verify plugin manager state after service container initialization
	proxyPlugins := pluginManager.GetGlobalPluginsForHookType("proxy_log")
	analyticsPlugins := pluginManager.GetGlobalPluginsForHookType("analytics")
	budgetPlugins := pluginManager.GetGlobalPluginsForHookType("budget")
	
	log.Debug().
		Int("proxy_plugins", len(proxyPlugins)).
		Int("analytics_plugins", len(analyticsPlugins)).
		Int("budget_plugins", len(budgetPlugins)).
		Msg("Plugin manager state verification in server")

	log.Debug().Msg("Analytics handler configured with plugin manager")

	// Note: Response hooks are implemented directly in the AI Gateway, not in microgateway plugin system

	// Create AI Gateway instance for mounting (not standalone)
	log.Debug().Msg("Creating AI Gateway for mounting in management server")
	gateway := aigateway.NewWithAnalytics(
		gatewayServiceAdapter,
		budgetServiceAdapter,
		analyticsHandler, // Use microgateway analytics handler
		&aigateway.Config{Port: cfg.Server.Port}, // Same port as management API
	)
	
	// Manually trigger resource loading since we're mounting, not calling Start()
	log.Debug().Msg("Loading AI Gateway resources...")
	if err := gateway.Reload(); err != nil {
		return nil, fmt.Errorf("failed to load AI Gateway resources: %w", err)
	}
	log.Debug().Msg("AI Gateway resources loaded successfully")

	// Create and register gRPC response plugin adapter with AI Gateway
	log.Debug().Msg("Setting up gRPC response plugin adapter")
	responsePluginAdapter := api.NewGRPCResponsePluginAdapter(serviceContainer, pluginManager)
	gateway.AddResponseHook(responsePluginAdapter)
	log.Debug().Msg("gRPC response plugin adapter registered with AI Gateway")

	// Register authentication hooks for executing pre-auth, auth, and post-auth plugins
	log.Debug().Msg("Setting up authentication hooks for plugin execution")
	authHooks := api.CreateAuthHooks(serviceContainer, pluginManager)
	gateway.SetAuthHooks(authHooks)
	log.Debug().Msg("Authentication hooks registered with AI Gateway")

	// Setup API router with mounted gateway
	routerConfig := &api.RouterConfig{
		AuthProvider:       serviceContainer.AuthProvider,
		Services:           serviceContainer,
		Gateway:            gateway, // Mount gateway back in router
		PluginManager:      api.NewPluginManagerAdapter(pluginManager),
		ReloadCoordinator:  nil, // Will be set for control instances
		ModelRouterService: serviceContainer.ModelRouterService, // Enterprise: Model router
		EnableSwagger:      cfg.IsDevelopment(),
		EnableMetrics:      cfg.Observability.EnableMetrics,
		Version:            version,
		BuildHash:          buildHash,
		BuildTime:          buildTime,
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

	srv := &Server{
		config:        cfg,
		services:      serviceContainer,
		gateway:       gateway,
		pluginManager: pluginManager,
		router:        router,
		server:        server,
		version:       version,
		buildHash:     buildHash,
		buildTime:     buildTime,
	}

	return srv, nil
}

// SetReloadCoordinator sets the reload coordinator for hub-and-spoke operations (control mode only)
func (s *Server) SetReloadCoordinator(reloadCoordinator *services.ReloadCoordinator) {
	// Since router is already created, we need to recreate it with the reload coordinator
	log.Debug().Msg("Recreating router with reload coordinator for hub-and-spoke endpoints")
	
	// Update router config with reload coordinator
	routerConfig := &api.RouterConfig{
		AuthProvider:     s.services.AuthProvider,
		Services:         s.services,
		Gateway:          s.gateway,
		PluginManager:    api.NewPluginManagerAdapter(s.pluginManager),
		ReloadCoordinator: reloadCoordinator, // Add reload coordinator
		EnableSwagger:    s.config.IsDevelopment(),
		EnableMetrics:    s.config.Observability.EnableMetrics,
		Version:          s.version,
		BuildHash:        s.buildHash,
		BuildTime:        s.buildTime,
	}

	// Recreate router with reload coordinator
	s.router = api.SetupRouter(routerConfig)
	s.server.Handler = s.router
	
	log.Debug().Msg("Router recreated with reload coordinator - hub-and-spoke endpoints now available")
}

// Start starts the unified HTTP server with mounted AI Gateway
func (s *Server) Start() error {
	log.Debug().
		Int("port", s.config.Server.Port).
		Msg("Starting unified server with management API and mounted AI Gateway")
	log.Debug().
		Str("management_endpoints", "/api/v1/*").
		Str("gateway_endpoints", "/llm/* /tools/* /datasource/*").
		Msg("Available endpoints on single port")

	if s.config.Server.TLSEnabled {
		log.Debug().Msg("Starting server with TLS")
		return s.server.ListenAndServeTLS(
			s.config.Server.TLSCertPath,
			s.config.Server.TLSKeyPath,
		)
	}

	log.Debug().Msg("Starting server without TLS")
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	log.Debug().Msg("Shutting down unified server...")

	// Shutdown plugin manager first
	if s.pluginManager != nil {
		log.Debug().Msg("Shutting down plugin manager...")
		if err := s.pluginManager.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("Failed to shutdown plugin manager")
		} else {
			log.Debug().Msg("Plugin manager shutdown completed")
		}
	}

	// The AI Gateway is mounted, so shutting down the main server handles everything
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	log.Debug().Msg("Server stopped successfully")
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
	log.Debug().Msg("Reloading AI Gateway configuration...")
	
	if err := s.gateway.Reload(); err != nil {
		log.Error().Err(err).Msg("Failed to reload AI Gateway configuration")
		return fmt.Errorf("failed to reload AI Gateway: %w", err)
	}
	
	log.Debug().Msg("AI Gateway configuration reloaded successfully")
	return nil
}