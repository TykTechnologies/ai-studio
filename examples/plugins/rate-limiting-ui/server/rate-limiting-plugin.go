package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strconv"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	configpb "github.com/TykTechnologies/midsommar/v2/proto/configpb"
	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services/grpc"
	"github.com/hashicorp/go-plugin"
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
	kvStore         map[string]interface{}
	serviceProvider grpc.AIStudioServiceProvider // Injected by AI Studio
	pluginID        uint32
}

// === Lifecycle Methods ===

func (p *RateLimitingUIPlugin) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	log.Printf("Initializing Rate Limiting UI Plugin with config: %v", req.Config)

	// Extract plugin ID from config (set by AI Studio plugin manager)
	if pluginIDStr, ok := req.Config["plugin_id"]; ok {
		if pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32); err == nil {
			p.pluginID = uint32(pluginID)
			log.Printf("Plugin ID set to: %d", p.pluginID)
		} else {
			log.Printf("Warning: Invalid plugin ID format: %s", pluginIDStr)
		}
	} else {
		log.Printf("Warning: Plugin ID not found in config")
	}

	// Check if service provider will be available
	if hasServiceProvider, ok := req.Config["has_service_provider"]; ok && hasServiceProvider == "true" {
		log.Printf("✅ AI Studio service provider will be injected")
		// Service provider will be injected by AI Studio plugin manager
		// No need to create network connections
	} else {
		log.Printf("⚠️ No service provider available - using mock data")
		p.initializeMockData()
	}

	// Initialize with minimal global settings
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
	}

	return &pb.InitResponse{
		Success: true,
	}, nil
}

// InjectServiceProvider implements ServiceProviderInjectable interface
func (p *RateLimitingUIPlugin) InjectServiceProvider(provider grpc.AIStudioServiceProvider) {
	p.serviceProvider = provider
	log.Printf("✅ Service provider injected into Rate Limiting Plugin")
}

// initializeMockData sets up mock data when service provider is unavailable
func (p *RateLimitingUIPlugin) initializeMockData() {
	log.Printf("Initializing with mock data (gRPC client unavailable)")
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
	case "get_available_tools":
		return p.getAvailableTools(ctx, req.Payload)
	case "get_datasources":
		return p.getDatasources(ctx, req.Payload)
	case "get_data_catalogues":
		return p.getDataCatalogues(ctx, req.Payload)
	case "get_tags":
		return p.getTags(ctx, req.Payload)
	default:
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Unknown method: %s", req.Method),
		}, nil
	}
}

// RPC method implementations
func (p *RateLimitingUIPlugin) getStatistics(ctx context.Context, payload string) (*pb.CallResponse, error) {
	// If service provider is available, fetch real analytics data
	if p.serviceProvider != nil && p.pluginID != 0 {
		return p.getStatisticsFromService(ctx)
	}

	// Fall back to mock data
	stats := p.kvStore["statistics"]

	data, err := json.Marshal(stats)
	if err != nil {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to marshal statistics: %v", err),
		}, nil
	}

	log.Printf("Returning mock statistics data")
	return &pb.CallResponse{
		Success: true,
		Data:    string(data),
	}, nil
}

// getStatisticsFromService fetches real analytics data via injected service provider
func (p *RateLimitingUIPlugin) getStatisticsFromService(ctx context.Context) (*pb.CallResponse, error) {
	log.Printf("Fetching real analytics data via injected service provider for plugin %d", p.pluginID)

	// Create plugin context for authentication
	pluginCtx := &mgmtpb.PluginContext{
		PluginId:    p.pluginID,
		MethodScope: "analytics.read",
	}

	// Get analytics summary from AI Studio via injected service provider (in-process call)
	analyticsResp, err := p.serviceProvider.GetAnalyticsSummary(ctx, &mgmtpb.GetAnalyticsSummaryRequest{
		Context:   pluginCtx,
		TimeRange: "24h",
	})
	if err != nil {
		log.Printf("Failed to fetch analytics data: %v", err)
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to fetch analytics: %v", err),
		}, nil
	}

	// Convert analytics data to the format expected by the UI
	topEndpoints := make([]map[string]interface{}, len(analyticsResp.TopEndpoints))
	for i, endpoint := range analyticsResp.TopEndpoints {
		successRate := float64(endpoint.RequestCount-endpoint.BlockedCount) / float64(endpoint.RequestCount)
		topEndpoints[i] = map[string]interface{}{
			"path":     endpoint.Path,
			"requests": endpoint.RequestCount,
			"blocked":  endpoint.BlockedCount,
			"success_rate": successRate,
		}
	}

	// Build statistics response in expected format
	stats := map[string]interface{}{
		"total_requests":   analyticsResp.TotalRequests,
		"blocked_requests": analyticsResp.FailedRequests, // Using failed as proxy for blocked
		"success_rate":     float64(analyticsResp.SuccessfulRequests) / float64(analyticsResp.TotalRequests),
		"top_endpoints":    topEndpoints,
		"total_cost":       analyticsResp.TotalCost,
		"currency":         analyticsResp.Currency,
		"total_tokens":     analyticsResp.TotalTokens,
	}

	data, err := json.Marshal(stats)
	if err != nil {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to marshal real analytics data: %v", err),
		}, nil
	}

	log.Printf("✅ Returning real analytics data via injected service provider")
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

func (p *RateLimitingUIPlugin) getAvailableTools(ctx context.Context, payload string) (*pb.CallResponse, error) {
	// Demonstrate tool access via injected service provider
	if p.serviceProvider != nil && p.pluginID != 0 {
		return p.getAvailableToolsFromService(ctx)
	}

	// Fall back to mock data
	mockTools := map[string]interface{}{
		"tools": []map[string]interface{}{
			{
				"id":   1,
				"name": "HTTP API Tool",
				"slug": "http-api-tool",
				"type": "rest",
				"operations": []string{"get_users", "create_user"},
			},
			{
				"id":   2,
				"name": "Database Query Tool",
				"slug": "db-query-tool",
				"type": "sql",
				"operations": []string{"select_query", "insert_query"},
			},
		},
		"total_count": 2,
	}

	data, err := json.Marshal(mockTools)
	if err != nil {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to marshal mock tools: %v", err),
		}, nil
	}

	log.Printf("Returning mock tools data")
	return &pb.CallResponse{
		Success: true,
		Data:    string(data),
	}, nil
}

// getAvailableToolsFromService fetches real tool data via gRPC
func (p *RateLimitingUIPlugin) getAvailableToolsFromService(ctx context.Context) (*pb.CallResponse, error) {
	log.Printf("Fetching real tools data via gRPC for plugin %d", p.pluginID)

	// Create plugin context for authentication
	pluginCtx := &mgmtpb.PluginContext{
		PluginId:    p.pluginID,
		MethodScope: "tools.read",
	}

	// Get tools list from AI Studio via injected service provider
	toolsResp, err := p.serviceProvider.ListTools(ctx, &mgmtpb.ListToolsRequest{
		Context: pluginCtx,
		Page:    1,
		Limit:   50, // Get up to 50 tools
	})
	if err != nil {
		log.Printf("Failed to fetch tools data: %v", err)
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to fetch tools: %v", err),
		}, nil
	}

	// Convert tools data to the format expected by the UI
	tools := make([]map[string]interface{}, len(toolsResp.Tools))
	for i, tool := range toolsResp.Tools {
		tools[i] = map[string]interface{}{
			"id":          tool.Id,
			"name":        tool.Name,
			"slug":        tool.Slug,
			"description": tool.Description,
			"type":        tool.ToolType,
			"operations":  tool.Operations,
			"is_active":   tool.IsActive,
			"privacy_score": tool.PrivacyScore,
		}
	}

	// Build tools response in expected format
	toolsData := map[string]interface{}{
		"tools":       tools,
		"total_count": toolsResp.TotalCount,
	}

	data, err := json.Marshal(toolsData)
	if err != nil {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to marshal real tools data: %v", err),
		}, nil
	}

	log.Printf("✅ Returning real tools data via gRPC")
	return &pb.CallResponse{
		Success: true,
		Data:    string(data),
	}, nil
}

// Datasources management demonstration
func (p *RateLimitingUIPlugin) getDatasources(ctx context.Context, payload string) (*pb.CallResponse, error) {
	if p.serviceProvider != nil && p.pluginID != 0 {
		pluginCtx := &mgmtpb.PluginContext{
			PluginId:    p.pluginID,
			MethodScope: "datasources.read",
		}

		resp, err := p.serviceProvider.ListDatasources(ctx, &mgmtpb.ListDatasourcesRequest{
			Context: pluginCtx,
			Page:    1,
			Limit:   20,
		})
		if err != nil {
			return &pb.CallResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to fetch datasources: %v", err),
			}, nil
		}

		// Convert to expected format
		datasourcesData := map[string]interface{}{
			"datasources":  resp.Datasources,
			"total_count":  resp.TotalCount,
		}

		data, err := json.Marshal(datasourcesData)
		if err != nil {
			return &pb.CallResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to marshal datasources: %v", err),
			}, nil
		}

		log.Printf("✅ Returning real datasources data via gRPC")
		return &pb.CallResponse{
			Success: true,
			Data:    string(data),
		}, nil
	}

	// Fallback mock data
	mockData := map[string]interface{}{
		"datasources": []map[string]interface{}{
			{"id": 1, "name": "Customer Database", "type": "sql", "active": true},
			{"id": 2, "name": "Product Catalog", "type": "api", "active": true},
		},
		"total_count": 2,
	}

	data, _ := json.Marshal(mockData)
	return &pb.CallResponse{Success: true, Data: string(data)}, nil
}

// Data catalogues management demonstration
func (p *RateLimitingUIPlugin) getDataCatalogues(ctx context.Context, payload string) (*pb.CallResponse, error) {
	if p.serviceProvider != nil && p.pluginID != 0 {
		pluginCtx := &mgmtpb.PluginContext{
			PluginId:    p.pluginID,
			MethodScope: "data-catalogues.read",
		}

		resp, err := p.serviceProvider.ListDataCatalogues(ctx, &mgmtpb.ListDataCataloguesRequest{
			Context: pluginCtx,
			Page:    1,
			Limit:   20,
		})
		if err != nil {
			return &pb.CallResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to fetch data catalogues: %v", err),
			}, nil
		}

		cataloguesData := map[string]interface{}{
			"data_catalogues": resp.DataCatalogues,
			"total_count":     resp.TotalCount,
		}

		data, err := json.Marshal(cataloguesData)
		if err != nil {
			return &pb.CallResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to marshal data catalogues: %v", err),
			}, nil
		}

		log.Printf("✅ Returning real data catalogues via gRPC")
		return &pb.CallResponse{Success: true, Data: string(data)}, nil
	}

	// Fallback mock data
	mockData := map[string]interface{}{
		"data_catalogues": []map[string]interface{}{
			{"id": 1, "name": "Customer Data", "description": "Customer information datasets"},
			{"id": 2, "name": "Product Data", "description": "Product catalog and inventory"},
		},
		"total_count": 2,
	}
	data, _ := json.Marshal(mockData)
	return &pb.CallResponse{Success: true, Data: string(data)}, nil
}

// Tags management demonstration
func (p *RateLimitingUIPlugin) getTags(ctx context.Context, payload string) (*pb.CallResponse, error) {
	if p.serviceProvider != nil && p.pluginID != 0 {
		pluginCtx := &mgmtpb.PluginContext{
			PluginId:    p.pluginID,
			MethodScope: "tags.read",
		}

		resp, err := p.serviceProvider.ListTags(ctx, &mgmtpb.ListTagsRequest{
			Context: pluginCtx,
			Page:    1,
			Limit:   50,
		})
		if err != nil {
			return &pb.CallResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to fetch tags: %v", err),
			}, nil
		}

		tagsData := map[string]interface{}{
			"tags":        resp.Tags,
			"total_count": resp.TotalCount,
		}

		data, err := json.Marshal(tagsData)
		if err != nil {
			return &pb.CallResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to marshal tags: %v", err),
			}, nil
		}

		log.Printf("✅ Returning real tags data via gRPC")
		return &pb.CallResponse{Success: true, Data: string(data)}, nil
	}

	// Fallback mock data
	mockData := map[string]interface{}{
		"tags": []map[string]interface{}{
			{"id": 1, "name": "AI/ML"},
			{"id": 2, "name": "Analytics"},
			{"id": 3, "name": "Database"},
		},
		"total_count": 3,
	}
	data, _ := json.Marshal(mockData)
	return &pb.CallResponse{Success: true, Data: string(data)}, nil
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
