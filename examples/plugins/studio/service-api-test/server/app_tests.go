package main

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
)

// RunAppTests executes full CRUD lifecycle test for Apps
func RunAppTests(ctx context.Context) ([]TestResult, []uint32) {
	var results []TestResult
	var createdIDs []uint32

	// Create App
	start := time.Now()
	name := generateTestName("Test App")
	createResp, err := ai_studio_sdk.CreateApp(ctx, name, "E2E test application", 1, []uint32{}, []uint32{}, nil)
	results = append(results, TestResult{
		Operation: "CreateApp",
		Success:   err == nil,
		Message:   errorOrResult(err, fmt.Sprintf("Created App ID %d", createResp.GetApp().GetId())),
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	})

	var appID uint32
	var createdName string
	if err == nil {
		appID = createResp.App.Id
		createdName = createResp.App.Name
		createdIDs = append(createdIDs, appID)

		// Get App
		start = time.Now()
		getResp, err := ai_studio_sdk.GetApp(ctx, appID)
		results = append(results, TestResult{
			Operation: "GetApp",
			Success:   err == nil && getResp.App.Name == createdName,
			Message:   errorOrResult(err, fmt.Sprintf("Retrieved: %s", getResp.App.Name)),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		// Update App
		start = time.Now()
		updatedName := generateTestName("Test App Updated")
		updateResp, err := ai_studio_sdk.UpdateApp(ctx, appID, updatedName, "Updated description", true, []uint32{}, []uint32{}, nil)
		results = append(results, TestResult{
			Operation: "UpdateApp",
			Success:   err == nil,
			Message:   errorOrResult(err, "App updated"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		if err == nil {
			updatedName = updateResp.App.Name
		}

		// Get App (verify update)
		start = time.Now()
		getResp, err = ai_studio_sdk.GetApp(ctx, appID)
		results = append(results, TestResult{
			Operation: "GetApp (updated)",
			Success:   err == nil && getResp.App.Name == updatedName,
			Message:   errorOrResult(err, fmt.Sprintf("Retrieved updated: %s", getResp.App.Name)),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		// Delete App
		start = time.Now()
		err = ai_studio_sdk.DeleteApp(ctx, appID)
		results = append(results, TestResult{
			Operation: "DeleteApp",
			Success:   err == nil,
			Message:   errorOrResult(err, "App deleted"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		if err == nil {
			createdIDs = createdIDs[:0] // Successfully deleted
		}
	}

	return results, createdIDs
}
