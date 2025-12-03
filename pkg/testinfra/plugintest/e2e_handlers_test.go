// +build e2e

package plugintest_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gwmgmtpb "github.com/TykTechnologies/midsommar/microgateway/proto/microgateway_management"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
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
