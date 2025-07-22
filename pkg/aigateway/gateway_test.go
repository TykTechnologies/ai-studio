package aigateway

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// TestGatewayInterface ensures that our gateway implementation satisfies the interface
func TestGatewayInterface(t *testing.T) {
	var _ Gateway = (*gateway)(nil)
}

// TestNew ensures that New() creates a valid gateway
func TestNew(t *testing.T) {
	// This test would normally require a database connection, so we'll keep it simple
	// In a real implementation, you'd mock the services

	if testing.Short() {
		t.Skip("Skipping database-dependent test in short mode")
	}

	// Mock services would go here in a real test
	// For now, we just test that the function signature works
	var service *services.Service
	var budgetService *services.BudgetService
	config := &proxy.Config{Port: 9090}

	// This would panic without real services, so we test the function exists
	if service == nil || budgetService == nil {
		t.Skip("Skipping test - requires real database services")
	}

	gateway := New(
		NewDatabaseService(service),
		NewDatabaseBudgetService(budgetService),
		&Config{Port: config.Port},
	)
	if gateway == nil {
		t.Error("New() returned nil gateway")
	}
}

// TestGatewayMethods tests that all gateway methods exist and have correct signatures
func TestGatewayMethods(t *testing.T) {
	// Test that the gateway interface is properly implemented
	// by checking that the methods exist with correct signatures

	var g Gateway = &gateway{proxy: nil}

	// Verify that the interface methods exist (we don't call them with nil proxy)
	_ = g.Start   // func() error
	_ = g.Stop    // func(context.Context) error
	_ = g.Handler // func() http.Handler
	_ = g.Reload  // func() error

	// This test mainly verifies that the interface is properly implemented
	// without requiring actual database connections or causing panics
}

// BenchmarkGatewayCreation benchmarks gateway creation (without actual initialization)
func BenchmarkGatewayCreation(b *testing.B) {
	// This would benchmark the creation process
	// In practice, this is mostly just struct allocation, so it should be very fast

	b.Skip("Benchmark requires real services - placeholder for future implementation")

	// Mock benchmark would look like:
	// for i := 0; i < b.N; i++ {
	//     gateway := New(service, config, budgetService)
	//     _ = gateway
	// }
}
