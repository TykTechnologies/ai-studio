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
	services plugin_sdk.ServiceBroker
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

	// Store services reference for later use
	p.services = ctx.Services

	log.Printf("✅ %s: Initialized successfully (broker will be available after OpenSession)", PluginName)
	return nil
}

// OnSessionReady implements plugin_sdk.SessionAware
// This is called when the session-based broker connection is established.
// We eagerly initialize the service API connection here so it's ready for RPC calls.
func (p *ServiceAPITestPlugin) OnSessionReady(ctx plugin_sdk.Context) {
	log.Printf("🧪 %s: OnSessionReady called - session broker is now active", PluginName)

	// Eagerly establish the broker connection by making a simple API call.
	// This "warms up" the connection so subsequent RPC calls don't need to dial.
	// The go-plugin broker only accepts ONE connection per broker ID, so we need
	// to establish it early while the broker is fresh.
	if ai_studio_sdk.IsInitialized() {
		log.Printf("🧪 %s: Warming up service API connection...", PluginName)
		// Make a lightweight API call to establish the connection
		_, err := ai_studio_sdk.GetPluginsCount(context.Background())
		if err != nil {
			log.Printf("🧪 %s: Service API warmup failed: %v (this is expected if the SDK dial fails)", PluginName, err)
		} else {
			log.Printf("🧪 %s: ✅ Service API connection established successfully", PluginName)
		}
	} else {
		log.Printf("🧪 %s: SDK not initialized yet, skipping warmup", PluginName)
	}
}

// OnSessionClosing implements plugin_sdk.SessionAware
// This is called before the session is explicitly closed
func (p *ServiceAPITestPlugin) OnSessionClosing(ctx plugin_sdk.Context) {
	log.Printf("🧪 %s: OnSessionClosing called", PluginName)
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
	log.Printf("🧪 %s: RPC call: method=%s, payload_size=%d", PluginName, method, len(payload))

	// The SDK wrapper (Call method in wrapper.go) automatically sets the broker ID
	// from req.ServiceBrokerId before calling HandleRPC, so service APIs should
	// "just work" at this point.
	//
	// Check if SDK is ready (broker ID should be set by the wrapper)
	if !ai_studio_sdk.IsInitialized() {
		log.Printf("🧪 %s: WARNING - ai_studio_sdk not initialized, service API calls may fail", PluginName)
		return nil, fmt.Errorf("service API not available - broker ID may not be set")
	}

	log.Printf("🧪 %s: ai_studio_sdk is initialized, proceeding with RPC", PluginName)

	switch method {
	case "run_e2e_tests":
		return p.runE2ETests(payload)
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

func (p *ServiceAPITestPlugin) runE2ETests(payload []byte) ([]byte, error) {
	ctx := context.Background()

	// Run all tests - SDK is already initialized via session
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
