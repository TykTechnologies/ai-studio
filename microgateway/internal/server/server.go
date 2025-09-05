// internal/server/server.go
package server

import (
	"context"
	"fmt"
	"net/http"

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

	// Setup API router
	routerConfig := &api.RouterConfig{
		AuthProvider:  serviceContainer.AuthProvider,
		Services:      serviceContainer,
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
		router:   router,
		server:   server,
	}, nil
}

// Start starts the HTTP server
func (s *Server) Start() error {
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
	log.Info().Msg("Shutting down HTTP server...")

	// Stop accepting new connections and close existing ones
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	log.Info().Msg("HTTP server stopped")
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