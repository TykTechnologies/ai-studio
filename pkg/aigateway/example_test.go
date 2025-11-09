package aigateway_test

import (
	"fmt"
)

// UsageExample_New demonstrates how to use the AI Gateway library
// with existing Midsommar services (database-backed configuration)
// This is kept for documentation purposes but not run as a test.
func UsageExample_New() {
	// Assume you have an existing database and services set up
	// service := services.NewService(db)
	// budgetService := services.NewBudgetService(db, service)

	// Create a gateway instance
	// gateway := aigateway.New(service, &proxy.Config{Port: 9090}, budgetService)

	// Option 1: Start as standalone server
	// if err := gateway.Start(); err != nil {
	//     log.Fatal(err)
	// }

	// Option 2: Use as HTTP handler in existing server
	// http.Handle("/ai/", http.StripPrefix("/ai", gateway.Handler()))
	// log.Fatal(http.ListenAndServe(":8080", nil))

	fmt.Println("AI Gateway can be used as a standalone server or integrated into existing HTTP servers")
	// Output: AI Gateway can be used as a standalone server or integrated into existing HTTP servers
}

// UsageExample_Handler demonstrates using the gateway as an HTTP handler
// This is kept for documentation purposes but not run as a test.
func UsageExample_Handler() {
	// This example shows how to integrate the AI Gateway into an existing HTTP server

	// Assume services are initialized
	// service := services.NewService(db)
	// budgetService := services.NewBudgetService(db, service)
	// gateway := aigateway.New(service, &proxy.Config{Port: 9090}, budgetService)

	// Create a new HTTP server with the gateway mounted at a path
	// mux := http.NewServeMux()
	// mux.Handle("/api/ai/", http.StripPrefix("/api/ai", gateway.Handler()))
	// mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
	//     w.WriteHeader(http.StatusOK)
	//     w.Write([]byte("OK"))
	// })

	// server := &http.Server{
	//     Addr:    ":8080",
	//     Handler: mux,
	// }

	// log.Fatal(server.ListenAndServe())

	fmt.Println("Gateway can be mounted at any path in an existing HTTP server")
	// Output: Gateway can be mounted at any path in an existing HTTP server
}

// UsageExample_Reload demonstrates hot reloading of configuration
// This is kept for documentation purposes but not run as a test.
func UsageExample_Reload() {
	// service := services.NewService(db)
	// budgetService := services.NewBudgetService(db, service)
	// gateway := aigateway.New(service, &proxy.Config{Port: 9090}, budgetService)

	// Start the gateway in a goroutine
	// go func() {
	//     if err := gateway.Start(); err != nil {
	//         log.Printf("Gateway stopped: %v", err)
	//     }
	// }()

	// Wait a moment for it to start
	// time.Sleep(100 * time.Millisecond)

	// Reload configuration (useful after database changes)
	// if err := gateway.Reload(); err != nil {
	//     log.Printf("Failed to reload: %v", err)
	// }

	// Gracefully stop the gateway
	// ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// defer cancel()
	// gateway.Stop(ctx)

	fmt.Println("Gateway supports hot reloading of LLM configurations")
	// Output: Gateway supports hot reloading of LLM configurations
}

// Note: Additional implementation examples can be found in:
// - pkg/aigateway/README.md (comprehensive documentation)
// - examples/standalone-gateway/main.go (working example)
// - features/AIGatewayLibrary.md (feature specification)
