package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	mgwsdk "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

//go:embed manifest.json
var manifestBytes []byte

const (
	PluginName    = "gateway-service-test"
	PluginVersion = "1.0.0"
)

// GatewayServiceTestPlugin tests all microgateway service APIs
type GatewayServiceTestPlugin struct {
	plugin_sdk.BasePlugin
}

// NewGatewayServiceTestPlugin creates a new gateway service test plugin
func NewGatewayServiceTestPlugin() *GatewayServiceTestPlugin {
	return &GatewayServiceTestPlugin{
		BasePlugin: plugin_sdk.NewBasePlugin(PluginName, PluginVersion, "Gateway Service API Test Plugin"),
	}
}

// Initialize implements plugin_sdk.Plugin
func (p *GatewayServiceTestPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	fmt.Printf("🧪 %s: Initialized in %s runtime\n", PluginName, ctx.Runtime)

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
			mgwsdk.SetServiceBrokerID(brokerID)
			fmt.Printf("🧪 %s: Set service broker ID: %d\n", PluginName, brokerID)
		}
	}

	// Extract plugin ID if present
	if pluginIDStr, ok := config["_plugin_id"]; ok {
		var pluginID uint32
		fmt.Sscanf(pluginIDStr, "%d", &pluginID)
		mgwsdk.SetPluginID(pluginID)
		fmt.Printf("🧪 %s: Plugin ID set to: %d\n", PluginName, pluginID)
	}

	fmt.Printf("✅ %s: Initialized successfully\n", PluginName)
	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *GatewayServiceTestPlugin) Shutdown(ctx plugin_sdk.Context) error {
	fmt.Printf("🧪 %s: Shutdown called\n", PluginName)
	return nil
}

// GetManifest implements plugin_sdk.UIProvider
func (p *GatewayServiceTestPlugin) GetManifest() ([]byte, error) {
	return manifestBytes, nil
}

// GetConfigSchema implements plugin_sdk.ConfigProvider
func (p *GatewayServiceTestPlugin) GetConfigSchema() ([]byte, error) {
	schema := map[string]interface{}{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"type":        "object",
		"title":       "Gateway Service Test Plugin Configuration",
		"description": "No configuration needed - this plugin tests service API access",
		"properties":  map[string]interface{}{},
	}

	return json.Marshal(schema)
}

// HandlePostAuth implements plugin_sdk.PostAuthHandler - runs all service API tests and returns results
func (p *GatewayServiceTestPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	fmt.Println("🧪 Starting Gateway Service API Tests...")

	// Check if SDK is initialized
	if !mgwsdk.IsInitialized() {
		errorResponse := map[string]interface{}{
			"error":   "SDK not initialized",
			"message": "Service API unavailable - broker not setup",
		}
		body, _ := json.Marshal(errorResponse)
		return &pb.PluginResponse{
			Block:      true,
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       body,
		}, nil
	}

	// Run all tests
	report := runAllTests(ctx.Context, req.Request.Context)

	// Convert report to JSON
	reportJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		errorResponse := map[string]interface{}{
			"error":   "Failed to serialize test report",
			"message": err.Error(),
		}
		body, _ := json.Marshal(errorResponse)
		return &pb.PluginResponse{
			Block:      true,
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       body,
		}, nil
	}

	fmt.Printf("✅ Tests completed: %d passed, %d failed out of %d total\n",
		report.PassedTests, report.FailedTests, report.TotalTests)

	// Block the request and return test results directly to user
	return &pb.PluginResponse{
		Block:      true,
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       reportJSON,
	}, nil
}

// TestResult represents a single test result
type TestResult struct {
	Name     string `json:"name"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
	Duration int64  `json:"duration_ms"`
	Details  string `json:"details,omitempty"`
}

// ContextMetadata holds metadata about the current request context
type ContextMetadata struct {
	RequestID   string                 `json:"request_id"`
	LLMID       uint                   `json:"llm_id"`
	LLMSlug     string                 `json:"llm_slug"`
	LLMName     string                 `json:"llm_name,omitempty"`
	Vendor      string                 `json:"vendor"`
	AppID       uint                   `json:"app_id"`
	AppDetails  map[string]interface{} `json:"app_details,omitempty"` // Full app data from GetApp()
	UserID      uint                   `json:"user_id"`
	Metadata    map[string]string      `json:"metadata,omitempty"` // Changed to match proto
}

// TestReport aggregates all test results
type TestReport struct {
	Context     *ContextMetadata `json:"request_context"`
	StartTime   time.Time        `json:"start_time"`
	EndTime     time.Time        `json:"end_time"`
	Duration    int64            `json:"total_duration_ms"`
	TotalTests  int              `json:"total_tests"`
	PassedTests int              `json:"passed_tests"`
	FailedTests int              `json:"failed_tests"`
	LLMTests    []TestResult     `json:"llm_tests"`
	AppTests    []TestResult     `json:"app_tests"`
	BudgetTests []TestResult     `json:"budget_tests"`
	PricingTests []TestResult    `json:"pricing_tests"`
	CredentialTests []TestResult `json:"credential_tests"`
	KVTests     []TestResult     `json:"kv_tests"`
}

// runAllTests executes all service API tests
func runAllTests(ctx context.Context, pluginCtx *pb.PluginContext) *TestReport {
	report := &TestReport{
		StartTime: time.Now(),
	}

	// Extract and enrich context metadata
	report.Context = extractContextMetadata(ctx, pluginCtx)

	// Run test suites
	report.LLMTests = runLLMTests(ctx)
	report.AppTests = runAppTests(ctx)
	report.BudgetTests = runBudgetTests(ctx)
	report.PricingTests = runPricingTests(ctx)
	report.CredentialTests = runCredentialTests(ctx)
	report.KVTests = runKVTests(ctx)

	// Calculate summary
	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(report.StartTime).Milliseconds()

	allTests := append(report.LLMTests, report.AppTests...)
	allTests = append(allTests, report.BudgetTests...)
	allTests = append(allTests, report.PricingTests...)
	allTests = append(allTests, report.CredentialTests...)
	allTests = append(allTests, report.KVTests...)

	report.TotalTests = len(allTests)
	for _, test := range allTests {
		if test.Success {
			report.PassedTests++
		} else {
			report.FailedTests++
		}
	}

	return report
}

// runLLMTests tests LLM service operations
func runLLMTests(ctx context.Context) []TestResult {
	var results []TestResult

	// Test: List LLMs
	start := time.Now()
	resp, err := mgwsdk.ListLLMs(ctx, 1, 10, "", nil)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, TestResult{
			Name:     "List LLMs",
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		})
	} else {
		results = append(results, TestResult{
			Name:     "List LLMs",
			Success:  true,
			Duration: duration,
			Details:  fmt.Sprintf("Found %d LLMs", resp.TotalCount),
		})

		// Test: Get LLM (if any exist)
		if len(resp.Llms) > 0 {
			start = time.Now()
			llmResp, err := mgwsdk.GetLLM(ctx, resp.Llms[0].Id)
			duration = time.Since(start).Milliseconds()

			if err != nil {
				results = append(results, TestResult{
					Name:     "Get LLM",
					Success:  false,
					Error:    err.Error(),
					Duration: duration,
				})
			} else {
				results = append(results, TestResult{
					Name:     "Get LLM",
					Success:  true,
					Duration: duration,
					Details:  fmt.Sprintf("Retrieved LLM: %s", llmResp.Llm.Name),
				})
			}
		}
	}

	return results
}

// runAppTests tests App service operations
func runAppTests(ctx context.Context) []TestResult {
	var results []TestResult

	// Test: List Apps
	start := time.Now()
	resp, err := mgwsdk.ListApps(ctx, 1, 10, nil)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, TestResult{
			Name:     "List Apps",
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		})
	} else {
		results = append(results, TestResult{
			Name:     "List Apps",
			Success:  true,
			Duration: duration,
			Details:  fmt.Sprintf("Found %d apps", resp.TotalCount),
		})

		// Test: Get App (if any exist)
		if len(resp.Apps) > 0 {
			start = time.Now()
			appResp, err := mgwsdk.GetApp(ctx, resp.Apps[0].Id)
			duration = time.Since(start).Milliseconds()

			if err != nil {
				results = append(results, TestResult{
					Name:     "Get App",
					Success:  false,
					Error:    err.Error(),
					Duration: duration,
				})
			} else {
				results = append(results, TestResult{
					Name:     "Get App",
					Success:  true,
					Duration: duration,
					Details:  fmt.Sprintf("Retrieved app: %s", appResp.App.Name),
				})
			}
		}
	}

	return results
}

// runBudgetTests tests Budget service operations
func runBudgetTests(ctx context.Context) []TestResult {
	var results []TestResult

	// First get an app to test with
	appResp, err := mgwsdk.ListApps(ctx, 1, 1, nil)
	if err != nil || len(appResp.Apps) == 0 {
		results = append(results, TestResult{
			Name:     "Get Budget Status",
			Success:  false,
			Error:    "No apps available to test budget status",
			Duration: 0,
		})
		return results
	}

	// Test: Get Budget Status
	start := time.Now()
	budgetResp, err := mgwsdk.GetBudgetStatus(ctx, appResp.Apps[0].Id, nil)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, TestResult{
			Name:     "Get Budget Status",
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		})
	} else {
		results = append(results, TestResult{
			Name:     "Get Budget Status",
			Success:  true,
			Duration: duration,
			Details:  fmt.Sprintf("Monthly budget: %.2f, Usage: %.2f", budgetResp.MonthlyBudget, budgetResp.CurrentUsage),
		})
	}

	return results
}

// runPricingTests tests Model Price service operations
func runPricingTests(ctx context.Context) []TestResult {
	var results []TestResult

	// Test: List Model Prices
	start := time.Now()
	resp, err := mgwsdk.ListModelPrices(ctx, "")
	duration := time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, TestResult{
			Name:     "List Model Prices",
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		})
	} else {
		results = append(results, TestResult{
			Name:     "List Model Prices",
			Success:  true,
			Duration: duration,
			Details:  fmt.Sprintf("Found %d model prices", len(resp.ModelPrices)),
		})

		// Test: Get Model Price (if any exist)
		if len(resp.ModelPrices) > 0 {
			start = time.Now()
			priceResp, err := mgwsdk.GetModelPrice(ctx, resp.ModelPrices[0].ModelName, resp.ModelPrices[0].Vendor)
			duration = time.Since(start).Milliseconds()

			if err != nil {
				results = append(results, TestResult{
					Name:     "Get Model Price",
					Success:  false,
					Error:    err.Error(),
					Duration: duration,
				})
			} else {
				results = append(results, TestResult{
					Name:     "Get Model Price",
					Success:  true,
					Duration: duration,
					Details:  fmt.Sprintf("Model: %s, CPT: %.6f", priceResp.ModelPrice.ModelName, priceResp.ModelPrice.Cpt),
				})
			}
		}
	}

	return results
}

// runCredentialTests tests Credential service operations
func runCredentialTests(ctx context.Context) []TestResult {
	var results []TestResult

	// Test: Validate Credential (with invalid credential - should fail gracefully)
	start := time.Now()
	resp, err := mgwsdk.ValidateCredential(ctx, "invalid-test-credential")
	duration := time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, TestResult{
			Name:     "Validate Credential",
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		})
	} else {
		results = append(results, TestResult{
			Name:     "Validate Credential",
			Success:  true,
			Duration: duration,
			Details:  fmt.Sprintf("Credential validation returned (expected invalid): valid=%v", resp.Valid),
		})
	}

	return results
}

// runKVTests tests Plugin KV storage operations
func runKVTests(ctx context.Context) []TestResult {
	var results []TestResult

	testKey := "test_key"
	testValue := []byte("test_value_" + time.Now().Format("20060102150405"))

	// Test: Write KV
	start := time.Now()
	created, err := mgwsdk.WritePluginKV(ctx, testKey, testValue, nil) // No expiration for test
	duration := time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, TestResult{
			Name:     "Write Plugin KV",
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		})
		return results // Can't continue without successful write
	}

	results = append(results, TestResult{
		Name:     "Write Plugin KV",
		Success:  true,
		Duration: duration,
		Details:  fmt.Sprintf("Created: %v", created),
	})

	// Test: Read KV
	start = time.Now()
	readValue, err := mgwsdk.ReadPluginKV(ctx, testKey)
	duration = time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, TestResult{
			Name:     "Read Plugin KV",
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		})
	} else if string(readValue) != string(testValue) {
		results = append(results, TestResult{
			Name:     "Read Plugin KV",
			Success:  false,
			Error:    fmt.Sprintf("Value mismatch: expected %s, got %s", testValue, readValue),
			Duration: duration,
		})
	} else {
		results = append(results, TestResult{
			Name:     "Read Plugin KV",
			Success:  true,
			Duration: duration,
			Details:  fmt.Sprintf("Successfully read %d bytes", len(readValue)),
		})
	}

	// Test: Delete KV
	start = time.Now()
	deleted, err := mgwsdk.DeletePluginKV(ctx, testKey)
	duration = time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, TestResult{
			Name:     "Delete Plugin KV",
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		})
	} else {
		results = append(results, TestResult{
			Name:     "Delete Plugin KV",
			Success:  true,
			Duration: duration,
			Details:  fmt.Sprintf("Deleted: %v", deleted),
		})
	}

	return results
}

// extractContextMetadata extracts and enriches metadata from the plugin context
func extractContextMetadata(ctx context.Context, pluginCtx *pb.PluginContext) *ContextMetadata {
	metadata := &ContextMetadata{
		RequestID: pluginCtx.RequestId,
		LLMID:     uint(pluginCtx.LlmId),
		LLMSlug:   pluginCtx.LlmSlug,
		Vendor:    pluginCtx.Vendor,
		AppID:     uint(pluginCtx.AppId),
		UserID:    uint(pluginCtx.UserId),
		Metadata:  pluginCtx.Metadata,
	}

	// Try to get LLM details from service API
	if pluginCtx.LlmId > 0 {
		if llmResp, err := mgwsdk.GetLLM(ctx, pluginCtx.LlmId); err == nil {
			metadata.LLMName = llmResp.Llm.Name
		}
	}

	// Try to get App details from service API and serialize all fields
	if pluginCtx.AppId > 0 {
		appResp, err := mgwsdk.GetApp(ctx, pluginCtx.AppId)
		if err != nil {
			fmt.Printf("⚠️ Failed to get app details for app_id=%d: %v\n", pluginCtx.AppId, err)
		} else if appResp == nil {
			fmt.Printf("⚠️ GetApp returned nil response for app_id=%d\n", pluginCtx.AppId)
		} else if appResp.App == nil {
			fmt.Printf("⚠️ GetApp response has nil App for app_id=%d\n", pluginCtx.AppId)
		} else {
			// Parse metadata JSON if present
			var appMetadata map[string]interface{}
			if appResp.App.Metadata != "" {
				json.Unmarshal([]byte(appResp.App.Metadata), &appMetadata)
			}

			// Convert App protobuf to map to show all fields
			appDetails := map[string]interface{}{
				"id":               appResp.App.Id,
				"name":             appResp.App.Name,
				"description":      appResp.App.Description,
				"owner_email":      appResp.App.OwnerEmail,
				"is_active":        appResp.App.IsActive,
				"monthly_budget":   appResp.App.MonthlyBudget,
				"budget_reset_day": appResp.App.BudgetResetDay,
				"rate_limit_rpm":   appResp.App.RateLimitRpm,
				"allowed_ips":      appResp.App.AllowedIps,
				"metadata":         appMetadata,
				"created_at":       appResp.App.CreatedAt.AsTime().Format(time.RFC3339),
				"updated_at":       appResp.App.UpdatedAt.AsTime().Format(time.RFC3339),
			}
			metadata.AppDetails = appDetails
			fmt.Printf("✅ Successfully retrieved app details for app_id=%d: %s\n", pluginCtx.AppId, appResp.App.Name)
		}
	}

	return metadata
}

func main() {
	fmt.Printf("🧪 Starting %s Plugin v%s\n", PluginName, PluginVersion)
	fmt.Printf("Gateway service API testing plugin using unified SDK\n")

	plugin := NewGatewayServiceTestPlugin()
	plugin_sdk.Serve(plugin)
}
