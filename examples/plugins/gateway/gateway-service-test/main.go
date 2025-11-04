package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	mgmtpb "github.com/TykTechnologies/midsommar/microgateway/proto/microgateway_management"
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
	// Note: Broker ID and Plugin ID are automatically set up by unified SDK
	// No manual setup needed
	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *GatewayServiceTestPlugin) Shutdown(ctx plugin_sdk.Context) error {
	return nil
}

// GetManifest implements plugin_sdk.ManifestProvider
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
	// Check if Gateway services are available
	if ctx.Services.Gateway() == nil {
		errorResponse := map[string]interface{}{
			"error":   "Gateway services not available",
			"message": "Service API not available in this context",
		}
		body, _ := json.Marshal(errorResponse)
		return &pb.PluginResponse{
			Block:      true,
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       body,
		}, nil
	}

	// Run all tests with plugin context
	report := runAllTests(ctx, req.Request.Context)

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
func runAllTests(pluginCtx plugin_sdk.Context, reqCtx *pb.PluginContext) *TestReport {
	report := &TestReport{
		StartTime: time.Now(),
	}

	// Extract and enrich context metadata
	report.Context = extractContextMetadata(pluginCtx, reqCtx)

	// Run test suites with plugin context
	report.LLMTests = runLLMTests(pluginCtx)
	report.AppTests = runAppTests(pluginCtx)
	report.BudgetTests = runBudgetTests(pluginCtx)
	report.PricingTests = runPricingTests(pluginCtx)
	report.CredentialTests = runCredentialTests(pluginCtx)
	report.KVTests = runKVTests(pluginCtx)

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
func runLLMTests(pluginCtx plugin_sdk.Context) []TestResult {
	var results []TestResult
	ctx := pluginCtx.Context

	// Test: List LLMs
	start := time.Now()
	resp, err := pluginCtx.Services.Gateway().ListLLMs(ctx, 1, 10, "", nil)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, TestResult{
			Name:     "List LLMs",
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		})
		return results
	}

	// Type assert the response
	llmListResp, ok := resp.(*mgmtpb.ListLLMsResponse)
	if !ok {
		results = append(results, TestResult{
			Name:     "List LLMs",
			Success:  false,
			Error:    "Failed to parse ListLLMs response",
			Duration: duration,
		})
		return results
	}

	results = append(results, TestResult{
		Name:     "List LLMs",
		Success:  true,
		Duration: duration,
		Details:  fmt.Sprintf("Found %d LLMs", llmListResp.TotalCount),
	})

	// Test: Get LLM (if any exist)
	if len(llmListResp.Llms) > 0 {
		start = time.Now()
		llmResp, err := pluginCtx.Services.Gateway().GetLLM(ctx, llmListResp.Llms[0].Id)
		duration = time.Since(start).Milliseconds()

		if err != nil {
			results = append(results, TestResult{
				Name:     "Get LLM",
				Success:  false,
				Error:    err.Error(),
				Duration: duration,
			})
		} else if llmGetResp, ok := llmResp.(*mgmtpb.GetLLMResponse); ok && llmGetResp.Llm != nil {
			results = append(results, TestResult{
				Name:     "Get LLM",
				Success:  true,
				Duration: duration,
				Details:  fmt.Sprintf("Retrieved LLM: %s", llmGetResp.Llm.Name),
			})
		} else {
			results = append(results, TestResult{
				Name:     "Get LLM",
				Success:  false,
				Error:    "Failed to parse GetLLM response",
				Duration: duration,
			})
		}
	}

	return results
}

// runAppTests tests App service operations
func runAppTests(pluginCtx plugin_sdk.Context) []TestResult {
	var results []TestResult
	ctx := pluginCtx.Context

	// Test: List Apps
	start := time.Now()
	resp, err := pluginCtx.Services.Gateway().ListApps(ctx, 1, 10, nil)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, TestResult{
			Name:     "List Apps",
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		})
		return results
	}

	listResp, ok := resp.(*mgmtpb.ListAppsResponse)
	if !ok {
		results = append(results, TestResult{
			Name:     "List Apps",
			Success:  false,
			Error:    "Failed to parse ListApps response",
			Duration: duration,
		})
		return results
	}

	results = append(results, TestResult{
		Name:     "List Apps",
		Success:  true,
		Duration: duration,
		Details:  fmt.Sprintf("Found %d apps", listResp.TotalCount),
	})

	// Test: Get App (if any exist)
	if len(listResp.Apps) > 0 {
		start = time.Now()
		appResp, err := pluginCtx.Services.Gateway().GetApp(ctx, listResp.Apps[0].Id)
		duration = time.Since(start).Milliseconds()

		if err != nil {
			results = append(results, TestResult{
				Name:     "Get App",
				Success:  false,
				Error:    err.Error(),
				Duration: duration,
			})
		} else if getResp, ok := appResp.(*mgmtpb.GetAppResponse); ok && getResp.App != nil {
			results = append(results, TestResult{
				Name:     "Get App",
				Success:  true,
				Duration: duration,
				Details:  fmt.Sprintf("Retrieved app: %s", getResp.App.Name),
			})
		} else {
			results = append(results, TestResult{
				Name:     "Get App",
				Success:  false,
				Error:    "Failed to parse GetApp response",
				Duration: duration,
			})
		}
	}

	return results
}

// runBudgetTests tests Budget service operations
func runBudgetTests(pluginCtx plugin_sdk.Context) []TestResult {
	var results []TestResult
	ctx := pluginCtx.Context

	// First get an app to test with
	appsResp, err := pluginCtx.Services.Gateway().ListApps(ctx, 1, 1, nil)
	if err != nil {
		results = append(results, TestResult{
			Name:     "Get Budget Status",
			Success:  false,
			Error:    fmt.Sprintf("Failed to list apps: %v", err),
			Duration: 0,
		})
		return results
	}

	listResp, ok := appsResp.(*mgmtpb.ListAppsResponse)
	if !ok || len(listResp.Apps) == 0 {
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
	budgetResp, err := pluginCtx.Services.Gateway().GetBudgetStatus(ctx, listResp.Apps[0].Id, nil)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, TestResult{
			Name:     "Get Budget Status",
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		})
		return results
	}

	if getBudgetResp, ok := budgetResp.(*mgmtpb.GetBudgetStatusResponse); ok {
		results = append(results, TestResult{
			Name:     "Get Budget Status",
			Success:  true,
			Duration: duration,
			Details:  fmt.Sprintf("Monthly budget: %.2f, Usage: %.2f", getBudgetResp.MonthlyBudget, getBudgetResp.CurrentUsage),
		})
	} else {
		results = append(results, TestResult{
			Name:     "Get Budget Status",
			Success:  false,
			Error:    "Failed to parse GetBudgetStatus response",
			Duration: duration,
		})
	}

	return results
}

// runPricingTests tests Model Price service operations
func runPricingTests(pluginCtx plugin_sdk.Context) []TestResult {
	var results []TestResult
	ctx := pluginCtx.Context

	// Test: List Model Prices
	start := time.Now()
	resp, err := pluginCtx.Services.Gateway().ListModelPrices(ctx, "")
	duration := time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, TestResult{
			Name:     "List Model Prices",
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		})
		return results
	}

	listPriceResp, ok := resp.(*mgmtpb.ListModelPricesResponse)
	if !ok {
		results = append(results, TestResult{
			Name:     "List Model Prices",
			Success:  false,
			Error:    "Failed to parse ListModelPrices response",
			Duration: duration,
		})
		return results
	}

	results = append(results, TestResult{
		Name:     "List Model Prices",
		Success:  true,
		Duration: duration,
		Details:  fmt.Sprintf("Found %d model prices", len(listPriceResp.ModelPrices)),
	})

	// Test: Get Model Price (if any exist)
	if len(listPriceResp.ModelPrices) > 0 {
		start = time.Now()
		priceResp, err := pluginCtx.Services.Gateway().GetModelPrice(ctx, listPriceResp.ModelPrices[0].ModelName, listPriceResp.ModelPrices[0].Vendor)
		duration = time.Since(start).Milliseconds()

		if err != nil {
			results = append(results, TestResult{
				Name:     "Get Model Price",
				Success:  false,
				Error:    err.Error(),
				Duration: duration,
			})
		} else if getPriceResp, ok := priceResp.(*mgmtpb.GetModelPriceResponse); ok && getPriceResp.ModelPrice != nil {
			results = append(results, TestResult{
				Name:     "Get Model Price",
				Success:  true,
				Duration: duration,
				Details:  fmt.Sprintf("Model: %s, CPT: %.6f", getPriceResp.ModelPrice.ModelName, getPriceResp.ModelPrice.Cpt),
			})
		} else {
			results = append(results, TestResult{
				Name:     "Get Model Price",
				Success:  false,
				Error:    "Failed to parse GetModelPrice response",
				Duration: duration,
			})
		}
	}

	return results
}

// runCredentialTests tests Credential service operations
func runCredentialTests(pluginCtx plugin_sdk.Context) []TestResult {
	var results []TestResult
	ctx := pluginCtx.Context

	// Test: Validate Credential (with invalid credential - should fail gracefully)
	start := time.Now()
	resp, err := pluginCtx.Services.Gateway().ValidateCredential(ctx, "invalid-test-credential")
	duration := time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, TestResult{
			Name:     "Validate Credential",
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		})
		return results
	}

	if validateResp, ok := resp.(*mgmtpb.ValidateCredentialResponse); ok {
		results = append(results, TestResult{
			Name:     "Validate Credential",
			Success:  true,
			Duration: duration,
			Details:  fmt.Sprintf("Credential validation returned (expected invalid): valid=%v", validateResp.Valid),
		})
	} else {
		results = append(results, TestResult{
			Name:     "Validate Credential",
			Success:  false,
			Error:    "Failed to parse ValidateCredential response",
			Duration: duration,
		})
	}

	return results
}

// runKVTests tests Plugin KV storage operations
func runKVTests(pluginCtx plugin_sdk.Context) []TestResult {
	var results []TestResult
	ctx := pluginCtx.Context

	testKey := "test_key"
	testValue := []byte("test_value_" + time.Now().Format("20060102150405"))

	// Test: Write KV
	start := time.Now()
	created, err := pluginCtx.Services.KV().Write(ctx, testKey, testValue, nil) // No expiration for test
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
	readValue, err := pluginCtx.Services.KV().Read(ctx, testKey)
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
	deleted, err := pluginCtx.Services.KV().Delete(ctx, testKey)
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
func extractContextMetadata(pluginCtx plugin_sdk.Context, reqCtx *pb.PluginContext) *ContextMetadata {
	ctx := pluginCtx.Context

	metadata := &ContextMetadata{
		RequestID: reqCtx.RequestId,
		LLMID:     uint(reqCtx.LlmId),
		LLMSlug:   reqCtx.LlmSlug,
		Vendor:    reqCtx.Vendor,
		AppID:     uint(reqCtx.AppId),
		UserID:    uint(reqCtx.UserId),
		Metadata:  reqCtx.Metadata,
	}

	// Try to get LLM details from service API
	if reqCtx.LlmId > 0 {
		if llmResp, err := pluginCtx.Services.Gateway().GetLLM(ctx, reqCtx.LlmId); err == nil {
			if getLLMResp, ok := llmResp.(*mgmtpb.GetLLMResponse); ok && getLLMResp.Llm != nil {
				metadata.LLMName = getLLMResp.Llm.Name
			}
		}
	}

	// Try to get App details from service API and serialize all fields
	if reqCtx.AppId > 0 {
		appResp, err := pluginCtx.Services.Gateway().GetApp(ctx, reqCtx.AppId)
		if err != nil {
			// Error getting app - skip details
		} else if getAppResp, ok := appResp.(*mgmtpb.GetAppResponse); ok && getAppResp.App != nil {
			// Parse metadata JSON if present
			var appMetadata map[string]interface{}
			if getAppResp.App.Metadata != "" {
				json.Unmarshal([]byte(getAppResp.App.Metadata), &appMetadata)
			}

			// Convert App protobuf to map to show all fields
			appDetails := map[string]interface{}{
				"id":               getAppResp.App.Id,
				"name":             getAppResp.App.Name,
				"description":      getAppResp.App.Description,
				"owner_email":      getAppResp.App.OwnerEmail,
				"is_active":        getAppResp.App.IsActive,
				"monthly_budget":   getAppResp.App.MonthlyBudget,
				"budget_reset_day": getAppResp.App.BudgetResetDay,
				"rate_limit_rpm":   getAppResp.App.RateLimitRpm,
				"allowed_ips":      getAppResp.App.AllowedIps,
				"metadata":         appMetadata,
				"created_at":       getAppResp.App.CreatedAt.AsTime().Format(time.RFC3339),
				"updated_at":       getAppResp.App.UpdatedAt.AsTime().Format(time.RFC3339),
			}
			metadata.AppDetails = appDetails
		}
	}

	return metadata
}

func main() {
	plugin := NewGatewayServiceTestPlugin()
	plugin_sdk.Serve(plugin)
}
