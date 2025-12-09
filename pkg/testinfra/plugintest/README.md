# Plugin Integration Testing Framework

This package provides E2E testing utilities for plugins. It enables true integration testing by spawning real plugin subprocesses via go-plugin, exactly mirroring how AI Studio and Microgateway load plugins in production.

## Features

- **E2E Plugin Harness**: Spawns real plugin binaries as subprocesses
- **Dual Runtime Support**: Test plugins in both AI Studio and Microgateway contexts
- **UI → RPC Contract Validation**: Detects mismatches between UI JavaScript and plugin handlers
- **Test Service Brokers**: Mock implementations of both AI Studio and Microgateway management services
- **Request/Response Builders**: Fluent builders for creating test proto messages with proper context

## Quick Start

### Running Unit Tests (No Build Tag)

The UI contract tests run without needing to build the plugin:

```bash
go test -v ./pkg/testinfra/plugintest/...
```

### Running E2E Tests (Requires Plugin Binary)

E2E tests require the `e2e` build tag and will compile the plugin binary.
The tests automatically include the `enterprise` build tag when compiling the plugin.

```bash
go test -v -tags e2e ./pkg/testinfra/plugintest/...
```

## Package Structure

```
pkg/testinfra/plugintest/
├── README.md                    # This documentation
├── e2e_harness.go               # E2E plugin harness (spawns subprocess)
├── test_service_broker.go       # Test implementations of gRPC services
├── request_builder.go           # Fluent builders for proto messages
├── ui_rpc_extractor.go          # JavaScript RPC method scanner
├── ui_contract_test.go          # UI → RPC contract validation tests
├── e2e_lifecycle_test.go        # Plugin lifecycle E2E tests
├── e2e_handlers_test.go         # Handler E2E tests
└── e2e_license_test.go          # License feature E2E tests
```

## Usage Examples

### UI → RPC Contract Validation

This catches mismatches like the `testBackend` bug where the UI called a method the plugin didn't implement:

```go
func TestMyPluginUIContract(t *testing.T) {
    uiDir := "path/to/plugin/ui/webc"

    // Extract RPC methods from JavaScript
    uiMethods, err := plugintest.ExtractRPCMethodsFromUI(uiDir)
    if err != nil {
        t.Fatalf("Failed to extract methods: %v", err)
    }

    // Check against implemented methods
    implemented := map[string]bool{
        "getMetrics": true,
        "clearCache": true,
        // ... add all your handlers
    }

    for _, method := range uiMethods {
        if !implemented[method] {
            t.Errorf("UI calls '%s' but plugin doesn't implement it", method)
        }
    }
}
```

### E2E Plugin Testing

```go
func TestMyPluginE2E(t *testing.T) {
    // Build and create harness
    harness := plugintest.NewE2EHarness("/path/to/plugin/binary")
    defer harness.Stop()

    // Configure license
    harness.SetLicense("enterprise", true, 365)
    harness.SetEntitlements([]string{"my-feature"})

    // Start plugin subprocess
    if err := harness.Start(); err != nil {
        t.Fatalf("Failed to start: %v", err)
    }

    // Initialize
    err := harness.Initialize(map[string]string{
        "enabled": "true",
    })
    if err != nil {
        t.Fatalf("Initialize failed: %v", err)
    }

    // Open session (triggers OnSessionReady)
    if err := harness.OpenSession(); err != nil {
        t.Fatalf("OpenSession failed: %v", err)
    }

    // Test RPC calls
    response, err := harness.CallRPC("getMetrics", []byte("{}"))
    if err != nil {
        t.Fatalf("RPC failed: %v", err)
    }

    t.Logf("Response: %s", response)
}
```

### Building Requests

```go
// Build a chat completion request
req := plugintest.NewRequestBuilder().
    WithChatCompletion([]plugintest.Message{
        {Role: "user", Content: "Hello!"},
    }).
    WithModel("gpt-4").
    WithVendor("openai").
    WithUserIDInt(1).
    WithAppIDInt(1).
    Build()

// Call the plugin
resp, err := harness.CallPostAuth(req)
```

### Building Responses

```go
// Build a chat completion response
resp := plugintest.NewResponseBuilder().
    WithChatCompletion("The answer is 42.").
    WithRequestID("req-123").  // Set context for proper tracking
    WithAppID(1).
    WithUserID(1).
    Build()

// Call OnBeforeWrite
writeResp, err := harness.CallOnBeforeWrite(resp)
```

### Gateway Runtime Testing

Plugins that implement gateway-specific handlers (`ProcessPostAuth`, `OnBeforeWrite`) need to run in gateway mode:

```go
func TestMyGatewayPlugin(t *testing.T) {
    harness := plugintest.NewE2EHarness("/path/to/plugin")
    defer harness.Stop()

    // Set runtime to Gateway (default is Studio)
    harness.SetRuntime(plugin_sdk.RuntimeGateway)

    // Configure enterprise license for gateway features
    harness.SetLicense("enterprise", true, 365)

    if err := harness.Start(); err != nil {
        t.Fatalf("Failed to start: %v", err)
    }

    // Initialize and open session
    harness.Initialize(map[string]string{"enabled": "true"})
    harness.OpenSession()

    // Build request with proper context
    req := plugintest.NewRequestBuilder().
        WithChatCompletion([]plugintest.Message{{Role: "user", Content: "Hi"}}).
        WithModel("gpt-4").
        WithVendor("openai").
        WithUserIDInt(1).
        WithAppIDInt(1).
        WithRequestID("test-req-123").  // Important for tracking
        Build()

    // Call gateway-specific handlers
    resp, err := harness.CallPostAuth(req)
    if err != nil {
        t.Fatalf("CallPostAuth failed: %v", err)
    }
}
```

### Using Gateway Test Services

The harness provides access to gateway-specific test data through `GatewayServer()`:

```go
harness := plugintest.NewE2EHarness("/path/to/plugin")
harness.SetRuntime(plugin_sdk.RuntimeGateway)

// Add test apps
harness.GatewayServer().AddTestApp(1, &gwmgmtpb.AppInfo{
    Id:   1,
    Name: "Test App",
    Slug: "test-app",
})

// Add test LLMs
harness.GatewayServer().AddTestLLM(1, &gwmgmtpb.LLMInfo{
    Id:     1,
    Name:   "GPT-4",
    Vendor: "openai",
})

// Configure budget status
harness.GatewayServer().SetBudgetStatus(1, &gwmgmtpb.GetBudgetStatusResponse{
    BudgetEnabled:   true,
    TotalBudget:     10000,
    UsedBudget:      5000,
    RemainingBudget: 5000,
})

// Add model pricing
harness.GatewayServer().AddModelPrice(&gwmgmtpb.ModelPriceInfo{
    Model:         "gpt-4",
    Vendor:        "openai",
    PromptPrice:   30000,  // $0.03 per 1K tokens
    ResponsePrice: 60000,  // $0.06 per 1K tokens
})
```

## Test Categories

### Unit Tests (No Build Tag)

- `TestAdvancedLLMCacheUIContract` - Validates UI/plugin RPC contract
- `TestUIRPCMethodExtraction` - Tests the JS scanner
- `TestUIRPCMethodsWithContext` - Tests detailed extraction
- `TestExpectedRPCMethods` - Documents expected methods

### E2E Tests (`-tags e2e`)

#### Lifecycle Tests
- `TestPluginBinaryBuild` - Verifies plugin compiles
- `TestPluginStartStop` - Tests clean start/stop
- `TestPluginInitializeBasic` - Tests basic initialization
- `TestPluginSessionLifecycle` - Tests full session lifecycle

#### Gateway Handler Tests (Microgateway Runtime)
These tests use `RuntimeGateway` and test gateway-specific plugin methods:
- `TestHandlerPostAuth` - Tests request handling via ProcessPostAuth
- `TestHandlerPostAuthWithBypass` - Tests cache bypass logic
- `TestHandlerOnBeforeWrite` - Tests response caching via OnBeforeWrite

#### Studio RPC Tests (AI Studio Runtime)
These tests use the default `RuntimeStudio`:
- `TestRPCGetMetrics` - Tests getMetrics RPC
- `TestRPCClearCache` - Tests clearCache RPC
- `TestRPCGetHealth` - Tests getHealth RPC
- `TestRPCGetConfig` - Tests getConfig RPC
- `TestRPCTestBackend` - Tests testBackend RPC
- `TestRPCUnknownMethod` - Tests error handling

#### License Tests
- `TestLicenseEnterpriseFeaturesEnabled` - Tests enterprise features
- `TestLicenseCommunityRestrictions` - Tests community restrictions
- `TestLicenseExpiringSoon` - Tests expiring license
- `TestLicenseExpired` - Tests expired license
- `TestLicenseEntitlementCheck` - Tests entitlement validation
- `TestLicenseServiceBrokerIntegration` - Tests service broker integration

## E2EPluginHarness API

### Setup Methods

```go
NewE2EHarness(pluginBinaryPath string) *E2EPluginHarness
SetLicense(licenseType string, valid bool, daysRemaining int)
SetEntitlements(entitlements []string)
SetRuntime(rt plugin_sdk.RuntimeType)
SetKVData(key string, value []byte)
```

### Lifecycle Methods

```go
Start() error
Stop()
ProcessExited() bool
Initialize(config map[string]string) error
OpenSession() error
```

### Call Methods

```go
CallRPC(method string, payload []byte) ([]byte, error)
CallPostAuth(req *pb.EnrichedRequest) (*pb.PluginResponse, error)
CallOnBeforeWrite(req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error)
```

### Inspection Methods

```go
LicenseWasChecked() bool
GetKVWrites() []KVWrite
GetPublishedEvents() []Event
InjectEvent(topic string, payload []byte)
GatewayServer() *TestGatewayManagementServer  // Access gateway test services
```

## TestGatewayManagementServer API

The gateway server implements `MicrogatewayManagementServiceServer` for testing plugins in gateway mode.

### Configuration Methods

```go
// License configuration
SetLicense(license *LicenseInfo)

// App management
AddTestApp(id uint32, app *gwmgmtpb.AppInfo)

// LLM management
AddTestLLM(id uint32, llm *gwmgmtpb.LLMInfo)

// Budget configuration
SetBudgetStatus(appID uint32, status *gwmgmtpb.GetBudgetStatusResponse)

// Model pricing
AddModelPrice(price *gwmgmtpb.ModelPriceInfo)
```

### Inspection Methods

```go
// Get control payloads sent by plugin
GetControlPayloads() []ControlPayload

// Get all service calls made by plugin
GetCalls() []ServiceCall

// Reset all state for new test
Reset()
```

## Adding Tests for New Plugins

1. Create a new test file (e.g., `e2e_myplugin_test.go`)
2. Use `setupE2EHarness` pattern for building the plugin
3. Configure license/entitlements as needed
4. Test the specific RPC methods and handlers

```go
// +build e2e

func TestMyNewPlugin(t *testing.T) {
    // Setup harness (modify for your plugin location)
    harness := setupE2EHarnessForMyPlugin(t)
    defer harness.Stop()

    // ... your tests
}

func setupE2EHarnessForMyPlugin(t *testing.T) *plugintest.E2EPluginHarness {
    t.Helper()

    projectRoot := findProjectRoot(t)
    pluginDir := filepath.Join(projectRoot, "path", "to", "my-plugin")

    binaryPath := filepath.Join(t.TempDir(), "my-plugin")
    // Add any required build tags (e.g., "enterprise" for enterprise plugins)
    cmd := exec.Command("go", "build", "-tags", "enterprise", "-o", binaryPath, ".")
    cmd.Dir = pluginDir

    output, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("Build failed: %v\n%s", err, output)
    }

    return plugintest.NewE2EHarness(binaryPath)
}
```

## CI Integration

Add to your CI pipeline:

```yaml
# Run unit tests (fast, no plugin build)
- name: Plugin Contract Tests
  run: go test -v ./pkg/testinfra/plugintest/...

# Run E2E tests (slower, builds plugin)
- name: Plugin E2E Tests
  run: go test -v -tags e2e ./pkg/testinfra/plugintest/...
```

## Troubleshooting

### "Plugin binary not found" or "build constraints exclude all Go files"

The E2E tests build the plugin binary automatically. If this fails:

1. Check the plugin directory exists
2. Verify the plugin compiles with correct build tags: `go build -tags enterprise ./enterprise/plugins/advanced-llm-cache/...`
3. Check for missing dependencies
4. Enterprise plugins require the `enterprise` build tag

### "Failed to create plugin client"

The go-plugin handshake failed. Check:

1. Plugin main() calls `plugin_sdk.Serve()`
2. Magic cookie values match
3. Plugin isn't writing to stdout before serving

### "OpenSession timeout"

The session didn't establish in time. Check:

1. Plugin implements OnSessionReady correctly
2. Service broker is accessible
3. Increase timeout if needed

### UI Method Not Found

If `TestAdvancedLLMCacheUIContract` fails with missing methods:

1. Check the UI JavaScript for the exact method name
2. Verify the plugin's HandleRPC switch statement includes the method
3. Method names are case-sensitive
