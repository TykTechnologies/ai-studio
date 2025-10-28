package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
)

// TestReport aggregates all test results
type TestReport struct {
	StartTime       time.Time               `json:"start_time"`
	EndTime         time.Time               `json:"end_time"`
	TotalDuration   time.Duration           `json:"total_duration_ms"`
	TotalTests      int                     `json:"total_tests"`
	PassedTests     int                     `json:"passed_tests"`
	FailedTests     int                     `json:"failed_tests"`
	LLMTests        []TestResult            `json:"llm_tests"`
	AppTests        []TestResult            `json:"app_tests"`
	ToolTests       []TestResult            `json:"tool_tests"`
	DatasourceTests []TestResult            `json:"datasource_tests"`
	TagTests        []TestResult            `json:"tag_tests"`
	FilterTests     []TestResult            `json:"filter_tests"`
	ModelPriceTests []TestResult            `json:"model_price_tests"`
	DataCatalogueTests []TestResult         `json:"data_catalogue_tests"`
	KVTests         []TestResult            `json:"kv_tests"`
	CleanupResults  []TestResult            `json:"cleanup_results"`
}

// CreatedResources tracks all resources created during testing for cleanup
type CreatedResources struct {
	LLMs           []uint32 `json:"llms"`
	Apps           []uint32 `json:"apps"`
	Tools          []uint32 `json:"tools"`
	Datasources    []uint32 `json:"datasources"`
	Tags           []uint32 `json:"tags"`
	Filters        []uint32 `json:"filters"`
	ModelPrices    []uint32 `json:"model_prices"`
	DataCatalogues []uint32 `json:"data_catalogues"`
	KVKeys         []string `json:"kv_keys"`
}

// RunE2ETests executes all service API tests and returns comprehensive results
func RunE2ETests(ctx context.Context) (*TestReport, error) {
	report := &TestReport{
		StartTime: time.Now(),
	}

	// Check SDK initialization
	if !ai_studio_sdk.IsInitialized() {
		return nil, fmt.Errorf("SDK not initialized - service API unavailable")
	}

	createdIDs := &CreatedResources{}

	// Run all test suites in sequence
	report.LLMTests, createdIDs.LLMs = RunLLMTests(ctx)
	report.AppTests, createdIDs.Apps = RunAppTests(ctx)
	report.ToolTests, createdIDs.Tools = RunToolTests(ctx)
	report.DatasourceTests, createdIDs.Datasources = RunDatasourceTests(ctx)
	report.TagTests, createdIDs.Tags = RunTagTests(ctx)
	report.FilterTests, createdIDs.Filters = RunFilterTests(ctx)
	report.ModelPriceTests, createdIDs.ModelPrices = RunModelPriceTests(ctx)
	report.DataCatalogueTests, createdIDs.DataCatalogues = RunDataCatalogueTests(ctx)
	report.KVTests, createdIDs.KVKeys = RunKVTests(ctx)

	// Always cleanup (even if tests failed)
	report.CleanupResults = CleanupTestArtifacts(ctx, createdIDs)

	// Calculate summary
	report.EndTime = time.Now()
	report.TotalDuration = report.EndTime.Sub(report.StartTime)
	report.TotalTests, report.PassedTests, report.FailedTests = calculateTestSummary(report)

	return report, nil
}

// calculateTestSummary counts total/passed/failed tests
func calculateTestSummary(report *TestReport) (int, int, int) {
	allTests := append(report.LLMTests, report.AppTests...)
	allTests = append(allTests, report.ToolTests...)
	allTests = append(allTests, report.DatasourceTests...)
	allTests = append(allTests, report.TagTests...)
	allTests = append(allTests, report.FilterTests...)
	allTests = append(allTests, report.ModelPriceTests...)
	allTests = append(allTests, report.DataCatalogueTests...)
	allTests = append(allTests, report.KVTests...)

	total := len(allTests)
	passed := 0
	failed := 0

	for _, test := range allTests {
		if test.Success {
			passed++
		} else {
			failed++
		}
	}

	return total, passed, failed
}

// CleanupTestArtifacts deletes all resources created during testing
// Returns results for each cleanup operation
func CleanupTestArtifacts(ctx context.Context, ids *CreatedResources) []TestResult {
	var results []TestResult

	// Delete in reverse dependency order to avoid foreign key constraints

	// 1. Delete Apps first (they reference LLMs/Tools)
	for _, appID := range ids.Apps {
		start := time.Now()
		err := ai_studio_sdk.DeleteApp(ctx, appID)
		results = append(results, TestResult{
			Operation: fmt.Sprintf("Cleanup: DeleteApp(%d)", appID),
			Success:   err == nil,
			Message:   errorOrSuccess(err, "App deleted"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})
	}

	// 2. Delete Tools
	for _, toolID := range ids.Tools {
		start := time.Now()
		err := ai_studio_sdk.DeleteTool(ctx, toolID)
		results = append(results, TestResult{
			Operation: fmt.Sprintf("Cleanup: DeleteTool(%d)", toolID),
			Success:   err == nil,
			Message:   errorOrSuccess(err, "Tool deleted"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})
	}

	// 3. Delete Datasources
	for _, dsID := range ids.Datasources {
		start := time.Now()
		err := ai_studio_sdk.DeleteDatasource(ctx, dsID)
		results = append(results, TestResult{
			Operation: fmt.Sprintf("Cleanup: DeleteDatasource(%d)", dsID),
			Success:   err == nil,
			Message:   errorOrSuccess(err, "Datasource deleted"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})
	}

	// 4. Delete Filters
	for _, filterID := range ids.Filters {
		start := time.Now()
		err := ai_studio_sdk.DeleteFilter(ctx, filterID)
		results = append(results, TestResult{
			Operation: fmt.Sprintf("Cleanup: DeleteFilter(%d)", filterID),
			Success:   err == nil,
			Message:   errorOrSuccess(err, "Filter deleted"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})
	}

	// 5. Delete LLMs
	for _, llmID := range ids.LLMs {
		start := time.Now()
		err := ai_studio_sdk.DeleteLLM(ctx, llmID)
		results = append(results, TestResult{
			Operation: fmt.Sprintf("Cleanup: DeleteLLM(%d)", llmID),
			Success:   err == nil,
			Message:   errorOrSuccess(err, "LLM deleted"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})
	}

	// 6. Delete Tags
	for _, tagID := range ids.Tags {
		start := time.Now()
		err := ai_studio_sdk.DeleteTag(ctx, tagID)
		results = append(results, TestResult{
			Operation: fmt.Sprintf("Cleanup: DeleteTag(%d)", tagID),
			Success:   err == nil,
			Message:   errorOrSuccess(err, "Tag deleted"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})
	}

	// 7. Delete Model Prices
	for _, priceID := range ids.ModelPrices {
		start := time.Now()
		err := ai_studio_sdk.DeleteModelPrice(ctx, priceID)
		results = append(results, TestResult{
			Operation: fmt.Sprintf("Cleanup: DeleteModelPrice(%d)", priceID),
			Success:   err == nil,
			Message:   errorOrSuccess(err, "Model Price deleted"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})
	}

	// 8. Delete Data Catalogues
	for _, catID := range ids.DataCatalogues {
		start := time.Now()
		err := ai_studio_sdk.DeleteDataCatalogue(ctx, catID)
		results = append(results, TestResult{
			Operation: fmt.Sprintf("Cleanup: DeleteDataCatalogue(%d)", catID),
			Success:   err == nil,
			Message:   errorOrSuccess(err, "Data Catalogue deleted"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})
	}

	// 9. Delete KV keys
	for _, key := range ids.KVKeys {
		start := time.Now()
		_, err := ai_studio_sdk.DeletePluginKV(ctx, key)
		results = append(results, TestResult{
			Operation: fmt.Sprintf("Cleanup: DeletePluginKV(%s)", key),
			Success:   err == nil,
			Message:   errorOrSuccess(err, "KV key deleted"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})
	}

	return results
}

// errorOrSuccess returns error message or success message
func errorOrSuccess(err error, successMsg string) string {
	if err != nil {
		return err.Error()
	}
	return successMsg
}

// toJSON converts test report to JSON bytes
func (r *TestReport) toJSON() ([]byte, error) {
	return json.Marshal(r)
}
