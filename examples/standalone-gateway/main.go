// Package main provides a simple example of using the AI Gateway library
// to create a standalone microproxy service.
package main

import (
	"log"
)

func main() {
	log.Println("AI Gateway Standalone Example")
	log.Println("This example shows how to use the AI Gateway library")
	log.Println("to create a standalone microproxy service.")

	// In a real application, you would:
	// 1. Set up your database connection
	// 2. Initialize the services with the database
	// 3. Configure LLMs, apps, and budgets via the database
	//
	// Example:
	//   db := setupDatabase()
	//   service := services.NewService(db)
	//   budgetService := services.NewBudgetService(db, service)

	// For this example, we'll demonstrate the API without real database connections
	demonstrateAPI()
}

func demonstrateAPI() {
	log.Println("\n=== AI Gateway Library API Demo ===")

	// This function demonstrates the AI Gateway API without requiring
	// a database connection. In real usage, you'd provide actual services.

	log.Println("1. Creating gateway with services (simulated)")
	log.Println("   gateway := aigateway.New(service, &proxy.Config{Port: 9090}, budgetService)")

	log.Println("\n2. Starting as standalone server:")
	log.Println("   gateway.Start() // Blocks and serves HTTP on :9090")

	log.Println("\n3. Using as HTTP handler:")
	log.Println("   http.Handle(\"/ai/\", gateway.Handler())")

	log.Println("\n4. Graceful shutdown:")
	log.Println("   ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)")
	log.Println("   gateway.Stop(ctx)")

	log.Println("\n5. Configuration hot reloading:")
	log.Println("   gateway.Reload() // Reloads LLMs, datasources, filters from database")

	log.Println("\nOnce running, the gateway provides these endpoints:")
	log.Println("  POST /llm/rest/{llmSlug}/{path}     - LLM API calls")
	log.Println("  POST /llm/stream/{llmSlug}/{path}   - Streaming LLM calls")
	log.Println("  POST /tools/{toolSlug}              - Tool operations")
	log.Println("  POST /tools/{toolSlug}/mcp          - MCP protocol")
	log.Println("  POST /datasource/{dsSlug}           - Vector search")

	log.Println("\n=== End Demo ===")
}

// realWorldExample shows what a real implementation would look like
// (commented out since it requires actual database setup)
func realWorldExample() {
	// This is what a real implementation would look like:

	/*
		// Database setup
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			log.Fatal("DATABASE_URL environment variable required")
		}

		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}

		// Initialize analytics first
		ctx := context.Background()
		analytics.InitDefault(ctx, db)

		// Services setup
		service := services.NewService(db)
		budgetService := services.NewBudgetService(db, service)

		// Gateway setup with default database analytics
		port := 9090
		if portStr := os.Getenv("PORT"); portStr != "" {
			if p, err := strconv.Atoi(portStr); err == nil {
				port = p
			}
		}

		gateway := aigateway.New(
			aigateway.NewDatabaseService(service),
			aigateway.NewDatabaseBudgetService(budgetService),
			&aigateway.Config{Port: port},
		)

		// Alternative: Gateway with custom HTTP analytics
		// httpAnalytics := aigateway.NewHTTPAnalyticsHandler("https://my-control-plane/api")
		// gateway := aigateway.NewWithAnalytics(
		// 	aigateway.NewDatabaseService(service),
		// 	aigateway.NewDatabaseBudgetService(budgetService),
		// 	httpAnalytics,
		// 	&aigateway.Config{Port: port},
		// )

		// Graceful shutdown setup
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		// Start server
		go func() {
			log.Printf("Starting AI Gateway microproxy on :%d", port)
			if err := gateway.Start(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Gateway error: %v", err)
			}
		}()

		// Wait for shutdown
		<-ctx.Done()
		log.Println("Received shutdown signal...")

		// Graceful shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := gateway.Stop(shutdownCtx); err != nil {
			log.Printf("Gateway shutdown error: %v", err)
			os.Exit(1)
		}

		log.Println("Gateway stopped gracefully")
	*/
}

// integrationExample shows how to integrate with existing HTTP servers
func integrationExample() {
	// This is what integration with existing servers would look like:

	/*
		// Initialize services
		service := services.NewService(db)
		budgetService := services.NewBudgetService(db, service)
		gateway := aigateway.New(service, &proxy.Config{}, budgetService)

		// Integration with standard library
		mux := http.NewServeMux()
		mux.Handle("/ai/", http.StripPrefix("/ai", gateway.Handler()))
		mux.HandleFunc("/health", healthHandler)

		// Integration with Gin
		ginRouter := gin.Default()
		ginRouter.Any("/ai/*path", gin.WrapH(gateway.Handler()))

		// Integration with Gorilla Mux
		gorillaRouter := mux.NewRouter()
		gorillaRouter.PathPrefix("/ai/").Handler(gateway.Handler())

		// Integration with Chi
		chiRouter := chi.NewRouter()
		chiRouter.Mount("/ai", gateway.Handler())
	*/
}
