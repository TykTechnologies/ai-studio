package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TykTechnologies/midsommar/v2/examples/file-based-demo/services"
)

// testFilters demonstrates the filter functionality in the file-based demo
func testFilters() {
	fmt.Println("=== Filter Demo ===")

	// Create the file-based gateway service
	gatewayService, err := services.NewFileGatewayService("../config")
	if err != nil {
		log.Fatalf("Failed to create gateway service: %v", err)
	}

	// Test 1: List all filters
	fmt.Println("\n1. Loading available filters:")
	filters, totalCount, totalPages, err := gatewayService.GetAllFilters(10, 1, true)
	if err != nil {
		log.Fatalf("Failed to get filters: %v", err)
	}

	fmt.Printf("Found %d filters across %d pages:\n", totalCount, totalPages)
	for _, filter := range filters {
		fmt.Printf("  - ID: %d, Name: %s\n", filter.ID, filter.Name)
		fmt.Printf("    Description: %s\n", filter.Description)
		fmt.Printf("    Script length: %d bytes\n", len(filter.Script))
	}

	// Test 2: Get specific filter
	fmt.Println("\n2. Getting specific filter details:")
	if len(filters) > 0 {
		filter, err := gatewayService.GetFilterByID(filters[0].ID)
		if err != nil {
			log.Fatalf("Failed to get filter by ID: %v", err)
		}
		fmt.Printf("Filter '%s' script preview:\n%s\n", filter.Name, string(filter.Script)[:min(200, len(filter.Script))])
	}

	// Test 3: Check LLM-Filter associations
	fmt.Println("\n3. Checking LLM-Filter associations:")
	llms, err := gatewayService.GetActiveLLMs(context.Background())
	if err != nil {
		log.Fatalf("Failed to get LLMs: %v", err)
	}

	for _, llm := range llms {
		fmt.Printf("LLM '%s' has %d filters:\n", llm.Name, len(llm.Filters))
		for _, filter := range llm.Filters {
			fmt.Printf("  - %s (ID: %d)\n", filter.Name, filter.ID)
		}
	}

	fmt.Println("\n=== Filter Demo Complete ===")
	fmt.Println("Filters are now loaded and associated with LLMs!")
	fmt.Println("When using the AI Gateway library, these filters will be automatically")
	fmt.Println("executed on outbound requests to filter-enabled LLMs.")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	testFilters()
}
