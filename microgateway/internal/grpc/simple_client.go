// internal/grpc/simple_client.go
package grpc

import (
	"context"
	"fmt"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	pb "github.com/TykTechnologies/midsommar/microgateway/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SimpleEdgeClient provides basic edge functionality for testing
type SimpleEdgeClient struct {
	config      *config.Config
	conn        *grpc.ClientConn
	client      pb.ConfigurationSyncServiceClient
	configCache *pb.ConfigurationSnapshot
	
	// Callback for configuration updates
	onConfigChange func(*pb.ConfigurationSnapshot)
}

// NewSimpleEdgeClient creates a basic edge client
func NewSimpleEdgeClient(cfg *config.Config) *SimpleEdgeClient {
	return &SimpleEdgeClient{
		config: cfg,
	}
}

// Start connects to the control server and gets initial configuration
func (c *SimpleEdgeClient) Start() error {
	log.Info().
		Str("control_endpoint", c.config.HubSpoke.ControlEndpoint).
		Str("edge_id", c.config.HubSpoke.EdgeID).
		Msg("Connecting to control server")

	// Connect to control server
	conn, err := grpc.Dial(c.config.HubSpoke.ControlEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to control server: %w", err)
	}

	c.conn = conn
	c.client = pb.NewConfigurationSyncServiceClient(conn)

	// Test basic connectivity
	if err := c.registerWithControl(); err != nil {
		conn.Close()
		return fmt.Errorf("failed to register with control: %w", err)
	}

	log.Info().Msg("Successfully connected to control server")
	return nil
}

// registerWithControl registers this edge with the control server
func (c *SimpleEdgeClient) registerWithControl() error {
	ctx := context.Background()

	// Create registration request
	req := &pb.EdgeRegistrationRequest{
		EdgeId:        c.config.HubSpoke.EdgeID,
		EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
		Version:       "dev",
		BuildHash:     "unknown",
		Metadata: map[string]string{
			"test": "true",
		},
		Health: &pb.HealthStatus{
			Status:    pb.HealthStatus_HEALTHY,
			Message:   "Starting up",
			Timestamp: timestamppb.Now(),
		},
	}

	// Register with control
	resp, err := c.client.RegisterEdge(ctx, req)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("registration rejected: %s", resp.Message)
	}

	// Store initial configuration if provided
	if resp.InitialConfig != nil {
		log.Info().
			Str("version", resp.InitialConfig.Version).
			Int32("llm_count", int32(len(resp.InitialConfig.Llms))).
			Int32("app_count", int32(len(resp.InitialConfig.Apps))).
			Msg("Received initial configuration from control")

		c.configCache = resp.InitialConfig

		// Notify provider if callback is set
		if c.onConfigChange != nil {
			c.onConfigChange(resp.InitialConfig)
		}
	}

	log.Info().
		Str("session_id", resp.SessionId).
		Msg("Successfully registered with control server")

	return nil
}

// SetOnConfigChange sets the callback for configuration changes
func (c *SimpleEdgeClient) SetOnConfigChange(callback func(*pb.ConfigurationSnapshot)) {
	c.onConfigChange = callback
}

// IsConnected returns true if connected to control
func (c *SimpleEdgeClient) IsConnected() bool {
	return c.conn != nil
}

// GetCurrentConfiguration returns the cached configuration
func (c *SimpleEdgeClient) GetCurrentConfiguration() *pb.ConfigurationSnapshot {
	return c.configCache
}

// ValidateTokenOnDemand validates a token by calling the control instance
func (c *SimpleEdgeClient) ValidateTokenOnDemand(token string) (*pb.TokenValidationResponse, error) {
	if c.conn == nil || c.client == nil {
		return nil, fmt.Errorf("not connected to control instance")
	}

	tokenPrefix := token
	if len(token) > 8 {
		tokenPrefix = token[:8]
	}

	log.Info().Str("token_prefix", tokenPrefix).Msg("SimpleEdgeClient: making on-demand token validation request to control")

	ctx := context.Background()

	// Create token validation request
	req := &pb.TokenValidationRequest{
		Token:         token,
		EdgeId:        c.config.HubSpoke.EdgeID,
		EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
	}

	// Call control instance
	resp, err := c.client.ValidateToken(ctx, req)
	if err != nil {
		log.Info().Err(err).Str("token_prefix", tokenPrefix).Msg("SimpleEdgeClient: token validation gRPC call failed")
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	log.Info().
		Str("token_prefix", tokenPrefix).
		Bool("valid", resp.Valid).
		Uint32("app_id", resp.AppId).
		Msg("SimpleEdgeClient: received token validation response from control")

	return resp, nil
}

// Stop closes the connection to control server
func (c *SimpleEdgeClient) Stop() error {
	if c.conn != nil {
		log.Info().Msg("Closing connection to control server")
		return c.conn.Close()
	}
	return nil
}