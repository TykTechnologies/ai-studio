package main

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
)

// RunLLMTests executes full CRUD lifecycle test for LLMs
// Returns: test results and created LLM IDs for cleanup
func RunLLMTests(ctx context.Context) ([]TestResult, []uint32) {
	var results []TestResult
	var createdIDs []uint32

	// Test 1: Create LLM
	result, llmID, createdName := testCreateLLM(ctx)
	results = append(results, result)
	if result.Success && llmID != 0 {
		createdIDs = append(createdIDs, llmID)
	}

	// Test 2: Get LLM (verify creation)
	if llmID != 0 {
		result = testGetLLM(ctx, llmID, createdName)
		results = append(results, result)
	}

	// Test 3: Update LLM
	var updatedName string
	if llmID != 0 {
		result, updatedName = testUpdateLLM(ctx, llmID)
		results = append(results, result)
	}

	// Test 4: Get LLM (verify update)
	if llmID != 0 {
		result = testGetLLM(ctx, llmID, updatedName)
		results = append(results, result)
	}

	// Test 5: Delete LLM
	if llmID != 0 {
		result = testDeleteLLM(ctx, llmID)
		results = append(results, result)
		if result.Success {
			// Remove from cleanup list if successfully deleted
			createdIDs = createdIDs[:len(createdIDs)-1]
		}
	}

	return results, createdIDs
}

func testCreateLLM(ctx context.Context) (TestResult, uint32, string) {
	start := time.Now()
	name := generateTestName("Test LLM")

	resp, err := ai_studio_sdk.CreateLLM(
		ctx,
		name,
		"test-api-key",
		"https://api.openai.com/v1",
		"openai",
		"gpt-4",
		5, // privacy score
		[]string{"gpt-4", "gpt-3.5-turbo"},
		nil, // no monthly budget
	)

	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "CreateLLM",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}, 0, ""
	}

	return TestResult{
		Operation: "CreateLLM",
		Success:   true,
		Message:   fmt.Sprintf("Created LLM ID %d: %s", resp.Llm.Id, resp.Llm.Name),
		Duration:  duration,
		Timestamp: time.Now(),
	}, resp.Llm.Id, resp.Llm.Name // Return actual name
}

func testGetLLM(ctx context.Context, llmID uint32, expectedName string) TestResult {
	start := time.Now()

	resp, err := ai_studio_sdk.GetLLM(ctx, llmID)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "GetLLM",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	// Verify the name matches expected
	if resp.Llm.Name != expectedName {
		return TestResult{
			Operation: "GetLLM",
			Success:   false,
			Message:   fmt.Sprintf("Name mismatch: expected '%s', got '%s'", expectedName, resp.Llm.Name),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	return TestResult{
		Operation: "GetLLM",
		Success:   true,
		Message:   fmt.Sprintf("Retrieved LLM ID %d: %s", resp.Llm.Id, resp.Llm.Name),
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

func testUpdateLLM(ctx context.Context, llmID uint32) (TestResult, string) {
	start := time.Now()
	updatedName := generateTestName("Test LLM Updated")

	resp, err := ai_studio_sdk.UpdateLLM(
		ctx,
		llmID,
		updatedName,
		"updated-api-key",
		"https://api.openai.com/v1",
		"gpt-4-turbo",
		7, // updated privacy score
		[]string{"gpt-4-turbo"},
		true,
		nil,
	)

	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "UpdateLLM",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}, ""
	}

	return TestResult{
		Operation: "UpdateLLM",
		Success:   true,
		Message:   fmt.Sprintf("Updated LLM ID %d: %s", resp.Llm.Id, resp.Llm.Name),
		Duration:  duration,
		Timestamp: time.Now(),
	}, resp.Llm.Name // Return actual updated name
}

func testDeleteLLM(ctx context.Context, llmID uint32) TestResult {
	start := time.Now()

	err := ai_studio_sdk.DeleteLLM(ctx, llmID)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "DeleteLLM",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	return TestResult{
		Operation: "DeleteLLM",
		Success:   true,
		Message:   fmt.Sprintf("Deleted LLM ID %d", llmID),
		Duration:  duration,
		Timestamp: time.Now(),
	}
}
