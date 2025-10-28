package main

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
)

// RunTagTests executes full CRUD lifecycle test for Tags
func RunTagTests(ctx context.Context) ([]TestResult, []uint32) {
	var results []TestResult
	var createdIDs []uint32

	// Create Tag
	start := time.Now()
	name := generateTestName("Test Tag")
	createResp, err := ai_studio_sdk.CreateTag(ctx, name)
	results = append(results, TestResult{
		Operation: "CreateTag",
		Success:   err == nil,
		Message:   errorOrResult(err, fmt.Sprintf("Created Tag ID %d", createResp.GetTag().GetId())),
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	})

	var tagID uint32
	if err == nil {
		tagID = createResp.Tag.Id
		createdIDs = append(createdIDs, tagID)

		// Get Tag
		start = time.Now()
		getResp, err := ai_studio_sdk.GetTag(ctx, tagID)
		results = append(results, TestResult{
			Operation: "GetTag",
			Success:   err == nil && getResp.Tag.Name == name,
			Message:   errorOrResult(err, fmt.Sprintf("Retrieved: %s", getResp.Tag.Name)),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		// Update Tag
		start = time.Now()
		updatedName := generateTestName("Test Tag Updated")
		_, err = ai_studio_sdk.UpdateTag(ctx, tagID, updatedName)
		results = append(results, TestResult{
			Operation: "UpdateTag",
			Success:   err == nil,
			Message:   errorOrResult(err, "Tag updated"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		// Get Tag (verify update)
		start = time.Now()
		getResp, err = ai_studio_sdk.GetTag(ctx, tagID)
		results = append(results, TestResult{
			Operation: "GetTag (updated)",
			Success:   err == nil && getResp.Tag.Name == updatedName,
			Message:   errorOrResult(err, fmt.Sprintf("Retrieved updated: %s", getResp.Tag.Name)),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		// Delete Tag
		start = time.Now()
		err = ai_studio_sdk.DeleteTag(ctx, tagID)
		results = append(results, TestResult{
			Operation: "DeleteTag",
			Success:   err == nil,
			Message:   errorOrResult(err, "Tag deleted"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		if err == nil {
			createdIDs = createdIDs[:0] // Clear - successfully deleted
		}
	}

	return results, createdIDs
}

func errorOrResult(err error, successMsg string) string {
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return successMsg
}
