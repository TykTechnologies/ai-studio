// internal/grpc/simple_server.go
package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

// SimpleControlServer provides a minimal gRPC server for testing
type SimpleControlServer struct {
	config     *config.Config
	grpcServer *grpc.Server
}

// NewSimpleControlServer creates a basic control server for testing
func NewSimpleControlServer(cfg *config.Config) *SimpleControlServer {
	return &SimpleControlServer{
		config: cfg,
	}
}

// Start starts the gRPC server
func (s *SimpleControlServer) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.HubSpoke.GRPCHost, s.config.HubSpoke.GRPCPort)
	
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	
	s.grpcServer = grpc.NewServer()
	
	log.Info().Str("address", addr).Msg("Starting simple gRPC control server (testing)")
	
	if err := s.grpcServer.Serve(listener); err != nil {
		return fmt.Errorf("gRPC server failed: %w", err)
	}
	
	return nil
}

// Stop stops the gRPC server
func (s *SimpleControlServer) Stop() {
	if s.grpcServer != nil {
		log.Info().Msg("Stopping simple gRPC control server")
		s.grpcServer.GracefulStop()
	}
}

// StartControlServerIfNeeded starts a gRPC control server if in control mode
func StartControlServerIfNeeded(ctx context.Context, cfg *config.Config) *SimpleControlServer {
	if !cfg.IsControl() {
		return nil
	}
	
	log.Info().
		Int("grpc_port", cfg.HubSpoke.GRPCPort).
		Msg("Starting gRPC control server")
	
	server := NewSimpleControlServer(cfg)
	
	go func() {
		if err := server.Start(); err != nil {
			log.Error().Err(err).Msg("gRPC control server failed")
		}
	}()
	
	return server
}