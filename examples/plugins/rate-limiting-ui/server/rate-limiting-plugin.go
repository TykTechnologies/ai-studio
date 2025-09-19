package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	configpb "github.com/TykTechnologies/midsommar/v2/proto/configpb"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// Embed UI assets and manifest into the binary
//
//go:embed ui assets
var embeddedAssets embed.FS

//go:embed plugin.manifest.json
var manifestFile []byte

// RateLimitingUIPlugin implements the AI Studio plugin interface with embedded UI assets
type RateLimitingUIPlugin struct {
	pb.UnimplementedPluginServiceServer
	kvStore map[string]interface{}
}

// === Lifecycle Methods ===

func (p *RateLimitingUIPlugin) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	log.Printf("Initializing Rate Limiting UI Plugin with config: %v", req.Config)

	// Initialize KV store with default data
	p.kvStore = map[string]interface{}{
		"global_settings": map[string]interface{}{
			"storage_type":       "redis",
			"redis_url":          "redis://localhost:6379",
			"default_limit":      1000,
			"default_window":     "1h",
			"enable_burst":       true,
			"burst_multiplier":   2.0,
			"monitoring_enabled": true,
			"alert_threshold":    0.8,
		},
		"statistics": map[string]interface{}{
			"total_requests":   15420,
			"blocked_requests": 142,
			"success_rate":     0.991,
			"top_endpoints": []map[string]interface{}{
				{"path": "/api/v1/chat", "requests": 8500, "blocked": 85},
				{"path": "/api/v1/completions", "requests": 4200, "blocked": 42},
				{"path": "/api/v1/embeddings", "requests": 2720, "blocked": 15},
			},
		},
		"rate_limits": map[string]interface{}{
			"endpoints": []map[string]interface{}{
				{"id": "1", "path": "/api/v1/chat", "method": "POST", "limit": 100, "window": "1m", "enabled": true},
				{"id": "2", "path": "/api/v1/completions", "method": "POST", "limit": 50, "window": "1m", "enabled": true},
			},
		},
	}

	return &pb.InitResponse{
		Success: true,
	}, nil
}

func (p *RateLimitingUIPlugin) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{
		Timestamp: req.Timestamp,
		Healthy:   true,
	}, nil
}

func (p *RateLimitingUIPlugin) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("Shutting down Rate Limiting UI Plugin")
	return &pb.ShutdownResponse{Success: true}, nil
}

// === Configuration Schema Method ===

func (p *RateLimitingUIPlugin) GetConfigSchema(ctx context.Context, req *pb.GetConfigSchemaRequest) (*pb.GetConfigSchemaResponse, error) {
	log.Printf("GetConfigSchema called")

	// Define the JSON Schema for this plugin's configuration
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object",
		"title":   "Rate Limiting Plugin Configuration",
		"description": "Configuration schema for the Rate Limiting UI Plugin",
		"properties": map[string]interface{}{
			"redis_url": map[string]interface{}{
				"type":        "string",
				"title":       "Redis URL",
				"description": "Connection URL for Redis storage backend",
				"default":     "redis://localhost:6379",
				"format":      "uri",
				"examples":    []string{"redis://localhost:6379", "redis://user:pass@host:6379/db"},
			},
			"default_limit": map[string]interface{}{
				"type":        "integer",
				"title":       "Default Rate Limit",
				"description": "Default number of requests allowed per time window",
				"default":     1000,
				"minimum":     1,
				"maximum":     1000000,
			},
			"default_window": map[string]interface{}{
				"type":        "string",
				"title":       "Default Time Window",
				"description": "Default time window for rate limiting (e.g., '1m', '1h', '1d')",
				"default":     "1h",
				"pattern":     "^\\d+[smhd]$",
				"examples":    []string{"30s", "1m", "5m", "1h", "24h"},
			},
			"enable_burst": map[string]interface{}{
				"type":        "boolean",
				"title":       "Enable Burst Mode",
				"description": "Allow temporary bursts above the rate limit",
				"default":     true,
			},
			"burst_multiplier": map[string]interface{}{
				"type":        "number",
				"title":       "Burst Multiplier",
				"description": "Multiplier for burst limits (e.g., 2.0 allows 2x the normal rate)",
				"default":     2.0,
				"minimum":     1.0,
				"maximum":     10.0,
			},
			"monitoring_enabled": map[string]interface{}{
				"type":        "boolean",
				"title":       "Enable Monitoring",
				"description": "Enable monitoring and metrics collection",
				"default":     true,
			},
			"alert_threshold": map[string]interface{}{
				"type":        "number",
				"title":       "Alert Threshold",
				"description": "Threshold for triggering rate limit alerts (0.0-1.0)",
				"default":     0.8,
				"minimum":     0.0,
				"maximum":     1.0,
			},
		},
		"required": []string{"redis_url", "default_limit"},
		"additionalProperties": false,
	}

	// Convert schema to JSON
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		log.Printf("Failed to marshal schema: %v", err)
		return &pb.GetConfigSchemaResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to generate schema: %v", err),
		}, nil
	}

	return &pb.GetConfigSchemaResponse{
		Success:    true,
		SchemaJson: string(schemaBytes),
	}, nil
}

// === AI Studio Asset Serving Methods ===

func (p *RateLimitingUIPlugin) GetAsset(ctx context.Context, req *pb.GetAssetRequest) (*pb.GetAssetResponse, error) {
	log.Printf("GetAsset called for: %s", req.AssetPath)

	// Normalize path - remove leading slash if present
	assetPath := req.AssetPath
	if len(assetPath) > 0 && assetPath[0] == '/' {
		assetPath = assetPath[1:]
	}

	log.Printf("Normalized asset path: %s", assetPath)

	// Read asset from embedded filesystem
	content, err := embeddedAssets.ReadFile(assetPath)
	if err != nil {
		log.Printf("Asset not found: %s, error: %v", req.AssetPath, err)
		return &pb.GetAssetResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Asset not found: %s", req.AssetPath),
		}, nil
	}

	// Detect MIME type
	mimeType := detectMimeType(req.AssetPath)

	log.Printf("✅ Serving asset: %s (%s, %d bytes)", req.AssetPath, mimeType, len(content))

	return &pb.GetAssetResponse{
		Success:       true,
		Content:       content,
		MimeType:      mimeType,
		ContentLength: int64(len(content)),
	}, nil
}

func (p *RateLimitingUIPlugin) ListAssets(ctx context.Context, req *pb.ListAssetsRequest) (*pb.ListAssetsResponse, error) {
	log.Printf("ListAssets called")

	// For MVP, return a simple list
	assets := []*pb.AssetInfo{
		{Path: "ui/webc/dashboard.js", MimeType: "application/javascript", Size: 1000},
		{Path: "ui/webc/settings.js", MimeType: "application/javascript", Size: 800},
		{Path: "assets/rate-limit.svg", MimeType: "image/svg+xml", Size: 500},
	}

	return &pb.ListAssetsResponse{
		Success: true,
		Assets:  assets,
	}, nil
}

func (p *RateLimitingUIPlugin) GetManifest(ctx context.Context, req *pb.GetManifestRequest) (*pb.GetManifestResponse, error) {
	log.Printf("GetManifest called")

	log.Printf("✅ Serving manifest (%d bytes)", len(manifestFile))

	return &pb.GetManifestResponse{
		Success:      true,
		ManifestJson: string(manifestFile),
	}, nil
}

func (p *RateLimitingUIPlugin) Call(ctx context.Context, req *pb.CallRequest) (*pb.CallResponse, error) {
	log.Printf("Call method: %s", req.Method)

	switch req.Method {
	case "get_statistics":
		return p.getStatistics(ctx, req.Payload)
	case "get_rate_limits":
		return p.getRateLimits(ctx, req.Payload)
	case "get_global_settings":
		return p.getGlobalSettings(ctx, req.Payload)
	case "set_global_settings":
		return p.setGlobalSettings(ctx, req.Payload)
	case "set_rate_limit":
		return p.setRateLimit(ctx, req.Payload)
	default:
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Unknown method: %s", req.Method),
		}, nil
	}
}

// RPC method implementations
func (p *RateLimitingUIPlugin) getStatistics(ctx context.Context, payload string) (*pb.CallResponse, error) {
	stats := p.kvStore["statistics"]

	data, err := json.Marshal(stats)
	if err != nil {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to marshal statistics: %v", err),
		}, nil
	}

	return &pb.CallResponse{
		Success: true,
		Data:    string(data),
	}, nil
}

func (p *RateLimitingUIPlugin) getRateLimits(ctx context.Context, payload string) (*pb.CallResponse, error) {
	rateLimits := p.kvStore["rate_limits"]

	data, err := json.Marshal(rateLimits)
	if err != nil {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to marshal rate limits: %v", err),
		}, nil
	}

	return &pb.CallResponse{
		Success: true,
		Data:    string(data),
	}, nil
}

func (p *RateLimitingUIPlugin) getGlobalSettings(ctx context.Context, payload string) (*pb.CallResponse, error) {
	settings := p.kvStore["global_settings"]

	data, err := json.Marshal(settings)
	if err != nil {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to marshal global settings: %v", err),
		}, nil
	}

	return &pb.CallResponse{
		Success: true,
		Data:    string(data),
	}, nil
}

func (p *RateLimitingUIPlugin) setGlobalSettings(ctx context.Context, payload string) (*pb.CallResponse, error) {
	var settings map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &settings); err != nil {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to parse settings payload: %v", err),
		}, nil
	}

	p.kvStore["global_settings"] = settings

	return &pb.CallResponse{
		Success: true,
		Data:    `{"message": "Settings updated successfully"}`,
	}, nil
}

func (p *RateLimitingUIPlugin) setRateLimit(ctx context.Context, payload string) (*pb.CallResponse, error) {
	var rateLimitData map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &rateLimitData); err != nil {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to parse rate limit payload: %v", err),
		}, nil
	}

	// For demo purposes, just acknowledge the request
	return &pb.CallResponse{
		Success: true,
		Data:    `{"message": "Rate limit updated successfully"}`,
	}, nil
}

// === Utility Functions ===

func detectMimeType(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".js":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json"
	case ".html":
		return "text/html"
	default:
		return "application/octet-stream"
	}
}

// === Plugin Interface Implementation ===

type AIStudioGRPCPlugin struct {
	plugin.Plugin
	Impl pb.PluginServiceServer
}

func (p *AIStudioGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterPluginServiceServer(s, p.Impl)
	return nil
}

// ConfigProviderGRPCPlugin provides config-only service for the rate limiting plugin
type ConfigProviderGRPCPlugin struct {
	plugin.Plugin
	Impl *RateLimitingUIPlugin
}

func (p *ConfigProviderGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	configpb.RegisterConfigProviderServiceServer(s, &ConfigProviderServer{Impl: p.Impl})
	return nil
}

func (p *ConfigProviderGRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return configpb.NewConfigProviderServiceClient(c), nil
}

// ConfigProviderServer implements the ConfigProviderService for rate limiting plugin
type ConfigProviderServer struct {
	configpb.UnimplementedConfigProviderServiceServer
	Impl *RateLimitingUIPlugin
}

func (s *ConfigProviderServer) GetConfigSchema(ctx context.Context, req *configpb.ConfigSchemaRequest) (*configpb.ConfigSchemaResponse, error) {
	// Return the same schema as the main plugin's GetConfigSchema method
	resp, err := s.Impl.GetConfigSchema(ctx, &pb.GetConfigSchemaRequest{})
	if err != nil {
		return &configpb.ConfigSchemaResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &configpb.ConfigSchemaResponse{
		Success:      resp.Success,
		SchemaJson:   resp.SchemaJson,
		ErrorMessage: resp.ErrorMessage,
	}, nil
}

func (s *ConfigProviderServer) Ping(ctx context.Context, req *configpb.ConfigPingRequest) (*configpb.ConfigPingResponse, error) {
	return &configpb.ConfigPingResponse{
		Timestamp: req.Timestamp,
		Healthy:   true,
	}, nil
}

func (p *AIStudioGRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return pb.NewPluginServiceClient(c), nil
}

// === Main ===

func main() {
	log.Printf("🚀 Starting Rate Limiting UI Plugin with embedded assets")

	pluginImpl := &RateLimitingUIPlugin{}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "AI_STUDIO_PLUGIN",
			MagicCookieValue: "v1",
		},
		Plugins: map[string]plugin.Plugin{
			"plugin": &AIStudioGRPCPlugin{
				Impl: pluginImpl,
			},
			"config": &ConfigProviderGRPCPlugin{
				Impl: pluginImpl,
			},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
