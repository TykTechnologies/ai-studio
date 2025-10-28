package main

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
)

// RunModelPriceTests executes full CRUD lifecycle test for Model Prices
func RunModelPriceTests(ctx context.Context) ([]TestResult, []uint32) {
	var results []TestResult
	var createdIDs []uint32

	// Create Model Price
	start := time.Now()
	modelName := generateTestName("test-model")
	createResp, err := ai_studio_sdk.CreateModelPrice(ctx, modelName, "test-vendor", "USD", 0.001, 0.0005, 0.0002, 0.0001)
	results = append(results, TestResult{
		Operation: "CreateModelPrice",
		Success:   err == nil,
		Message:   errorOrResult(err, fmt.Sprintf("Created Model Price ID %d", createResp.GetModelPrice().GetId())),
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	})

	var priceID uint32
	var createdModelName string
	if err == nil {
		priceID = createResp.ModelPrice.Id
		createdModelName = createResp.ModelPrice.ModelName
		createdIDs = append(createdIDs, priceID)

		// Get Model Price
		start = time.Now()
		getResp, err := ai_studio_sdk.GetModelPrice(ctx, priceID)
		results = append(results, TestResult{
			Operation: "GetModelPrice",
			Success:   err == nil && getResp.ModelPrice.ModelName == createdModelName,
			Message:   errorOrResult(err, fmt.Sprintf("Retrieved: %s", getResp.ModelPrice.ModelName)),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		// Update Model Price
		start = time.Now()
		updatedModelName := generateTestName("test-model-updated")
		updateResp, err := ai_studio_sdk.UpdateModelPrice(ctx, priceID, updatedModelName, "test-vendor", "USD", 0.002, 0.001, 0.0003, 0.00015)
		results = append(results, TestResult{
			Operation: "UpdateModelPrice",
			Success:   err == nil,
			Message:   errorOrResult(err, "Model Price updated"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		if err == nil {
			updatedModelName = updateResp.ModelPrice.ModelName
		}

		// Get Model Price (verify update)
		start = time.Now()
		getResp, err = ai_studio_sdk.GetModelPrice(ctx, priceID)
		results = append(results, TestResult{
			Operation: "GetModelPrice (updated)",
			Success:   err == nil && getResp.ModelPrice.ModelName == updatedModelName,
			Message:   errorOrResult(err, fmt.Sprintf("Retrieved updated: %s", getResp.ModelPrice.ModelName)),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		// Delete Model Price
		start = time.Now()
		err = ai_studio_sdk.DeleteModelPrice(ctx, priceID)
		results = append(results, TestResult{
			Operation: "DeleteModelPrice",
			Success:   err == nil,
			Message:   errorOrResult(err, "Model Price deleted"),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		})

		if err == nil {
			createdIDs = createdIDs[:0] // Successfully deleted
		}
	}

	return results, createdIDs
}
