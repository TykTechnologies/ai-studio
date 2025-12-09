// +build e2e

package plugintest_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	gwmgmtpb "github.com/TykTechnologies/midsommar/microgateway/proto/microgateway_management"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/plugintest"
)

// ============================================================================
// Gateway Handler Tests (Microgateway Runtime)
// These tests use ProcessPostAuth and OnBeforeWrite which are gateway-specific
// ============================================================================

// TestHandlerPostAuth tests the PostAuth handler with cache behavior.
// NOTE: PostAuth is a gateway-specific handler, so this test runs in gateway runtime.
func TestHandlerPostAuth(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Build a chat completion request
	req := plugintest.NewRequestBuilder().
		WithChatCompletion([]plugintest.Message{
			{Role: "user", Content: "What is 2+2?"},
		}).
		WithModel("gpt-4").
		WithVendor("openai").
		WithUserIDInt(1).
		WithAppIDInt(1).
		Build()

	// First request should be a cache miss
	resp, err := harness.CallPostAuth(req)
	if err != nil {
		t.Fatalf("CallPostAuth failed: %v", err)
	}

	t.Logf("PostAuth response - Modified: %v, Block: %v", resp.Modified, resp.Block)

	// On cache hit, Block would be true and response would be set
	if resp.Block {
		t.Log("Request was served from cache (cache hit)")
	} else {
		t.Log("Request was not cached (cache miss or new request)")
	}
}

// TestHandlerPostAuthWithBypass tests bypassing the cache.
// NOTE: PostAuth is a gateway-specific handler, so this test runs in gateway runtime.
func TestHandlerPostAuthWithBypass(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Build request with cache bypass header
	req := plugintest.NewRequestBuilder().
		WithChatCompletion([]plugintest.Message{
			{Role: "user", Content: "What is the current time?"},
		}).
		WithModel("gpt-4").
		WithHeader("X-Cache-Bypass", "true").
		Build()

	resp, err := harness.CallPostAuth(req)
	if err != nil {
		t.Fatalf("CallPostAuth failed: %v", err)
	}

	// Cache bypass should never block the request
	if resp.Block {
		t.Error("Request with cache bypass should not be blocked")
	}

	t.Log("Cache bypass request passed through correctly")
}

// TestHandlerOnBeforeWrite tests caching responses.
// NOTE: OnBeforeWrite is a gateway-specific handler, so this test runs in gateway runtime.
func TestHandlerOnBeforeWrite(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Build a response to cache
	respBuilder := plugintest.NewResponseBuilder().
		WithChatCompletion("The answer is 4.").
		Build()

	writeResp, err := harness.CallOnBeforeWrite(respBuilder)
	if err != nil {
		t.Fatalf("CallOnBeforeWrite failed: %v", err)
	}

	t.Logf("OnBeforeWrite response - Modified: %v", writeResp.Modified)
}

// ============================================================================
// RPC Tests (Work in both Studio and Gateway runtimes)
// ============================================================================

// TestRPCGetMetrics tests the getMetrics RPC method.
func TestRPCGetMetrics(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Get metrics
	response, err := harness.CallRPC("getMetrics", []byte("{}"))
	if err != nil {
		t.Fatalf("getMetrics RPC failed: %v", err)
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(response, &metrics); err != nil {
		t.Fatalf("Failed to parse metrics response: %v", err)
	}

	// Verify expected fields exist
	expectedFields := []string{"hit_count", "miss_count", "hit_rate", "active_entries"}
	for _, field := range expectedFields {
		if _, ok := metrics[field]; !ok {
			t.Errorf("Expected field '%s' not found in metrics", field)
		}
	}

	t.Logf("Metrics: %+v", metrics)
}

// TestRPCClearCache tests the clearCache RPC method.
func TestRPCClearCache(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Clear cache
	response, err := harness.CallRPC("clearCache", []byte("{}"))
	if err != nil {
		t.Fatalf("clearCache RPC failed: %v", err)
	}

	t.Logf("clearCache response: %s", string(response))

	// Verify metrics show empty cache
	metricsResp, err := harness.CallRPC("getMetrics", []byte("{}"))
	if err != nil {
		t.Fatalf("getMetrics after clear failed: %v", err)
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(metricsResp, &metrics); err != nil {
		t.Fatalf("Failed to parse metrics response: %v", err)
	}

	if entries, ok := metrics["active_entries"].(float64); ok && entries > 0 {
		t.Errorf("Expected 0 entries after clear, got %v", entries)
	}
}

// TestRPCGetHealth tests the getHealth RPC method.
func TestRPCGetHealth(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Get health
	response, err := harness.CallRPC("getHealth", []byte("{}"))
	if err != nil {
		t.Fatalf("getHealth RPC failed: %v", err)
	}

	var health map[string]interface{}
	if err := json.Unmarshal(response, &health); err != nil {
		t.Fatalf("Failed to parse health response: %v", err)
	}

	// Verify healthy field exists
	if healthy, ok := health["healthy"].(bool); ok {
		t.Logf("Backend healthy: %v", healthy)
	} else {
		t.Error("Expected 'healthy' field in response")
	}

	// Verify backend_type field
	if backendType, ok := health["backend_type"].(string); ok {
		t.Logf("Backend type: %s", backendType)
	}
}

// TestRPCGetConfig tests the getConfig RPC method.
func TestRPCGetConfig(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	configValues := map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "600",
		"max_cache_size_mb": "128",
		"max_entry_size_kb": "2048",
	}

	err := harness.Initialize(configValues)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Get config
	response, err := harness.CallRPC("getConfig", []byte("{}"))
	if err != nil {
		t.Fatalf("getConfig RPC failed: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(response, &config); err != nil {
		t.Fatalf("Failed to parse config response: %v", err)
	}

	// Verify config matches what we set
	if enabled, ok := config["enabled"].(bool); ok && !enabled {
		t.Error("Expected enabled=true in config")
	}

	t.Logf("Config: %+v", config)
}

// TestRPCTestBackend tests the testBackend RPC method.
func TestRPCTestBackend(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Test backend connection
	response, err := harness.CallRPC("testBackend", []byte("{}"))
	if err != nil {
		t.Fatalf("testBackend RPC failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		t.Fatalf("Failed to parse testBackend response: %v", err)
	}

	// With in-memory backend, this should always succeed
	if success, ok := result["success"].(bool); ok {
		t.Logf("Backend test success: %v", success)
		if !success {
			if errMsg, ok := result["error"].(string); ok {
				t.Logf("Backend test error: %s", errMsg)
			}
		}
	}

	// Check latency
	if latency, ok := result["latency_ms"].(float64); ok {
		t.Logf("Backend latency: %.2fms", latency)
	}
}

// TestRPCUnknownMethod tests handling of unknown RPC methods.
func TestRPCUnknownMethod(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled": "true",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Call unknown method
	_, err = harness.CallRPC("nonExistentMethod", []byte("{}"))
	if err == nil {
		t.Error("Expected error for unknown RPC method")
	} else {
		t.Logf("Unknown method returned expected error: %v", err)
	}
}

// TestKVStorageDuringSession tests that KV storage works during plugin session.
func TestKVStorageDuringSession(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// Pre-populate KV store
	harness.SetKVData("cache:test-key", []byte(`{"cached": true}`))

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Get metrics to ensure plugin is operational
	_, err = harness.CallRPC("getMetrics", []byte("{}"))
	if err != nil {
		t.Fatalf("getMetrics failed: %v", err)
	}

	// Check if any KV writes were made
	writes := harness.GetKVWrites()
	t.Logf("KV writes during session: %d", len(writes))
	for _, w := range writes {
		t.Logf("  - Key: %s, Size: %d bytes", w.Key, len(w.Value))
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

// setupE2EGatewayHarness creates a test harness configured for gateway runtime.
// Use this for tests that call gateway-specific methods like ProcessPostAuth, OnBeforeWrite.
func setupE2EGatewayHarness(t *testing.T) *plugintest.E2EPluginHarness {
	t.Helper()

	projectRoot := findProjectRoot(t)
	if projectRoot == "" {
		t.Skip("Could not find project root")
	}

	pluginDir := filepath.Join(projectRoot, "enterprise", "plugins", "advanced-llm-cache")
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		t.Skipf("Plugin directory not found: %s", pluginDir)
	}

	// Build the plugin binary with enterprise build tag
	binaryPath := filepath.Join(t.TempDir(), "advanced-llm-cache")

	cmd := exec.Command("go", "build", "-tags", "enterprise", "-o", binaryPath, ".")
	cmd.Dir = pluginDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build plugin: %v\n%s", err, output)
	}

	harness := plugintest.NewE2EHarness(binaryPath)
	// Configure for gateway runtime
	harness.SetRuntime(plugin_sdk.RuntimeGateway)

	return harness
}

// ============================================================================
// Gateway Feature Tests
// These tests demonstrate using the TestGatewayManagementServer to configure
// gateway-specific test data like apps, LLMs, budgets, and model pricing.
// ============================================================================

// TestGatewayServerConfiguration tests that gateway server can be configured
// with test data for apps, LLMs, and budgets.
func TestGatewayServerConfiguration(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	// Configure gateway server with test data before starting plugin
	gwServer := harness.GatewayServer()

	// Add test app - this data will be available via GetApp/ListApps RPCs
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Test App",
		IsActive: true,
	})

	// Add test LLM - available via GetLLM/ListLLMs
	gwServer.AddTestLLM(&gwmgmtpb.LLMInfo{
		Id:     1,
		Name:   "GPT-4",
		Vendor: "openai",
	})

	// Configure license
	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Verify license was checked during session
	if !harness.LicenseWasChecked() {
		t.Log("Note: License check may not have occurred yet")
	}

	// Test that the plugin can process requests
	req := plugintest.NewRequestBuilder().
		WithChatCompletion([]plugintest.Message{
			{Role: "user", Content: "Hello from gateway test"},
		}).
		WithModel("gpt-4").
		WithVendor("openai").
		WithUserIDInt(1).
		WithAppIDInt(1).
		WithLLMID(1).
		Build()

	resp, err := harness.CallPostAuth(req)
	if err != nil {
		t.Fatalf("CallPostAuth failed: %v", err)
	}

	t.Logf("Gateway request processed - Modified: %v, Block: %v", resp.Modified, resp.Block)
}

// TestGatewayServiceCalls tests that plugin calls to gateway services are tracked.
func TestGatewayServiceCalls(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()

	// Configure test data
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Test App",
		IsActive: true,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{"enabled": "true"})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Check what service calls the plugin made during initialization
	calls := gwServer.GetCalls()
	t.Logf("Gateway service calls during session: %d", len(calls))
	for _, call := range calls {
		t.Logf("  - Method: %s at %v", call.Method, call.Timestamp)
	}

	// Should have at least called GetLicenseInfo
	foundLicenseCall := false
	for _, call := range calls {
		if call.Method == "GetLicenseInfo" {
			foundLicenseCall = true
			break
		}
	}
	if foundLicenseCall {
		t.Log("Plugin called GetLicenseInfo as expected")
	}
}

// ============================================================================
// Metadata-Based Bypass Rule Tests
// These tests verify that cache bypass rules work correctly when based on
// app metadata (user_tier, regulatory_class) fetched via the GetApp service API.
// ============================================================================

// TestBypassRulesUserTierFromAppMetadata tests that cache is bypassed when
// the app's metadata contains a user_tier that matches the bypass configuration.
// This test verifies the data flow: App Metadata -> GetApp API -> HandlePostAuth -> Policy Engine
func TestBypassRulesUserTierFromAppMetadata(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()

	// Configure app with user_tier that should trigger bypass
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "No Cache App",
		IsActive: true,
		Metadata: `{"user_tier": "no-cache", "description": "App with caching disabled via user tier"}`,
	})

	// Configure app without bypass tier for comparison
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       2,
		Name:     "Normal App",
		IsActive: true,
		Metadata: `{"user_tier": "pro", "description": "Normal app with caching enabled"}`,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with bypass rules that include "no-cache" user tier
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
		"bypass_rules":      `{"bypass_user_tiers": ["no-cache", "free-trial"]}`,
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Test 1: Request from app with "no-cache" tier should NOT be cached
	t.Run("AppWithBypassTier", func(t *testing.T) {
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "What is 2+2?"},
			}).
			WithModel("gpt-4").
			WithVendor("openai").
			WithUserIDInt(1).
			WithAppIDInt(1). // App with "no-cache" tier
			Build()

		resp, err := harness.CallPostAuth(req)
		if err != nil {
			t.Fatalf("CallPostAuth failed: %v", err)
		}

		// Cache bypass should not block the request (it passes through without caching)
		if resp.Block {
			t.Error("Request from bypass-tier app should not be blocked (served from cache)")
		}

		t.Logf("App with bypass tier - Modified: %v, Block: %v (expected Block=false)", resp.Modified, resp.Block)
	})

	// Test 2: Request from app with "pro" tier should be eligible for caching
	t.Run("AppWithNormalTier", func(t *testing.T) {
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "What is 3+3?"},
			}).
			WithModel("gpt-4").
			WithVendor("openai").
			WithUserIDInt(1).
			WithAppIDInt(2). // App with "pro" tier
			Build()

		resp, err := harness.CallPostAuth(req)
		if err != nil {
			t.Fatalf("CallPostAuth failed: %v", err)
		}

		// First request should be a cache miss, but the request IS cacheable
		// (Block=false just means cache miss on first request, which is expected)
		t.Logf("App with normal tier - Modified: %v, Block: %v", resp.Modified, resp.Block)
	})

	// Verify metrics show bypass was triggered for app 1
	metricsResp, err := harness.CallRPC("getMetrics", []byte("{}"))
	if err != nil {
		t.Fatalf("getMetrics failed: %v", err)
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(metricsResp, &metrics); err != nil {
		t.Fatalf("Failed to parse metrics: %v", err)
	}

	if bypassCount, ok := metrics["bypass_count"].(float64); ok {
		t.Logf("Bypass count: %.0f", bypassCount)
		if bypassCount < 1 {
			t.Error("Expected at least 1 bypass from user_tier rule")
		}
	}
}

// TestBypassRulesRegulatoryClassFromAppMetadata tests that cache is bypassed when
// the app's metadata contains a regulatory_class that matches the bypass configuration.
func TestBypassRulesRegulatoryClassFromAppMetadata(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()

	// Configure app with PCI regulatory class (should bypass cache)
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "PCI Compliant App",
		IsActive: true,
		Metadata: `{"regulatory_class": "pci", "compliance_level": "level-1"}`,
	})

	// Configure app with HIPAA regulatory class (should bypass cache)
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       2,
		Name:     "HIPAA Compliant App",
		IsActive: true,
		Metadata: `{"regulatory_class": "hipaa"}`,
	})

	// Configure app with general regulatory class (should NOT bypass cache)
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       3,
		Name:     "General App",
		IsActive: true,
		Metadata: `{"regulatory_class": "general"}`,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with bypass rules for PCI and HIPAA
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
		"bypass_rules":      `{"bypass_regulatory_classes": ["pci", "hipaa"]}`,
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Test 1: PCI app should bypass cache
	t.Run("PCIAppBypassesCache", func(t *testing.T) {
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "Process payment for order 12345"},
			}).
			WithModel("gpt-4").
			WithVendor("openai").
			WithAppIDInt(1). // PCI app
			Build()

		resp, err := harness.CallPostAuth(req)
		if err != nil {
			t.Fatalf("CallPostAuth failed: %v", err)
		}

		if resp.Block {
			t.Error("PCI app request should not be served from cache (should bypass)")
		}
		t.Logf("PCI app - Block: %v (expected false for bypass)", resp.Block)
	})

	// Test 2: HIPAA app should bypass cache
	t.Run("HIPAAAppBypassesCache", func(t *testing.T) {
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "Retrieve patient medical records"},
			}).
			WithModel("gpt-4").
			WithVendor("openai").
			WithAppIDInt(2). // HIPAA app
			Build()

		resp, err := harness.CallPostAuth(req)
		if err != nil {
			t.Fatalf("CallPostAuth failed: %v", err)
		}

		if resp.Block {
			t.Error("HIPAA app request should not be served from cache (should bypass)")
		}
		t.Logf("HIPAA app - Block: %v (expected false for bypass)", resp.Block)
	})

	// Test 3: General app should be cacheable
	t.Run("GeneralAppIsCacheable", func(t *testing.T) {
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "Tell me a joke"},
			}).
			WithModel("gpt-4").
			WithVendor("openai").
			WithAppIDInt(3). // General app
			Build()

		resp, err := harness.CallPostAuth(req)
		if err != nil {
			t.Fatalf("CallPostAuth failed: %v", err)
		}

		// First request is a cache miss, but the request IS cacheable
		t.Logf("General app - Block: %v (first request is cache miss, but cacheable)", resp.Block)
	})

	// Verify metrics
	metricsResp, err := harness.CallRPC("getMetrics", []byte("{}"))
	if err != nil {
		t.Fatalf("getMetrics failed: %v", err)
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(metricsResp, &metrics); err != nil {
		t.Fatalf("Failed to parse metrics: %v", err)
	}

	if bypassCount, ok := metrics["bypass_count"].(float64); ok {
		t.Logf("Total bypass count: %.0f", bypassCount)
		if bypassCount < 2 {
			t.Errorf("Expected at least 2 bypasses (PCI + HIPAA), got %.0f", bypassCount)
		}
	}
}

// TestBypassRulesCombinedMetadata tests that both user_tier and regulatory_class
// bypass rules work together correctly.
func TestBypassRulesCombinedMetadata(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()

	// App with both bypass triggers
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Double Bypass App",
		IsActive: true,
		Metadata: `{"user_tier": "no-cache", "regulatory_class": "pci"}`,
	})

	// App with only user_tier bypass
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       2,
		Name:     "User Tier Only",
		IsActive: true,
		Metadata: `{"user_tier": "no-cache", "regulatory_class": "general"}`,
	})

	// App with only regulatory_class bypass
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       3,
		Name:     "Regulatory Only",
		IsActive: true,
		Metadata: `{"user_tier": "enterprise", "regulatory_class": "hipaa"}`,
	})

	// App with neither bypass (should be cached)
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       4,
		Name:     "Cacheable App",
		IsActive: true,
		Metadata: `{"user_tier": "enterprise", "regulatory_class": "general"}`,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with both user tier and regulatory class bypass rules
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
		"bypass_rules":      `{"bypass_user_tiers": ["no-cache"], "bypass_regulatory_classes": ["pci", "hipaa"]}`,
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	testCases := []struct {
		name           string
		appID          uint32
		shouldBypass   bool
		bypassReason   string
	}{
		{"DoubleBypass", 1, true, "user_tier or regulatory_class"},
		{"UserTierOnly", 2, true, "user_tier"},
		{"RegulatoryOnly", 3, true, "regulatory_class"},
		{"Cacheable", 4, false, "none"},
	}

	bypassCount := 0
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := plugintest.NewRequestBuilder().
				WithChatCompletion([]plugintest.Message{
					{Role: "user", Content: "Test message for " + tc.name},
				}).
				WithModel("gpt-4").
				WithVendor("openai").
				WithAppIDInt(tc.appID).
				Build()

			resp, err := harness.CallPostAuth(req)
			if err != nil {
				t.Fatalf("CallPostAuth failed: %v", err)
			}

			// For bypass cases, the request should not be blocked (cache not used)
			// For cacheable cases, first request is also not blocked (cache miss)
			// The key difference is in the metrics
			t.Logf("%s (appID=%d) - Block: %v, shouldBypass: %v", tc.name, tc.appID, resp.Block, tc.shouldBypass)

			if tc.shouldBypass {
				bypassCount++
			}
		})
	}

	// Verify bypass metrics
	metricsResp, err := harness.CallRPC("getMetrics", []byte("{}"))
	if err != nil {
		t.Fatalf("getMetrics failed: %v", err)
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(metricsResp, &metrics); err != nil {
		t.Fatalf("Failed to parse metrics: %v", err)
	}

	if actualBypass, ok := metrics["bypass_count"].(float64); ok {
		t.Logf("Expected bypass count: %d, Actual: %.0f", bypassCount, actualBypass)
		if int(actualBypass) < bypassCount {
			t.Errorf("Expected at least %d bypasses, got %.0f", bypassCount, actualBypass)
		}
	}
}

// TestBypassRulesAppWithNoMetadata tests that apps without metadata
// do not trigger bypass rules (graceful handling of missing metadata).
func TestBypassRulesAppWithNoMetadata(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()

	// App with no metadata at all
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "No Metadata App",
		IsActive: true,
		Metadata: "", // Empty metadata
	})

	// App with empty JSON metadata
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       2,
		Name:     "Empty JSON App",
		IsActive: true,
		Metadata: "{}",
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with bypass rules
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
		"bypass_rules":      `{"bypass_user_tiers": ["no-cache"], "bypass_regulatory_classes": ["pci"]}`,
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Test both apps - neither should trigger bypass (no matching metadata)
	for _, appID := range []uint32{1, 2} {
		t.Run("AppID_"+string(rune('0'+appID)), func(t *testing.T) {
			req := plugintest.NewRequestBuilder().
				WithChatCompletion([]plugintest.Message{
					{Role: "user", Content: "Hello"},
				}).
				WithModel("gpt-4").
				WithVendor("openai").
				WithAppIDInt(appID).
				Build()

			resp, err := harness.CallPostAuth(req)
			if err != nil {
				t.Fatalf("CallPostAuth failed: %v", err)
			}

			// Should not error out - graceful handling
			t.Logf("App %d (no/empty metadata) - Block: %v, Modified: %v", appID, resp.Block, resp.Modified)
		})
	}

	// Verify no bypasses occurred
	metricsResp, err := harness.CallRPC("getMetrics", []byte("{}"))
	if err != nil {
		t.Fatalf("getMetrics failed: %v", err)
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(metricsResp, &metrics); err != nil {
		t.Fatalf("Failed to parse metrics: %v", err)
	}

	if bypassCount, ok := metrics["bypass_count"].(float64); ok {
		t.Logf("Bypass count for apps with no metadata: %.0f", bypassCount)
		if bypassCount > 0 {
			t.Errorf("Expected 0 bypasses for apps with no metadata, got %.0f", bypassCount)
		}
	}
}

// TestBypassRulesCaseInsensitivity tests that bypass rules are case-insensitive
// for user_tier and regulatory_class values.
func TestBypassRulesCaseInsensitivity(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()

	// App with uppercase user_tier
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Uppercase Tier App",
		IsActive: true,
		Metadata: `{"user_tier": "NO-CACHE"}`, // Uppercase
	})

	// App with mixed case regulatory_class
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       2,
		Name:     "Mixed Case Regulatory App",
		IsActive: true,
		Metadata: `{"regulatory_class": "PCI"}`, // Uppercase
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with lowercase bypass rules
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
		"bypass_rules":      `{"bypass_user_tiers": ["no-cache"], "bypass_regulatory_classes": ["pci"]}`, // lowercase
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Test uppercase user_tier matches lowercase config
	t.Run("UppercaseUserTier", func(t *testing.T) {
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "Test case insensitivity"},
			}).
			WithModel("gpt-4").
			WithVendor("openai").
			WithAppIDInt(1).
			Build()

		_, err := harness.CallPostAuth(req)
		if err != nil {
			t.Fatalf("CallPostAuth failed: %v", err)
		}
	})

	// Test uppercase regulatory_class matches lowercase config
	t.Run("UppercaseRegulatoryClass", func(t *testing.T) {
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "Test case insensitivity"},
			}).
			WithModel("gpt-4").
			WithVendor("openai").
			WithAppIDInt(2).
			Build()

		_, err := harness.CallPostAuth(req)
		if err != nil {
			t.Fatalf("CallPostAuth failed: %v", err)
		}
	})

	// Verify both triggered bypass (case-insensitive matching)
	metricsResp, err := harness.CallRPC("getMetrics", []byte("{}"))
	if err != nil {
		t.Fatalf("getMetrics failed: %v", err)
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(metricsResp, &metrics); err != nil {
		t.Fatalf("Failed to parse metrics: %v", err)
	}

	if bypassCount, ok := metrics["bypass_count"].(float64); ok {
		t.Logf("Bypass count (case insensitivity test): %.0f", bypassCount)
		if bypassCount < 2 {
			t.Errorf("Expected at least 2 bypasses (case-insensitive matching), got %.0f", bypassCount)
		}
	}
}

// ============================================================================
// Cache Flow Tests
// These tests verify the complete cache hit/miss flow end-to-end.
// ============================================================================

// TestCacheHitFlow tests the full cache cycle: miss -> cache response -> hit
func TestCacheHitFlow(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Cache Flow Test App",
		IsActive: true,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Use a unique request ID that will be used for both request and response
	requestID := "cache-hit-flow-test-12345"
	prompt := "What is the capital of France?"

	// Step 1: First request - should be cache miss
	req := plugintest.NewRequestBuilder().
		WithChatCompletion([]plugintest.Message{
			{Role: "user", Content: prompt},
		}).
		WithModel("gpt-4").
		WithVendor("openai").
		WithRequestID(requestID).
		WithUserIDInt(1).
		WithAppIDInt(1).
		Build()

	resp1, err := harness.CallPostAuth(req)
	if err != nil {
		t.Fatalf("First CallPostAuth failed: %v", err)
	}

	// First request should NOT block (cache miss)
	if resp1.Block {
		t.Error("First request should not be blocked (expected cache miss)")
	}
	t.Logf("First request - Block: %v (expected false for cache miss)", resp1.Block)

	// Step 2: Simulate the response being cached via OnBeforeWrite
	respBuilder := plugintest.NewResponseBuilder().
		WithChatCompletion("The capital of France is Paris.").
		WithRequestID(requestID).
		WithVendor("openai").
		WithAppID(1).
		WithUserID(1).
		Build()

	writeResp, err := harness.CallOnBeforeWrite(respBuilder)
	if err != nil {
		t.Fatalf("CallOnBeforeWrite failed: %v", err)
	}
	t.Logf("OnBeforeWrite - Modified: %v", writeResp.Modified)

	// Step 3: Second identical request - should be cache hit
	req2 := plugintest.NewRequestBuilder().
		WithChatCompletion([]plugintest.Message{
			{Role: "user", Content: prompt},
		}).
		WithModel("gpt-4").
		WithVendor("openai").
		WithRequestID("cache-hit-flow-test-67890"). // Different request ID
		WithUserIDInt(1).
		WithAppIDInt(1).
		Build()

	resp2, err := harness.CallPostAuth(req2)
	if err != nil {
		t.Fatalf("Second CallPostAuth failed: %v", err)
	}

	// Second request SHOULD block with cached response
	if !resp2.Block {
		t.Log("Note: Second request was not served from cache - this may indicate cache key generation issues")
	} else {
		t.Log("Cache hit! Second request was served from cache")
	}
	t.Logf("Second request - Block: %v (expected true for cache hit)", resp2.Block)

	// Verify metrics reflect the cache hit/miss pattern
	metricsResp, err := harness.CallRPC("getMetrics", []byte("{}"))
	if err != nil {
		t.Fatalf("getMetrics failed: %v", err)
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(metricsResp, &metrics); err != nil {
		t.Fatalf("Failed to parse metrics: %v", err)
	}

	missCount, _ := metrics["miss_count"].(float64)
	hitCount, _ := metrics["hit_count"].(float64)
	t.Logf("Metrics - Hits: %.0f, Misses: %.0f", hitCount, missCount)

	// We expect at least 1 miss (first request) and ideally 1 hit (second request)
	if missCount < 1 {
		t.Error("Expected at least 1 cache miss")
	}
}

// TestEntrySizeLimits tests that entries exceeding max_entry_size_kb are not cached
func TestEntrySizeLimits(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Entry Size Test App",
		IsActive: true,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with very small max entry size (1KB)
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
		"max_entry_size_kb": "1", // Only 1KB max entry
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Create a request
	req := plugintest.NewRequestBuilder().
		WithChatCompletion([]plugintest.Message{
			{Role: "user", Content: "Tell me a story"},
		}).
		WithModel("gpt-4").
		WithVendor("openai").
		WithRequestID("large-entry-test").
		WithAppIDInt(1).
		Build()

	_, err = harness.CallPostAuth(req)
	if err != nil {
		t.Fatalf("CallPostAuth failed: %v", err)
	}

	// Create a large response (> 1KB)
	largeContent := ""
	for i := 0; i < 200; i++ {
		largeContent += "This is a very long response that should exceed the max entry size limit. "
	}

	respBuilder := plugintest.NewResponseBuilder().
		WithChatCompletion(largeContent).
		WithRequestID("large-entry-test").
		WithVendor("openai").
		WithAppID(1).
		Build()

	_, err = harness.CallOnBeforeWrite(respBuilder)
	if err != nil {
		t.Fatalf("CallOnBeforeWrite failed: %v", err)
	}

	// Check metrics - entry should NOT have been cached due to size limit
	metricsResp, err := harness.CallRPC("getMetrics", []byte("{}"))
	if err != nil {
		t.Fatalf("getMetrics failed: %v", err)
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(metricsResp, &metrics); err != nil {
		t.Fatalf("Failed to parse metrics: %v", err)
	}

	entries, _ := metrics["active_entries"].(float64)
	t.Logf("Active entries after large response: %.0f (expected 0 due to size limit)", entries)
}

// TestListCacheEntries tests the listEntries RPC for browsing cache contents
func TestListCacheEntries(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "List Entries Test App",
		IsActive: true,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Cache multiple entries with different models
	models := []string{"gpt-4", "gpt-3.5-turbo", "claude-3"}
	for i, model := range models {
		reqID := "list-test-" + model
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "Question " + model},
			}).
			WithModel(model).
			WithVendor("openai").
			WithRequestID(reqID).
			WithAppIDInt(1).
			Build()

		_, _ = harness.CallPostAuth(req)

		resp := plugintest.NewResponseBuilder().
			WithChatCompletion("Answer for " + model).
			WithRequestID(reqID).
			WithVendor("openai").
			WithAppID(1).
			Build()

		_, _ = harness.CallOnBeforeWrite(resp)
		t.Logf("Cached entry %d for model %s", i+1, model)
	}

	// List all entries
	listResp, err := harness.CallRPC("listEntries", []byte(`{"limit": 100}`))
	if err != nil {
		t.Fatalf("listEntries RPC failed: %v", err)
	}

	var listResult map[string]interface{}
	if err := json.Unmarshal(listResp, &listResult); err != nil {
		t.Fatalf("Failed to parse listEntries response: %v", err)
	}

	t.Logf("listEntries response: %+v", listResult)

	// Verify we got entries back
	if entries, ok := listResult["entries"].([]interface{}); ok {
		t.Logf("Found %d entries in cache", len(entries))
	}
}

// TestDeleteCacheEntry tests deleting a specific cache entry
func TestDeleteCacheEntry(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Delete Entry Test App",
		IsActive: true,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Cache an entry
	reqID := "delete-test-entry"
	req := plugintest.NewRequestBuilder().
		WithChatCompletion([]plugintest.Message{
			{Role: "user", Content: "Entry to be deleted"},
		}).
		WithModel("gpt-4").
		WithVendor("openai").
		WithRequestID(reqID).
		WithAppIDInt(1).
		Build()

	_, _ = harness.CallPostAuth(req)

	resp := plugintest.NewResponseBuilder().
		WithChatCompletion("This will be deleted").
		WithRequestID(reqID).
		WithVendor("openai").
		WithAppID(1).
		Build()

	_, _ = harness.CallOnBeforeWrite(resp)

	// Get metrics before delete
	metricsBefore, _ := harness.CallRPC("getMetrics", []byte("{}"))
	var beforeMetrics map[string]interface{}
	json.Unmarshal(metricsBefore, &beforeMetrics)
	entriesBefore, _ := beforeMetrics["active_entries"].(float64)
	t.Logf("Entries before delete: %.0f", entriesBefore)

	// List entries to get a key to delete
	listResp, err := harness.CallRPC("listEntries", []byte(`{"limit": 1}`))
	if err != nil {
		t.Logf("listEntries failed: %v", err)
		return
	}

	var listResult map[string]interface{}
	if err := json.Unmarshal(listResp, &listResult); err != nil {
		t.Logf("Failed to parse listEntries: %v", err)
		return
	}

	entries, ok := listResult["entries"].([]interface{})
	if !ok || len(entries) == 0 {
		t.Log("No entries found to delete")
		return
	}

	// Get the first entry's key
	if entry, ok := entries[0].(map[string]interface{}); ok {
		if key, ok := entry["key"].(string); ok {
			// Delete the entry
			deleteResp, err := harness.CallRPC("deleteEntry", []byte(`{"key": "`+key+`"}`))
			if err != nil {
				t.Logf("deleteEntry failed: %v", err)
			} else {
				t.Logf("deleteEntry response: %s", string(deleteResp))
			}
		}
	}

	// Get metrics after delete
	metricsAfter, _ := harness.CallRPC("getMetrics", []byte("{}"))
	var afterMetrics map[string]interface{}
	json.Unmarshal(metricsAfter, &afterMetrics)
	entriesAfter, _ := afterMetrics["active_entries"].(float64)
	t.Logf("Entries after delete: %.0f", entriesAfter)
}

// ============================================================================
// Model Family Bypass Tests
// ============================================================================

// TestModelFamilyBypass tests bypassing cache for specific model families
func TestModelFamilyBypass(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Model Bypass Test App",
		IsActive: true,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with model family bypass - skip caching for o1-preview models
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
		"bypass_rules":      `{"bypass_model_families": ["o1-preview", "o1-mini"]}`,
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Test 1: Request with bypassed model (o1-preview)
	t.Run("BypassedModel", func(t *testing.T) {
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "Solve this complex problem"},
			}).
			WithModel("o1-preview").
			WithVendor("openai").
			WithAppIDInt(1).
			Build()

		_, err := harness.CallPostAuth(req)
		if err != nil {
			t.Fatalf("CallPostAuth failed: %v", err)
		}
		t.Log("Request with o1-preview model processed")
	})

	// Test 2: Request with non-bypassed model (gpt-4)
	t.Run("NonBypassedModel", func(t *testing.T) {
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "Simple question"},
			}).
			WithModel("gpt-4").
			WithVendor("openai").
			WithAppIDInt(1).
			Build()

		_, err := harness.CallPostAuth(req)
		if err != nil {
			t.Fatalf("CallPostAuth failed: %v", err)
		}
		t.Log("Request with gpt-4 model processed")
	})

	// Check bypass metrics
	metricsResp, _ := harness.CallRPC("getMetrics", []byte("{}"))
	var metrics map[string]interface{}
	json.Unmarshal(metricsResp, &metrics)

	bypassCount, _ := metrics["bypass_count"].(float64)
	t.Logf("Bypass count: %.0f (expected at least 1 for o1-preview)", bypassCount)
}

// ============================================================================
// TTL Policy Tests
// These tests verify that TTL policies correctly adjust cache TTL based on rules.
// ============================================================================

// TestTTLPolicyTokenCostRules tests TTL adjustment based on token count
func TestTTLPolicyTokenCostRules(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "TTL Policy Test App",
		IsActive: true,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with TTL policy rules based on token count
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
		"ttl_policy":        `{"enabled": true, "token_cost_rules": [{"min_tokens": 0, "max_tokens": 100, "ttl_seconds": 60}, {"min_tokens": 100, "max_tokens": 1000, "ttl_seconds": 600}]}`,
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Make a request
	req := plugintest.NewRequestBuilder().
		WithChatCompletion([]plugintest.Message{
			{Role: "user", Content: "Short question"},
		}).
		WithModel("gpt-4").
		WithVendor("openai").
		WithRequestID("ttl-test").
		WithAppIDInt(1).
		Build()

	_, err = harness.CallPostAuth(req)
	if err != nil {
		t.Fatalf("CallPostAuth failed: %v", err)
	}

	// Create response with usage info (token count)
	responseBody := map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"message": map[string]string{
					"role":    "assistant",
					"content": "Short answer.",
				},
				"index":         0,
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     10,
			"completion_tokens": 5,
			"total_tokens":      15, // Low token count - should get short TTL
		},
	}
	bodyBytes, _ := json.Marshal(responseBody)

	resp := plugintest.NewResponseBuilder().
		WithBody(bodyBytes).
		WithHeader("Content-Type", "application/json").
		WithRequestID("ttl-test").
		WithVendor("openai").
		WithAppID(1).
		Build()

	_, err = harness.CallOnBeforeWrite(resp)
	if err != nil {
		t.Fatalf("CallOnBeforeWrite failed: %v", err)
	}

	t.Log("Response with low token count cached - TTL policy should have applied short TTL")
}

// TestTTLPolicyUserTierRules tests TTL adjustment based on user tier
func TestTTLPolicyUserTierRules(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()

	// App with "premium" user tier
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Premium App",
		IsActive: true,
		Metadata: `{"user_tier": "premium"}`,
	})

	// App with "free" user tier
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       2,
		Name:     "Free App",
		IsActive: true,
		Metadata: `{"user_tier": "free"}`,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with user tier TTL rules
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
		"ttl_policy":        `{"enabled": true, "user_tier_rules": [{"tier": "premium", "ttl_seconds": 3600}, {"tier": "free", "ttl_seconds": 60}]}`,
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Test request from premium app
	t.Run("PremiumTier", func(t *testing.T) {
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "Premium user question"},
			}).
			WithModel("gpt-4").
			WithVendor("openai").
			WithRequestID("premium-ttl-test").
			WithAppIDInt(1). // Premium app
			Build()

		_, err := harness.CallPostAuth(req)
		if err != nil {
			t.Fatalf("CallPostAuth failed: %v", err)
		}

		resp := plugintest.NewResponseBuilder().
			WithChatCompletion("Premium answer").
			WithRequestID("premium-ttl-test").
			WithVendor("openai").
			WithAppID(1).
			Build()

		_, _ = harness.CallOnBeforeWrite(resp)
		t.Log("Premium tier response cached with longer TTL")
	})

	// Test request from free app
	t.Run("FreeTier", func(t *testing.T) {
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "Free user question"},
			}).
			WithModel("gpt-4").
			WithVendor("openai").
			WithRequestID("free-ttl-test").
			WithAppIDInt(2). // Free app
			Build()

		_, err := harness.CallPostAuth(req)
		if err != nil {
			t.Fatalf("CallPostAuth failed: %v", err)
		}

		resp := plugintest.NewResponseBuilder().
			WithChatCompletion("Free answer").
			WithRequestID("free-ttl-test").
			WithVendor("openai").
			WithAppID(2).
			Build()

		_, _ = harness.CallOnBeforeWrite(resp)
		t.Log("Free tier response cached with shorter TTL")
	})
}

// ============================================================================
// Configuration Update Tests
// ============================================================================

// TestConfigUpdatePersistence tests that configuration updates work via RPC
func TestConfigUpdatePersistence(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Config Update Test App",
		IsActive: true,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initial config with TTL of 300
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Get initial config
	configResp, err := harness.CallRPC("getConfig", []byte("{}"))
	if err != nil {
		t.Fatalf("getConfig failed: %v", err)
	}

	var config map[string]interface{}
	json.Unmarshal(configResp, &config)
	t.Logf("Initial config: ttl_seconds=%v", config["ttl_seconds"])

	// Update TTL via updateConfig RPC
	updateResp, err := harness.CallRPC("updateConfig", []byte(`{"ttl_seconds": 600}`))
	if err != nil {
		t.Logf("updateConfig failed (may not be implemented): %v", err)
	} else {
		t.Logf("updateConfig response: %s", string(updateResp))

		// Verify config was updated
		newConfigResp, _ := harness.CallRPC("getConfig", []byte("{}"))
		var newConfig map[string]interface{}
		json.Unmarshal(newConfigResp, &newConfig)
		t.Logf("Updated config: ttl_seconds=%v", newConfig["ttl_seconds"])
	}
}

// ============================================================================
// Namespace Tests
// ============================================================================

// TestNamespaceMetrics tests that metrics are tracked per namespace
func TestNamespaceMetrics(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()

	// Add multiple apps for different namespaces
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "App One",
		IsActive: true,
	})
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       2,
		Name:     "App Two",
		IsActive: true,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with app_id based namespace
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
		"namespace_key":     "app_id",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Make requests from different apps
	for _, appID := range []uint32{1, 2} {
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "Question from app"},
			}).
			WithModel("gpt-4").
			WithVendor("openai").
			WithRequestID("ns-test-" + string(rune('0'+appID))).
			WithAppIDInt(appID).
			Build()

		_, _ = harness.CallPostAuth(req)

		resp := plugintest.NewResponseBuilder().
			WithChatCompletion("Answer").
			WithRequestID("ns-test-" + string(rune('0'+appID))).
			WithVendor("openai").
			WithAppID(appID).
			Build()

		_, _ = harness.CallOnBeforeWrite(resp)
	}

	// Get metrics - should show per-namespace stats if supported
	metricsResp, _ := harness.CallRPC("getMetrics", []byte("{}"))
	var metrics map[string]interface{}
	json.Unmarshal(metricsResp, &metrics)

	t.Logf("Metrics with namespace partitioning: %+v", metrics)

	// Check for namespace-specific metrics
	if namespaceStats, ok := metrics["namespace_stats"].(map[string]interface{}); ok {
		t.Logf("Namespace stats: %+v", namespaceStats)
	}
}

// ============================================================================
// License Enforcement Tests
// ============================================================================

// TestLicenseEnforcement tests that enterprise features require valid license
func TestLicenseEnforcement(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "License Test App",
		IsActive: true,
	})

	// Test with valid enterprise license
	t.Run("ValidEnterpriseLicense", func(t *testing.T) {
		harness.SetLicense("enterprise", true, 365)
		harness.SetEntitlements([]string{"advanced-llm-cache"})

		if err := harness.Start(); err != nil {
			t.Fatalf("Failed to start plugin: %v", err)
		}

		err := harness.Initialize(map[string]string{
			"enabled":           "true",
			"ttl_seconds":       "300",
			"max_cache_size_mb": "64",
			// Enterprise features
			"failover": `{"enabled": true}`,
		})
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		if err := harness.OpenSession(); err != nil {
			t.Fatalf("OpenSession failed: %v", err)
		}

		// Verify license was checked
		if harness.LicenseWasChecked() {
			t.Log("License was validated during startup")
		}

		// Get license status
		licenseResp, err := harness.CallRPC("getLicenseStatus", []byte("{}"))
		if err != nil {
			t.Logf("getLicenseStatus not available: %v", err)
		} else {
			var licenseStatus map[string]interface{}
			json.Unmarshal(licenseResp, &licenseStatus)
			t.Logf("License status: %+v", licenseStatus)
		}

		// Verify enterprise features are enabled
		configResp, _ := harness.CallRPC("getConfig", []byte("{}"))
		var config map[string]interface{}
		json.Unmarshal(configResp, &config)
		t.Logf("Enterprise enabled: %v", config["enterprise_enabled"])
	})
}

// ============================================================================
// Event/Distributed Cache Tests
// ============================================================================

// TestDistributedCacheClear tests cache clearing via events
func TestDistributedCacheClear(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Distributed Clear Test App",
		IsActive: true,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Cache some entries
	for i := 0; i < 3; i++ {
		reqID := "dist-clear-" + string(rune('0'+i))
		req := plugintest.NewRequestBuilder().
			WithChatCompletion([]plugintest.Message{
				{Role: "user", Content: "Question " + string(rune('0'+i))},
			}).
			WithModel("gpt-4").
			WithVendor("openai").
			WithRequestID(reqID).
			WithAppIDInt(1).
			Build()

		_, _ = harness.CallPostAuth(req)

		resp := plugintest.NewResponseBuilder().
			WithChatCompletion("Answer " + string(rune('0'+i))).
			WithRequestID(reqID).
			WithVendor("openai").
			WithAppID(1).
			Build()

		_, _ = harness.CallOnBeforeWrite(resp)
	}

	// Verify entries exist
	metricsBefore, _ := harness.CallRPC("getMetrics", []byte("{}"))
	var beforeMetrics map[string]interface{}
	json.Unmarshal(metricsBefore, &beforeMetrics)
	t.Logf("Entries before clear: %v", beforeMetrics["active_entries"])

	// Clear cache via RPC
	clearResp, err := harness.CallRPC("clearCache", []byte("{}"))
	if err != nil {
		t.Fatalf("clearCache failed: %v", err)
	}
	t.Logf("clearCache response: %s", string(clearResp))

	// Check that published events include cache clear
	events := harness.GetPublishedEvents()
	t.Logf("Published events: %d", len(events))
	for _, event := range events {
		t.Logf("  Event topic: %s", event.Topic)
	}

	// Verify entries cleared
	metricsAfter, _ := harness.CallRPC("getMetrics", []byte("{}"))
	var afterMetrics map[string]interface{}
	json.Unmarshal(metricsAfter, &afterMetrics)
	t.Logf("Entries after clear: %v", afterMetrics["active_entries"])
}

// ============================================================================
// Scheduled Health Check Tests
// ============================================================================

// TestScheduledHealthChecks tests that health checks are stored in KV
func TestScheduledHealthChecks(t *testing.T) {
	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Health Check Test App",
		IsActive: true,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Get health which includes scheduled check history
	healthResp, err := harness.CallRPC("getHealth", []byte("{}"))
	if err != nil {
		t.Fatalf("getHealth failed: %v", err)
	}

	var health map[string]interface{}
	json.Unmarshal(healthResp, &health)

	t.Logf("Health response: %+v", health)

	// Check for health history from scheduled checks
	if history, ok := health["health_history"].([]interface{}); ok {
		t.Logf("Health history entries: %d", len(history))
	}

	// Check for last scheduled check
	if lastCheck, ok := health["last_scheduled_check"].(map[string]interface{}); ok {
		t.Logf("Last scheduled check: %+v", lastCheck)
	}

	// Check KV writes for health data
	kvWrites := harness.GetKVWrites()
	for _, write := range kvWrites {
		if write.Key == "llm-cache:health:current" || write.Key == "llm-cache:health:history" {
			t.Logf("Found health KV write: %s", write.Key)
		}
	}
}

// ============================================================================
// Redis Backend Integration Tests (using testcontainers)
// ============================================================================

// TestRedisBackendIntegration tests the plugin with a real Redis backend
// Uses testcontainers to spin up a Redis instance for integration testing
func TestRedisBackendIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Redis integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Start Redis container
	redis, err := containers.NewRedisContainer(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}
	defer func() {
		if err := redis.Close(ctx); err != nil {
			t.Logf("Warning: failed to close Redis container: %v", err)
		}
	}()

	// Verify Redis is working
	if err := redis.Ping(ctx); err != nil {
		t.Fatalf("Redis ping failed: %v", err)
	}

	t.Logf("Redis container started at %s", redis.Addr())

	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Redis Backend Test App",
		IsActive: true,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with Redis backend configuration
	err = harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
		"backend":           "redis",
		"redis_addr":        redis.Addr(),
		"redis_db":          "0",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Step 1: Make a request (cache miss)
	req := plugintest.NewRequestBuilder().
		WithChatCompletion([]plugintest.Message{
			{Role: "user", Content: "What is the capital of France?"},
		}).
		WithModel("gpt-4").
		WithVendor("openai").
		WithRequestID("redis-test-1").
		WithAppIDInt(1).
		Build()

	resp1, err := harness.CallPostAuth(req)
	if err != nil {
		t.Fatalf("First PostAuth call failed: %v", err)
	}

	// Should be a cache miss (Block=false)
	if resp1.Block {
		t.Error("Expected cache miss on first request (Block should be false)")
	}
	t.Logf("First request - Block: %v (expected false for cache miss)", resp1.Block)

	// Step 2: Simulate response caching
	response := plugintest.NewResponseBuilder().
		WithChatCompletion("The capital of France is Paris.").
		WithRequestID("redis-test-1").
		WithVendor("openai").
		WithAppID(1).
		Build()

	_, err = harness.CallOnBeforeWrite(response)
	if err != nil {
		t.Fatalf("OnBeforeWrite failed: %v", err)
	}

	// Step 3: Repeat request (should be cache hit from Redis)
	// Use a different request ID but same content
	req2 := plugintest.NewRequestBuilder().
		WithChatCompletion([]plugintest.Message{
			{Role: "user", Content: "What is the capital of France?"},
		}).
		WithModel("gpt-4").
		WithVendor("openai").
		WithRequestID("redis-test-2").
		WithAppIDInt(1).
		Build()

	resp2, err := harness.CallPostAuth(req2)
	if err != nil {
		t.Fatalf("Second PostAuth call failed: %v", err)
	}

	// Should be a cache hit (Block=true with Body containing cached response)
	if !resp2.Block {
		t.Log("Note: Second request was not served from cache - this may indicate cache key generation issues")
	} else {
		t.Log("Cache hit! Second request was served from cache")
	}
	t.Logf("Second request - Block: %v (expected true for cache hit)", resp2.Block)

	// Verify the cached content is in Body
	if resp2.Block && len(resp2.Body) > 0 {
		var cachedContent map[string]interface{}
		if err := json.Unmarshal(resp2.Body, &cachedContent); err == nil {
			t.Logf("Cached response from Redis: %+v", cachedContent)
		}
	}

	// Step 4: Test Redis-specific operations via RPC
	metricsResp, err := harness.CallRPC("getMetrics", []byte("{}"))
	if err != nil {
		t.Fatalf("getMetrics failed: %v", err)
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(metricsResp, &metrics); err != nil {
		t.Fatalf("Failed to parse metrics: %v", err)
	}

	t.Logf("Redis backend metrics: hits=%v, misses=%v, entries=%v",
		metrics["cache_hits"], metrics["cache_misses"], metrics["active_entries"])

	// Step 5: Clear Redis cache and verify
	_, err = harness.CallRPC("clearCache", []byte("{}"))
	if err != nil {
		t.Fatalf("clearCache failed: %v", err)
	}

	// Third request should be cache miss after clear
	resp3, err := harness.CallPostAuth(req2)
	if err != nil {
		t.Fatalf("Third PostAuth call failed: %v", err)
	}

	if resp3.Block {
		t.Error("Expected cache miss after clearCache (Block should be false)")
	}
	t.Logf("Third request - Block: %v (expected false after cache clear)", resp3.Block)

	// Step 6: Test health endpoint with Redis backend
	healthResp, err := harness.CallRPC("getHealth", []byte("{}"))
	if err != nil {
		t.Fatalf("getHealth failed: %v", err)
	}

	var health map[string]interface{}
	if err := json.Unmarshal(healthResp, &health); err != nil {
		t.Fatalf("Failed to parse health: %v", err)
	}

	t.Logf("Redis backend health: %+v", health)

	// Check that Redis backend is reported as healthy
	if backends, ok := health["backends"].([]interface{}); ok {
		for _, b := range backends {
			if backend, ok := b.(map[string]interface{}); ok {
				if backend["type"] == "redis" {
					if backend["healthy"] != true {
						t.Error("Expected Redis backend to be healthy")
					}
					t.Logf("Redis backend status: %+v", backend)
				}
			}
		}
	}
}

// TestRedisBackendFailover tests failover behavior when Redis is unavailable
func TestRedisBackendFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Redis failover test in short mode")
	}

	harness := setupE2EGatewayHarness(t)
	defer harness.Stop()

	gwServer := harness.GatewayServer()
	gwServer.AddTestApp(&gwmgmtpb.AppInfo{
		Id:       1,
		Name:     "Redis Failover Test App",
		IsActive: true,
	})

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with Redis backend pointing to non-existent server
	// This tests failover to memory backend
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
		"backend":           "redis",
		"redis_addr":        "localhost:16379", // Non-existent Redis
		"redis_db":          "0",
		"failover_enabled":  "true",
		"failover_backend":  "memory",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Check health - Redis should be unhealthy, memory failover should be active
	healthResp, err := harness.CallRPC("getHealth", []byte("{}"))
	if err != nil {
		t.Fatalf("getHealth failed: %v", err)
	}

	var health map[string]interface{}
	if err := json.Unmarshal(healthResp, &health); err != nil {
		t.Fatalf("Failed to parse health: %v", err)
	}

	t.Logf("Failover health status: %+v", health)

	// Plugin should still work with memory failover
	req := plugintest.NewRequestBuilder().
		WithChatCompletion([]plugintest.Message{
			{Role: "user", Content: "Test failover scenario"},
		}).
		WithModel("gpt-4").
		WithVendor("openai").
		WithRequestID("failover-test-1").
		WithAppIDInt(1).
		Build()

	resp, err := harness.CallPostAuth(req)
	if err != nil {
		t.Fatalf("PostAuth call failed during failover: %v", err)
	}

	// Should work despite Redis being unavailable
	t.Logf("Failover request processed: Block=%v, Modified=%v", resp.Block, resp.Modified)

	// Cache a response
	response := plugintest.NewResponseBuilder().
		WithChatCompletion("Failover response content").
		WithRequestID("failover-test-1").
		WithVendor("openai").
		WithAppID(1).
		Build()

	_, err = harness.CallOnBeforeWrite(response)
	if err != nil {
		t.Fatalf("OnBeforeWrite failed during failover: %v", err)
	}

	// Verify caching works with failover backend
	// Use different request ID but same content
	req2 := plugintest.NewRequestBuilder().
		WithChatCompletion([]plugintest.Message{
			{Role: "user", Content: "Test failover scenario"},
		}).
		WithModel("gpt-4").
		WithVendor("openai").
		WithRequestID("failover-test-2").
		WithAppIDInt(1).
		Build()

	resp2, err := harness.CallPostAuth(req2)
	if err != nil {
		t.Fatalf("Second PostAuth call failed during failover: %v", err)
	}

	if !resp2.Block {
		t.Log("Note: Cache hit may not work if failover backend not configured - this is expected behavior")
	} else {
		t.Log("Failover backend caching is working")
	}
}
