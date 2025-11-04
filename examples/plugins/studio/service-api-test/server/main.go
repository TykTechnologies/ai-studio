package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

//go:embed ui assets plugin.manifest.json
var embeddedAssets embed.FS

//go:embed plugin.manifest.json
var manifestFile []byte

const (
	PluginName    = "service-api-test"
	PluginVersion = "1.0.0"
)

// ServiceAPITestPlugin implements the AI Studio plugin for service API testing
type ServiceAPITestPlugin struct {
	plugin_sdk.BasePlugin
}

// NewServiceAPITestPlugin creates a new service API test plugin
func NewServiceAPITestPlugin() *ServiceAPITestPlugin {
	return &ServiceAPITestPlugin{
		BasePlugin: plugin_sdk.NewBasePlugin(PluginName, PluginVersion, "Service API E2E Test Plugin"),
	}
}

// Initialize implements plugin_sdk.Plugin
func (p *ServiceAPITestPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	log.Printf("🧪 %s: Initialized in %s runtime", PluginName, ctx.Runtime)

	// Extract and set broker ID for service API access
	brokerIDStr := ""
	if id, ok := config["_service_broker_id"]; ok {
		brokerIDStr = id
	} else if id, ok := config["service_broker_id"]; ok {
		brokerIDStr = id
	}

	if brokerIDStr != "" {
		var brokerID uint32
		if _, err := fmt.Sscanf(brokerIDStr, "%d", &brokerID); err == nil {
			ai_studio_sdk.SetServiceBrokerID(brokerID)
			log.Printf("🧪 %s: Set service broker ID: %d", PluginName, brokerID)
		}
	}

	// Extract plugin ID if present
	if pluginIDStr, ok := config["plugin_id"]; ok {
		var pluginID uint32
		fmt.Sscanf(pluginIDStr, "%d", &pluginID)
		ai_studio_sdk.SetPluginID(pluginID)
		log.Printf("🧪 %s: Plugin ID set to: %d", PluginName, pluginID)
	}

	log.Printf("✅ %s: Initialized successfully", PluginName)
	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *ServiceAPITestPlugin) Shutdown(ctx plugin_sdk.Context) error {
	log.Printf("🧪 %s: Shutdown called", PluginName)
	return nil
}

// GetManifest implements plugin_sdk.UIProvider
func (p *ServiceAPITestPlugin) GetManifest() ([]byte, error) {
	return manifestFile, nil
}

// GetAsset implements plugin_sdk.UIProvider
func (p *ServiceAPITestPlugin) GetAsset(assetPath string) ([]byte, string, error) {
	if len(assetPath) > 0 && assetPath[0] == '/' {
		assetPath = assetPath[1:]
	}

	content, err := embeddedAssets.ReadFile(assetPath)
	if err != nil {
		return nil, "", fmt.Errorf("asset not found: %s", assetPath)
	}

	mimeType := "application/octet-stream"
	if len(assetPath) > 3 && assetPath[len(assetPath)-3:] == ".js" {
		mimeType = "application/javascript"
	} else if len(assetPath) > 4 && assetPath[len(assetPath)-4:] == ".svg" {
		mimeType = "image/svg+xml"
	}

	return content, mimeType, nil
}

// ListAssets implements plugin_sdk.UIProvider
func (p *ServiceAPITestPlugin) ListAssets(pathPrefix string) ([]*pb.AssetInfo, error) {
	return []*pb.AssetInfo{
		{Path: "ui/webc/test-dashboard.js", MimeType: "application/javascript"},
		{Path: "assets/test-icon.svg", MimeType: "image/svg+xml"},
	}, nil
}

// GetConfigSchema implements plugin_sdk.ConfigProvider
func (p *ServiceAPITestPlugin) GetConfigSchema() ([]byte, error) {
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

	return json.Marshal(schema)
}

// HandleRPC implements plugin_sdk.UIProvider
func (p *ServiceAPITestPlugin) HandleRPC(method string, payload []byte) ([]byte, error) {
	log.Printf("🧪 %s: ========== RPC CALL START ==========", PluginName)
	log.Printf("🧪 %s: Method: %s", PluginName, method)
	log.Printf("🧪 %s: Payload size: %d bytes", PluginName, len(payload))
	log.Printf("🧪 %s: Payload content: %s", PluginName, string(payload))

	// Extract broker ID from payload
	brokerID := ai_studio_sdk.ExtractBrokerIDFromPayload(payload)
	log.Printf("🧪 %s: Extracted broker ID from payload: %d", PluginName, brokerID)

	if brokerID != 0 {
		ai_studio_sdk.SetServiceBrokerID(brokerID)
		log.Printf("🧪 %s: ✅ Set service broker ID: %d", PluginName, brokerID)
	} else {
		log.Printf("🧪 %s: ⚠️ WARNING: No broker ID in payload!", PluginName)
		log.Printf("🧪 %s: Raw payload: %q", PluginName, payload)
	}

	switch method {
	case "run_e2e_tests":
		return p.runE2ETests(payload)
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

func (p *ServiceAPITestPlugin) runE2ETests(payload []byte) ([]byte, error) {
	// Extract broker ID FIRST to ensure it's set
	brokerID := ai_studio_sdk.ExtractBrokerIDFromPayload(payload)
	if brokerID != 0 {
		ai_studio_sdk.SetServiceBrokerID(brokerID)
	}

	// NOW check if initialized - after setting broker ID
	debugInfo := map[string]interface{}{
		"sdk_initialized_before_set": ai_studio_sdk.IsInitialized(),
		"broker_id_extracted":        brokerID,
		"payload_size":               len(payload),
		"payload_content":            string(payload),
	}

	// Check again after setting broker ID
	debugInfo["sdk_initialized_after_set"] = ai_studio_sdk.IsInitialized()

	if !ai_studio_sdk.IsInitialized() {
		debugInfo["error"] = "SDK STILL not initialized after setting broker ID"
		debugInfo["note"] = "IsInitialized() only checks if Initialize() was called during GRPCServer setup"
		debugInfo["note2"] = "Setting broker ID alone is not enough - the grpcBroker must also be set during Initialize()"
		debugJSON, _ := json.Marshal(debugInfo)
		return nil, fmt.Errorf("SDK not initialized - Debug: %s", string(debugJSON))
	}

	ctx := context.Background()

	// Run all tests
	report, err := RunE2ETests(ctx)
	if err != nil {
		return nil, fmt.Errorf("test execution failed: %v", err)
	}

	log.Printf("🧪 %s: E2E tests completed: %d passed, %d failed out of %d total",
		PluginName, report.PassedTests, report.FailedTests, report.TotalTests)

	// Convert report to JSON
	return report.toJSON()
}

func main() {
	log.Printf("🧪 Starting %s Plugin v%s", PluginName, PluginVersion)
	log.Printf("Service API testing plugin with UI using unified SDK")

	plugin := NewServiceAPITestPlugin()
	plugin_sdk.Serve(plugin)
}
