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
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// Server represents the HTTP server
type Server struct {
	config   *config.Config
	services *services.ServiceContainer
	gateway  aigateway.Gateway
	router   *gin.Engine
	server   *http.Server
}

// New creates a new server instance
func New(cfg *config.Config, serviceContainer *services.ServiceContainer) (*Server, error) {
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
	)

	budgetServiceAdapter := services.NewBudgetServiceAdapter(
		serviceContainer.BudgetService,
		serviceContainer.GatewayService,
	)

	// Create analytics handler for microgateway with configuration
	analyticsHandler := services.NewMicrogatewaAnalyticsHandler(serviceContainer.DB, &cfg.Analytics)
	analyticsHandler.SetAsGlobalHandler()

	// Create AI Gateway instance for mounting (not standalone)
	log.Info().Msg("Creating AI Gateway for mounting in management server")
	gateway := aigateway.NewWithAnalytics(
		gatewayServiceAdapter,
		budgetServiceAdapter,
		analyticsHandler, // Use microgateway analytics handler
		&aigateway.Config{Port: cfg.Server.Port}, // Same port as management API
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
		EnableSwagger: cfg.IsDevelopment(),
		EnableMetrics: cfg.Observability.EnableMetrics,
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
		config:   cfg,
		services: serviceContainer,
		gateway:  gateway,
		router:   router,
		server:   server,
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