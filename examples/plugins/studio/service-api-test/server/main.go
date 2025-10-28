package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
)

//go:embed ui assets plugin.manifest.json
var embeddedAssets embed.FS

//go:embed plugin.manifest.json
var manifestFile []byte

// ServiceAPITestPlugin implements the AI Studio plugin for service API testing
type ServiceAPITestPlugin struct {
	pb.UnimplementedPluginServiceServer
	pluginID uint32
}

// Initialize plugin
func (p *ServiceAPITestPlugin) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	log.Printf("Initializing Service API Test Plugin")

	// Extract plugin ID from config
	if pluginIDStr, ok := req.Config["plugin_id"]; ok {
		fmt.Sscanf(pluginIDStr, "%d", &p.pluginID)
		ai_studio_sdk.SetPluginID(p.pluginID)
		log.Printf("Plugin ID set to: %d", p.pluginID)
	}

	return &pb.InitResponse{Success: true}, nil
}

func (p *ServiceAPITestPlugin) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{Timestamp: req.Timestamp, Healthy: true}, nil
}

func (p *ServiceAPITestPlugin) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("Shutting down Service API Test Plugin")
	return &pb.ShutdownResponse{Success: true}, nil
}

func (p *ServiceAPITestPlugin) GetManifest(ctx context.Context, req *pb.GetManifestRequest) (*pb.GetManifestResponse, error) {
	return &pb.GetManifestResponse{
		Success:      true,
		ManifestJson: string(manifestFile),
	}, nil
}

func (p *ServiceAPITestPlugin) GetAsset(ctx context.Context, req *pb.GetAssetRequest) (*pb.GetAssetResponse, error) {
	assetPath := req.AssetPath
	if len(assetPath) > 0 && assetPath[0] == '/' {
		assetPath = assetPath[1:]
	}

	content, err := embeddedAssets.ReadFile(assetPath)
	if err != nil {
		return &pb.GetAssetResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Asset not found: %s", req.AssetPath),
		}, nil
	}

	mimeType := "application/octet-stream"
	if len(assetPath) > 3 && assetPath[len(assetPath)-3:] == ".js" {
		mimeType = "application/javascript"
	} else if len(assetPath) > 4 && assetPath[len(assetPath)-4:] == ".svg" {
		mimeType = "image/svg+xml"
	}

	return &pb.GetAssetResponse{
		Success:       true,
		Content:       content,
		MimeType:      mimeType,
		ContentLength: int64(len(content)),
	}, nil
}

func (p *ServiceAPITestPlugin) ListAssets(ctx context.Context, req *pb.ListAssetsRequest) (*pb.ListAssetsResponse, error) {
	return &pb.ListAssetsResponse{
		Success: true,
		Assets: []*pb.AssetInfo{
			{Path: "ui/webc/test-dashboard.js", MimeType: "application/javascript"},
			{Path: "assets/test-icon.svg", MimeType: "image/svg+xml"},
		},
	}, nil
}

func (p *ServiceAPITestPlugin) GetConfigSchema(ctx context.Context, req *pb.GetConfigSchemaRequest) (*pb.GetConfigSchemaResponse, error) {
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object",
		"title":   "Service API Test Plugin Configuration",
		"properties": map[string]interface{}{
			"test_user_id": map[string]interface{}{
				"type":        "integer",
				"title":       "Test User ID",
				"description": "User ID to use for test resource creation",
				"default":     1,
			},
		},
	}

	schemaBytes, _ := json.Marshal(schema)
	return &pb.GetConfigSchemaResponse{
		Success:    true,
		SchemaJson: string(schemaBytes),
	}, nil
}

func (p *ServiceAPITestPlugin) Call(ctx context.Context, req *pb.CallRequest) (*pb.CallResponse, error) {
	log.Printf("Call method: %s", req.Method)

	switch req.Method {
	case "run_e2e_tests":
		return p.runE2ETests(ctx, req.Payload)
	default:
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Unknown method: %s", req.Method),
		}, nil
	}
}

func (p *ServiceAPITestPlugin) runE2ETests(ctx context.Context, payload string) (*pb.CallResponse, error) {
	log.Printf("Starting E2E tests for plugin %d", p.pluginID)

	if !ai_studio_sdk.IsInitialized() {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: "SDK not initialized - service API unavailable",
		}, nil
	}

	// Run all tests
	report, err := RunE2ETests(ctx)
	if err != nil {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Test execution failed: %v", err),
		}, nil
	}

	// Convert report to JSON
	reportJSON, err := report.toJSON()
	if err != nil {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to serialize test report: %v", err),
		}, nil
	}

	log.Printf("E2E tests completed: %d passed, %d failed out of %d total",
		report.PassedTests, report.FailedTests, report.TotalTests)

	return &pb.CallResponse{
		Success: true,
		Data:    string(reportJSON),
	}, nil
}

func main() {
	log.Printf("🧪 Starting Service API E2E Test Plugin")

	pluginImpl := &ServiceAPITestPluginSDK{
		plugin: &ServiceAPITestPlugin{},
	}

	ai_studio_sdk.ServePlugin(pluginImpl)
}

// ServiceAPITestPluginSDK implements the SDK interface
type ServiceAPITestPluginSDK struct {
	plugin *ServiceAPITestPlugin
}

func (p *ServiceAPITestPluginSDK) OnInitialize(serviceAPI mgmtpb.AIStudioManagementServiceClient, pluginID uint32, config map[string]string) error {
	log.Printf("Service API Test Plugin SDK initializing with %d config keys", len(config))

	// Merge plugin_id into config
	if config == nil {
		config = make(map[string]string)
	}
	config["plugin_id"] = fmt.Sprintf("%d", pluginID)

	ctx := context.Background()
	req := &pb.InitRequest{
		Config: config,
	}
	_, err := p.plugin.Initialize(ctx, req)

	if err == nil {
		log.Printf("✅ Service API Test Plugin initialized successfully")
	}

	return err
}

func (p *ServiceAPITestPluginSDK) OnShutdown() error {
	return nil
}

func (p *ServiceAPITestPluginSDK) GetAsset(assetPath string) ([]byte, string, error) {
	ctx := context.Background()
	resp, err := p.plugin.GetAsset(ctx, &pb.GetAssetRequest{AssetPath: assetPath})
	if err != nil || !resp.Success {
		return nil, "", fmt.Errorf("asset request failed")
	}
	return resp.Content, resp.MimeType, nil
}

func (p *ServiceAPITestPluginSDK) GetManifest() ([]byte, error) {
	return manifestFile, nil
}

func (p *ServiceAPITestPluginSDK) HandleCall(method string, payload []byte) ([]byte, error) {
	// Extract broker ID
	if brokerID := ai_studio_sdk.ExtractBrokerIDFromPayload(payload); brokerID != 0 {
		ai_studio_sdk.SetServiceBrokerID(brokerID)
	}

	ctx := context.Background()
	resp, err := p.plugin.Call(ctx, &pb.CallRequest{
		Method:  method,
		Payload: string(payload),
	})
	if err != nil || !resp.Success {
		return nil, fmt.Errorf("call failed: %s", resp.ErrorMessage)
	}
	return []byte(resp.Data), nil
}

func (p *ServiceAPITestPluginSDK) GetConfigSchema() ([]byte, error) {
	ctx := context.Background()
	resp, err := p.plugin.GetConfigSchema(ctx, &pb.GetConfigSchemaRequest{})
	if err != nil || !resp.Success {
		return nil, fmt.Errorf("schema request failed")
	}
	return []byte(resp.SchemaJson), nil
}
