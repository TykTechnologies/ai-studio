package main

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
)

// RunFilterTests executes full CRUD lifecycle test for Filters
func RunFilterTests(ctx context.Context) ([]TestResult, []uint32) {
	var results []TestResult
	var createdIDs []uint32

	// Create Filter
	start := time.Now()
	name := generateTestName("Test Filter")
	script := "function filter(req) { return req; }" // Simple pass-through filter
	createResp, err := ai_studio_sdk.CreateFilter(ctx, name, "Test filter for E2E", script)
	results = append(results, TestResult{
		Operation: "CreateFilter",
		Success:   err == nil,
		Message:   errorOrResult(err, fmt.Sprintf("Created Filter ID %d", createResp.GetFilter().GetId())),
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	})

	var filterID uint32
	var createdName string
	if err == nil {
		filterID = createResp.Filter.Id
		createdName = createResp.Filter.Name
		createdIDs = append(createdIDs, filterID)

		// Get Filter
		start = time.Now()
		getResp, err := ai_studio_sdk.GetFilter(ctx, filterID)
		results = append(results, TestResult{
			Operation: "GetFilter",
			Success:   err == nil && getResp.Filter.Name == createdName,
			Message:   errorOrResult(err, fmt.Sprintf("Retrieved: %s", getResp.Filter.Name)),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		// Update Filter
		start = time.Now()
		updatedName := generateTestName("Test Filter Updated")
		updatedScript := "function filter(req) { req.modified = true; return req; }"
		updateResp, err := ai_studio_sdk.UpdateFilter(ctx, filterID, updatedName, "Updated description", updatedScript)
		results = append(results, TestResult{
			Operation: "UpdateFilter",
			Success:   err == nil,
			Message:   errorOrResult(err, "Filter updated"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		if err == nil {
			updatedName = updateResp.Filter.Name
		}

		// Get Filter (verify update)
		start = time.Now()
		getResp, err = ai_studio_sdk.GetFilter(ctx, filterID)
		results = append(results, TestResult{
			Operation: "GetFilter (updated)",
			Success:   err == nil && getResp.Filter.Name == updatedName,
			Message:   errorOrResult(err, fmt.Sprintf("Retrieved updated: %s", getResp.Filter.Name)),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		// Delete Filter
		start = time.Now()
		err = ai_studio_sdk.DeleteFilter(ctx, filterID)
		results = append(results, TestResult{
			Operation: "DeleteFilter",
			Success:   err == nil,
			Message:   errorOrResult(err, "Filter deleted"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		if err == nil {
			createdIDs = createdIDs[:0] // Successfully deleted
		}
	}

	return results, createdIDs
}
