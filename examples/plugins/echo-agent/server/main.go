package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

//go:embed plugin.manifest.json
var manifestFile []byte

//go:embed config.schema.json
var configSchemaFile []byte

type EchoAgentPlugin struct {
	pb.UnimplementedPluginServiceServer
	pluginID        uint32
	prefix          string
	suffix          string
	includeMetadata bool
}

type Config struct {
	Prefix          string `json:"prefix"`
	Suffix          string `json:"suffix"`
	IncludeMetadata bool   `json:"include_metadata"`
}

func (p *EchoAgentPlugin) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	log.Println("EchoAgent: Initialize called")

	// Set defaults
	p.prefix = "<<"
	p.suffix = ">>"
	p.includeMetadata = false

	// Extract plugin ID from config
	if pluginIDStr, ok := req.Config["plugin_id"]; ok {
		fmt.Sscanf(pluginIDStr, "%d", &p.pluginID)
		log.Printf("EchoAgent: Plugin ID set to %d", p.pluginID)
	}

	// Parse custom configuration
	if configJSON, ok := req.Config["config"]; ok {
		var config Config
		if err := json.Unmarshal([]byte(configJSON), &config); err == nil {
			if config.Prefix != "" {
				p.prefix = config.Prefix
			}
			if config.Suffix != "" {
				p.suffix = config.Suffix
			}
			p.includeMetadata = config.IncludeMetadata
			log.Printf("EchoAgent: Using custom config - prefix: %s, suffix: %s, metadata: %v",
				p.prefix, p.suffix, p.includeMetadata)
		}
	}

	return &pb.InitResponse{
		Success: true,
	}, nil
}

func (p *EchoAgentPlugin) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{
		Timestamp: req.Timestamp,
		Healthy:   true,
	}, nil
}

func (p *EchoAgentPlugin) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Println("EchoAgent: Shutdown called")
	return &pb.ShutdownResponse{
		Success: true,
	}, nil
}

func (p *EchoAgentPlugin) GetManifest(ctx context.Context, req *pb.GetManifestRequest) (*pb.GetManifestResponse, error) {
	return &pb.GetManifestResponse{
		Success:      true,
		ManifestJson: string(manifestFile),
	}, nil
}

func (p *EchoAgentPlugin) GetConfigSchema(ctx context.Context, req *pb.GetConfigSchemaRequest) (*pb.GetConfigSchemaResponse, error) {
	return &pb.GetConfigSchemaResponse{
		Success:    true,
		SchemaJson: string(configSchemaFile),
	}, nil
}

func (p *EchoAgentPlugin) HandleAgentMessage(req *pb.AgentMessageRequest, stream grpc.ServerStreamingServer[pb.AgentMessageChunk]) error {
	log.Printf("EchoAgent: Received message: %s", req.UserMessage)

	// Simply echo back the user message wrapped with configured prefix/suffix
	// This is a test plugin to verify the agent flow works end-to-end
	wrappedContent := fmt.Sprintf("%s %s %s", p.prefix, req.UserMessage, p.suffix)
	log.Printf("EchoAgent: Sending wrapped response: %s", wrappedContent)

	// Send content chunk
	if err := stream.Send(&pb.AgentMessageChunk{
		Type:    pb.AgentMessageChunk_CONTENT,
		Content: wrappedContent,
		IsFinal: false,
	}); err != nil {
		return err
	}

	// Send done chunk
	return stream.Send(&pb.AgentMessageChunk{
		Type:    pb.AgentMessageChunk_DONE,
		Content: "completed",
		IsFinal: true,
	})
}

// AgentPluginGRPC implements the go-plugin Plugin interface for agent plugins
type AgentPluginGRPC struct {
	goplugin.NetRPCUnsupportedPlugin
	Impl pb.PluginServiceServer
}

func (p *AgentPluginGRPC) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterPluginServiceServer(s, p.Impl)
	return nil
}

func (p *AgentPluginGRPC) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return pb.NewPluginServiceClient(c), nil
}

func main() {
	log.Printf("🤖 Starting Echo Agent Plugin")

	plugin := &EchoAgentPlugin{}

	// Agent plugins serve directly without SDK wrapper
	// The SDK wrapper is designed for UI plugins, not agent plugins
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: goplugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "AI_STUDIO_PLUGIN",
			MagicCookieValue: "v1",
		},
		Plugins: map[string]goplugin.Plugin{
			"plugin": &AgentPluginGRPC{Impl: plugin},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}
