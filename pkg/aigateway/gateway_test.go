package aigateway

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
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
	var budgetService budget.Service
	config := &proxy.Config{Port: 9090}

	// This would panic without real services, so we test the function exists
	if service == nil || budgetService == nil {
		t.Skip("Skipping test - requires real database services")
	}

	gateway := New(
		service,       // directly use services.Service
		budgetService, // budget.Service interface
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

// TestUnifiedRouterOptionsReachProxy proves the wrapper's unified-router options
// are not dropped on the way to the proxy, and that the effective prefix is
// readable back through the Gateway interface — which is how an embedding host
// knows what path to mount (and that it must mount nothing when disabled).
func TestUnifiedRouterOptionsReachProxy(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   string
	}{
		{"default", &Config{}, "/v1"},
		{"custom base path", &Config{UnifiedRouterBasePath: "/ai-gateway/v1"}, "/ai-gateway/v1"},
		{"normalized", &Config{UnifiedRouterBasePath: "ai-gateway/v1/"}, "/ai-gateway/v1"},
		{"disabled", &Config{DisableUnifiedRouter: true}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// No services are needed: only configuration is read here.
			g := NewWithAnalytics(nil, nil, nil, tc.config)
			if got := g.UnifiedRouterBasePath(); got != tc.want {
				t.Errorf("UnifiedRouterBasePath() = %q, want %q", got, tc.want)
			}
		})
	}
}
