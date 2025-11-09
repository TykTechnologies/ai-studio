package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
)

// RunKVTests executes KV storage tests
func RunKVTests(ctx context.Context) ([]TestResult, []string) {
	var results []TestResult
	var createdKeys []string

	key := fmt.Sprintf("test-key-%d", time.Now().Unix())
	createdKeys = append(createdKeys, key)

	// Test 1: Write KV
	start := time.Now()
	testData := map[string]interface{}{
		"test": "data",
		"timestamp": time.Now().Unix(),
	}
	dataBytes, _ := json.Marshal(testData)

	created, err := ai_studio_sdk.WritePluginKV(ctx, key, dataBytes, nil) // No expiration for test
	results = append(results, TestResult{
		Operation: "WritePluginKV",
		Success:   err == nil && created,
		Message:   errorOrResult(err, fmt.Sprintf("Created key '%s'", key)),
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	})

	if err == nil {
		// Test 2: Read KV
		start = time.Now()
		value, err := ai_studio_sdk.ReadPluginKV(ctx, key)
		results = append(results, TestResult{
			Operation: "ReadPluginKV",
			Success:   err == nil && len(value) > 0,
			Message:   errorOrResult(err, fmt.Sprintf("Read key '%s' (%d bytes)", key, len(value))),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		// Test 3: Write KV again (update)
		start = time.Now()
		testData["updated"] = true
		dataBytes, _ = json.Marshal(testData)
		created, err = ai_studio_sdk.WritePluginKV(ctx, key, dataBytes, nil) // No expiration for test
		results = append(results, TestResult{
			Operation: "WritePluginKV (update)",
			Success:   err == nil && !created, // Should be update, not create
			Message:   errorOrResult(err, fmt.Sprintf("Updated key '%s'", key)),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		// Test 4: Delete KV
		start = time.Now()
		deleted, err := ai_studio_sdk.DeletePluginKV(ctx, key)
		results = append(results, TestResult{
			Operation: "DeletePluginKV",
			Success:   err == nil && deleted,
			Message:   errorOrResult(err, fmt.Sprintf("Deleted key '%s'", key)),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		if err == nil && deleted {
			createdKeys = createdKeys[:0] // Successfully deleted
		}
	}

	return results, createdKeys
}
