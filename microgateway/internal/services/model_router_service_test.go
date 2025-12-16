package services

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ModelRouterService Unit Tests
// ============================================================================
// SPECIFICATION: These tests validate the runtime routing logic for the
// model router feature. The service is responsible for:
// 1. Pattern matching models to pools
// 2. Vendor selection (round-robin and weighted)
// 3. Model name mapping at the vendor level

// ============================================================================
// Test Helpers
// ============================================================================

// createTestRouter creates a compiled router for testing
func createTestRouter(slug string, pools []*CompiledPool) *CompiledRouter {
	dbRouter := &database.ModelRouter{
		ID:       1,
		Name:     "Test Router",
		Slug:     slug,
		IsActive: true,
	}
	return &CompiledRouter{
		Router:        dbRouter,
		CompiledPools: pools,
	}
}

// createTestPool creates a compiled pool for testing
func createTestPool(name string, pattern string, algorithm string, vendors []database.PoolVendor) *CompiledPool {
	pool := &database.ModelPool{
		ID:                 1,
		Name:               name,
		ModelPattern:       pattern,
		SelectionAlgorithm: algorithm,
		Priority:           10,
		Vendors:            vendors,
	}
	return &CompiledPool{
		Pool:    pool,
		Pattern: pattern,
		Counter: 0,
	}
}

// createTestVendor creates a test vendor
func createTestVendor(llmSlug string, weight int, active bool, mappings []database.ModelMapping) database.PoolVendor {
	return database.PoolVendor{
		ID:       1,
		LLMSlug:  llmSlug,
		Weight:   weight,
		IsActive: active,
		Mappings: mappings,
	}
}

// ============================================================================
// Pattern Matching Tests
// ============================================================================

func TestSelectVendor_PatternMatching_WildcardAll(t *testing.T) {
	svc := NewModelRouterService(nil)

	// Setup: Router with wildcard pool
	vendors := []database.PoolVendor{createTestVendor("openai", 1, true, nil)}
	pool := createTestPool("Wildcard", "*", "round_robin", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	// SPECIFICATION: Wildcard pattern "*" MUST match any model name
	testModels := []string{"gpt-4", "claude-3-opus", "llama-70b", "anything", "model-with-dashes"}
	for _, model := range testModels {
		selection, err := svc.SelectVendor("test-router", model)
		require.NoError(t, err, "Wildcard MUST match model: %s", model)
		assert.Equal(t, "openai", selection.Vendor.LLMSlug)
		assert.Equal(t, model, selection.TargetModel, "Target model MUST equal source when no mapping")
	}
}

func TestSelectVendor_PatternMatching_PrefixWildcard(t *testing.T) {
	svc := NewModelRouterService(nil)

	vendors := []database.PoolVendor{createTestVendor("anthropic", 1, true, nil)}
	pool := createTestPool("Claude Models", "claude-*", "round_robin", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	tests := []struct {
		model   string
		matches bool
	}{
		{"claude-3-opus", true},
		{"claude-3-sonnet", true},
		{"claude-instant", true},
		{"gpt-4", false},       // MUST NOT match
		{"claudes-model", false}, // MUST NOT match (no dash)
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			selection, err := svc.SelectVendor("test-router", tt.model)
			if tt.matches {
				// SPECIFICATION: Prefix wildcard "name-*" MUST match models starting with "name-"
				require.NoError(t, err, "Pattern claude-* MUST match: %s", tt.model)
				assert.Equal(t, "anthropic", selection.Vendor.LLMSlug)
			} else {
				// SPECIFICATION: Prefix wildcard MUST NOT match models not starting with prefix
				assert.ErrorIs(t, err, ErrNoMatchingPool, "Pattern claude-* MUST NOT match: %s", tt.model)
			}
		})
	}
}

func TestSelectVendor_PatternMatching_SuffixWildcard(t *testing.T) {
	svc := NewModelRouterService(nil)

	vendors := []database.PoolVendor{createTestVendor("openai", 1, true, nil)}
	pool := createTestPool("Turbo Models", "*-turbo", "round_robin", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	tests := []struct {
		model   string
		matches bool
	}{
		{"gpt-4-turbo", true},
		{"gpt-3.5-turbo", true},
		{"claude-turbo", true},
		{"turbo", false},      // No prefix
		{"gpt-4", false},      // No suffix
		{"gpt-4-turbo-preview", false}, // Extra suffix
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			selection, err := svc.SelectVendor("test-router", tt.model)
			if tt.matches {
				require.NoError(t, err, "Pattern *-turbo MUST match: %s", tt.model)
				assert.Equal(t, "openai", selection.Vendor.LLMSlug)
			} else {
				assert.ErrorIs(t, err, ErrNoMatchingPool, "Pattern *-turbo MUST NOT match: %s", tt.model)
			}
		})
	}
}

func TestSelectVendor_PatternMatching_ExactMatch(t *testing.T) {
	svc := NewModelRouterService(nil)

	vendors := []database.PoolVendor{createTestVendor("openai", 1, true, nil)}
	pool := createTestPool("GPT-4 Only", "gpt-4", "round_robin", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	// SPECIFICATION: Exact pattern MUST only match exact model name
	selection, err := svc.SelectVendor("test-router", "gpt-4")
	require.NoError(t, err)
	assert.Equal(t, "openai", selection.Vendor.LLMSlug)

	// MUST NOT match variants
	_, err = svc.SelectVendor("test-router", "gpt-4-turbo")
	assert.ErrorIs(t, err, ErrNoMatchingPool)

	_, err = svc.SelectVendor("test-router", "gpt-4o")
	assert.ErrorIs(t, err, ErrNoMatchingPool)
}

func TestSelectVendor_PatternMatching_PoolPriority(t *testing.T) {
	svc := NewModelRouterService(nil)

	// Two pools: specific (high priority) and wildcard (low priority)
	specificVendors := []database.PoolVendor{createTestVendor("anthropic", 1, true, nil)}
	specificPool := createTestPool("Claude Specific", "claude-*", "round_robin", specificVendors)
	specificPool.Pool.Priority = 100 // Higher priority

	wildcardVendors := []database.PoolVendor{createTestVendor("openai", 1, true, nil)}
	wildcardPool := createTestPool("Fallback", "*", "round_robin", wildcardVendors)
	wildcardPool.Pool.Priority = 10 // Lower priority

	// Put wildcard first in slice to test that priority sorting works
	router := createTestRouter("test-router", []*CompiledPool{wildcardPool, specificPool})

	// Manually sort by priority DESC (as LoadRouters does)
	router.CompiledPools = []*CompiledPool{specificPool, wildcardPool}

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	// SPECIFICATION: Higher priority pools MUST be checked first
	selection, err := svc.SelectVendor("test-router", "claude-3-opus")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", selection.Vendor.LLMSlug,
		"claude-3-opus MUST match high-priority claude-* pool, not wildcard")

	// Non-claude model should fall through to wildcard
	selection, err = svc.SelectVendor("test-router", "gpt-4")
	require.NoError(t, err)
	assert.Equal(t, "openai", selection.Vendor.LLMSlug,
		"gpt-4 MUST match fallback wildcard pool")
}

func TestSelectVendor_PatternMatching_CommaSeparated(t *testing.T) {
	svc := NewModelRouterService(nil)

	// Setup: Router with comma-separated pattern pool
	vendors := []database.PoolVendor{createTestVendor("multi-vendor", 1, true, nil)}
	pool := createTestPool("Multi Model", "gpt-*,claude-*", "round_robin", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	// SPECIFICATION: Comma-separated patterns MUST match any of the individual patterns
	tests := []struct {
		model   string
		matches bool
		desc    string
	}{
		{"gpt-4", true, "gpt-* pattern should match gpt-4"},
		{"gpt-4-turbo", true, "gpt-* pattern should match gpt-4-turbo"},
		{"gpt-3.5-turbo", true, "gpt-* pattern should match gpt-3.5-turbo"},
		{"claude-3-opus", true, "claude-* pattern should match claude-3-opus"},
		{"claude-sonnet-4-5", true, "claude-* pattern should match claude-sonnet-4-5"},
		{"claude-instant", true, "claude-* pattern should match claude-instant"},
		{"llama-70b", false, "llama model should NOT match gpt-* or claude-*"},
		{"mistral-7b", false, "mistral model should NOT match gpt-* or claude-*"},
		{"o1-preview", false, "o1 model should NOT match gpt-* or claude-*"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			selection, err := svc.SelectVendor("test-router", tt.model)
			if tt.matches {
				require.NoError(t, err, tt.desc)
				assert.Equal(t, "multi-vendor", selection.Vendor.LLMSlug)
			} else {
				assert.ErrorIs(t, err, ErrNoMatchingPool, tt.desc)
			}
		})
	}
}

func TestSelectVendor_PatternMatching_CommaSeparatedWithSpaces(t *testing.T) {
	svc := NewModelRouterService(nil)

	// Setup: Router with comma-separated pattern with spaces (edge case)
	vendors := []database.PoolVendor{createTestVendor("multi-vendor", 1, true, nil)}
	pool := createTestPool("Multi Model Spaces", "gpt-* , claude-* , llama-*", "round_robin", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	// SPECIFICATION: Spaces around patterns in comma-separated list MUST be trimmed
	tests := []struct {
		model   string
		matches bool
	}{
		{"gpt-4", true},
		{"claude-3-opus", true},
		{"llama-70b", true},
		{"mistral-7b", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			selection, err := svc.SelectVendor("test-router", tt.model)
			if tt.matches {
				require.NoError(t, err, "Pattern with spaces should match: %s", tt.model)
				assert.Equal(t, "multi-vendor", selection.Vendor.LLMSlug)
			} else {
				assert.ErrorIs(t, err, ErrNoMatchingPool)
			}
		})
	}
}

// ============================================================================
// Round-Robin Selection Tests
// ============================================================================

func TestSelectVendor_RoundRobin_Distribution(t *testing.T) {
	svc := NewModelRouterService(nil)

	vendors := []database.PoolVendor{
		createTestVendor("vendor-a", 1, true, nil),
		createTestVendor("vendor-b", 1, true, nil),
		createTestVendor("vendor-c", 1, true, nil),
	}
	vendors[0].ID = 1
	vendors[1].ID = 2
	vendors[2].ID = 3

	pool := createTestPool("Multi Vendor", "*", "round_robin", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	// SPECIFICATION: Round-robin MUST cycle through vendors in order
	// Call 9 times to verify 3 complete cycles
	expectedOrder := []string{"vendor-a", "vendor-b", "vendor-c"}
	for cycle := 0; cycle < 3; cycle++ {
		for i, expected := range expectedOrder {
			selection, err := svc.SelectVendor("test-router", "gpt-4")
			require.NoError(t, err, "Selection %d in cycle %d", i, cycle)
			assert.Equal(t, expected, selection.Vendor.LLMSlug,
				"Round-robin selection %d in cycle %d MUST be %s", i, cycle, expected)
		}
	}
}

func TestSelectVendor_RoundRobin_SkipsInactiveVendors(t *testing.T) {
	svc := NewModelRouterService(nil)

	vendors := []database.PoolVendor{
		createTestVendor("vendor-active-1", 1, true, nil),
		createTestVendor("vendor-inactive", 1, false, nil), // INACTIVE
		createTestVendor("vendor-active-2", 1, true, nil),
	}

	pool := createTestPool("Mixed Active", "*", "round_robin", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	// SPECIFICATION: Round-robin MUST only select from active vendors
	selections := make(map[string]int)
	for i := 0; i < 10; i++ {
		selection, err := svc.SelectVendor("test-router", "gpt-4")
		require.NoError(t, err)
		selections[selection.Vendor.LLMSlug]++
	}

	assert.Zero(t, selections["vendor-inactive"],
		"Inactive vendor MUST never be selected")
	assert.Greater(t, selections["vendor-active-1"], 0,
		"Active vendor 1 MUST be selected")
	assert.Greater(t, selections["vendor-active-2"], 0,
		"Active vendor 2 MUST be selected")
}

// ============================================================================
// Weighted Selection Tests
// ============================================================================

func TestSelectVendor_Weighted_RespectWeights(t *testing.T) {
	svc := NewModelRouterService(nil)

	// Vendor A has weight 90, Vendor B has weight 10
	// Over many selections, A should get ~90% and B ~10%
	vendors := []database.PoolVendor{
		createTestVendor("vendor-heavy", 90, true, nil),
		createTestVendor("vendor-light", 10, true, nil),
	}

	pool := createTestPool("Weighted", "*", "weighted", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	// SPECIFICATION: Weighted selection MUST approximately respect weight ratios
	selections := make(map[string]int)
	iterations := 1000
	for i := 0; i < iterations; i++ {
		selection, err := svc.SelectVendor("test-router", "gpt-4")
		require.NoError(t, err)
		selections[selection.Vendor.LLMSlug]++
	}

	heavyCount := selections["vendor-heavy"]
	lightCount := selections["vendor-light"]

	// With 90:10 ratio over 1000 iterations, heavy should be 800-950
	// Allow some variance due to randomness
	assert.Greater(t, heavyCount, 700,
		"vendor-heavy (weight 90) MUST get majority of selections, got %d", heavyCount)
	assert.Less(t, lightCount, 300,
		"vendor-light (weight 10) MUST get minority of selections, got %d", lightCount)
}

func TestSelectVendor_Weighted_ZeroWeightDefaultsToOne(t *testing.T) {
	svc := NewModelRouterService(nil)

	// Both vendors have weight 0, should default to weight 1 (equal distribution)
	vendors := []database.PoolVendor{
		createTestVendor("vendor-a", 0, true, nil),
		createTestVendor("vendor-b", 0, true, nil),
	}

	pool := createTestPool("Zero Weights", "*", "weighted", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	// SPECIFICATION: Zero weight MUST default to 1 for fair distribution
	selections := make(map[string]int)
	for i := 0; i < 100; i++ {
		selection, err := svc.SelectVendor("test-router", "gpt-4")
		require.NoError(t, err)
		selections[selection.Vendor.LLMSlug]++
	}

	// Both should have some selections (roughly equal)
	assert.Greater(t, selections["vendor-a"], 20,
		"vendor-a with zero weight MUST be selected (defaults to 1)")
	assert.Greater(t, selections["vendor-b"], 20,
		"vendor-b with zero weight MUST be selected (defaults to 1)")
}

func TestSelectVendor_Weighted_SkipsInactiveVendors(t *testing.T) {
	svc := NewModelRouterService(nil)

	vendors := []database.PoolVendor{
		createTestVendor("vendor-active", 50, true, nil),
		createTestVendor("vendor-inactive", 50, false, nil), // Same weight but inactive
	}

	pool := createTestPool("Weighted Mixed", "*", "weighted", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	// SPECIFICATION: Weighted selection MUST skip inactive vendors
	for i := 0; i < 50; i++ {
		selection, err := svc.SelectVendor("test-router", "gpt-4")
		require.NoError(t, err)
		assert.Equal(t, "vendor-active", selection.Vendor.LLMSlug,
			"Only active vendor MUST be selected")
	}
}

// ============================================================================
// Model Mapping Tests
// ============================================================================

func TestSelectVendor_ModelMapping_AppliesMapping(t *testing.T) {
	svc := NewModelRouterService(nil)

	mappings := []database.ModelMapping{
		{SourceModel: "gpt-4", TargetModel: "claude-3-opus"},
		{SourceModel: "gpt-3.5", TargetModel: "claude-instant"},
	}
	vendors := []database.PoolVendor{createTestVendor("anthropic", 1, true, mappings)}

	pool := createTestPool("With Mapping", "*", "round_robin", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	// SPECIFICATION: Model mapping MUST translate source to target
	selection, err := svc.SelectVendor("test-router", "gpt-4")
	require.NoError(t, err)
	assert.Equal(t, "claude-3-opus", selection.TargetModel,
		"gpt-4 MUST be mapped to claude-3-opus")

	selection, err = svc.SelectVendor("test-router", "gpt-3.5")
	require.NoError(t, err)
	assert.Equal(t, "claude-instant", selection.TargetModel,
		"gpt-3.5 MUST be mapped to claude-instant")
}

func TestSelectVendor_ModelMapping_NoMappingPreservesModel(t *testing.T) {
	svc := NewModelRouterService(nil)

	mappings := []database.ModelMapping{
		{SourceModel: "gpt-4", TargetModel: "claude-3-opus"},
	}
	vendors := []database.PoolVendor{createTestVendor("anthropic", 1, true, mappings)}

	pool := createTestPool("Partial Mapping", "*", "round_robin", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	// SPECIFICATION: Models without mapping MUST preserve original name
	selection, err := svc.SelectVendor("test-router", "unknown-model")
	require.NoError(t, err)
	assert.Equal(t, "unknown-model", selection.TargetModel,
		"Unmapped model MUST preserve original name")
}

func TestSelectVendor_ModelMapping_VendorSpecific(t *testing.T) {
	svc := NewModelRouterService(nil)

	// Two vendors with different mappings for the same source model
	vendorA := createTestVendor("vendor-a", 1, true, []database.ModelMapping{
		{SourceModel: "gpt-4", TargetModel: "vendor-a-model"},
	})
	vendorB := createTestVendor("vendor-b", 1, true, []database.ModelMapping{
		{SourceModel: "gpt-4", TargetModel: "vendor-b-model"},
	})

	vendors := []database.PoolVendor{vendorA, vendorB}
	pool := createTestPool("Multi Vendor Mapping", "*", "round_robin", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	// SPECIFICATION: Each vendor MUST use its own mapping
	selection1, err := svc.SelectVendor("test-router", "gpt-4")
	require.NoError(t, err)
	assert.Equal(t, "vendor-a", selection1.Vendor.LLMSlug)
	assert.Equal(t, "vendor-a-model", selection1.TargetModel,
		"Vendor A MUST use its own mapping")

	selection2, err := svc.SelectVendor("test-router", "gpt-4")
	require.NoError(t, err)
	assert.Equal(t, "vendor-b", selection2.Vendor.LLMSlug)
	assert.Equal(t, "vendor-b-model", selection2.TargetModel,
		"Vendor B MUST use its own mapping")
}

// ============================================================================
// Error Handling Tests
// ============================================================================

func TestSelectVendor_RouterNotFound(t *testing.T) {
	svc := NewModelRouterService(nil)

	_, err := svc.SelectVendor("non-existent", "gpt-4")

	// SPECIFICATION: Non-existent router MUST return ErrRouterNotFound
	assert.ErrorIs(t, err, ErrRouterNotFound)
}

func TestSelectVendor_NoMatchingPool(t *testing.T) {
	svc := NewModelRouterService(nil)

	vendors := []database.PoolVendor{createTestVendor("openai", 1, true, nil)}
	pool := createTestPool("GPT Only", "gpt-*", "round_robin", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	_, err := svc.SelectVendor("test-router", "claude-3-opus")

	// SPECIFICATION: No matching pool MUST return ErrNoMatchingPool
	assert.ErrorIs(t, err, ErrNoMatchingPool)
}

func TestSelectVendor_NoActiveVendors(t *testing.T) {
	svc := NewModelRouterService(nil)

	// All vendors are inactive
	vendors := []database.PoolVendor{
		createTestVendor("vendor-a", 1, false, nil),
		createTestVendor("vendor-b", 1, false, nil),
	}
	pool := createTestPool("All Inactive", "*", "round_robin", vendors)
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	_, err := svc.SelectVendor("test-router", "gpt-4")

	// SPECIFICATION: No active vendors MUST return ErrNoActiveVendors
	assert.ErrorIs(t, err, ErrNoActiveVendors)
}

// ============================================================================
// Router Management Tests
// ============================================================================

func TestGetRouterCount(t *testing.T) {
	svc := NewModelRouterService(nil)

	assert.Equal(t, 0, svc.GetRouterCount(), "Initial count MUST be 0")

	// Add routers
	vendors := []database.PoolVendor{createTestVendor("openai", 1, true, nil)}
	pool := createTestPool("Test", "*", "round_robin", vendors)

	svc.routerMutex.Lock()
	svc.routers["router-1"] = createTestRouter("router-1", []*CompiledPool{pool})
	svc.routers["router-2"] = createTestRouter("router-2", []*CompiledPool{pool})
	svc.routerMutex.Unlock()

	assert.Equal(t, 2, svc.GetRouterCount(), "Count MUST reflect loaded routers")
}

func TestRouterExists(t *testing.T) {
	svc := NewModelRouterService(nil)

	vendors := []database.PoolVendor{createTestVendor("openai", 1, true, nil)}
	pool := createTestPool("Test", "*", "round_robin", vendors)
	router := createTestRouter("my-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["my-router"] = router
	svc.routerMutex.Unlock()

	assert.True(t, svc.RouterExists("my-router"), "Existing router MUST return true")
	assert.False(t, svc.RouterExists("non-existent"), "Non-existent router MUST return false")
}

func TestGetRouterSlugs(t *testing.T) {
	svc := NewModelRouterService(nil)

	vendors := []database.PoolVendor{createTestVendor("openai", 1, true, nil)}
	pool := createTestPool("Test", "*", "round_robin", vendors)

	svc.routerMutex.Lock()
	svc.routers["alpha"] = createTestRouter("alpha", []*CompiledPool{pool})
	svc.routers["beta"] = createTestRouter("beta", []*CompiledPool{pool})
	svc.routers["gamma"] = createTestRouter("gamma", []*CompiledPool{pool})
	svc.routerMutex.Unlock()

	slugs := svc.GetRouterSlugs()
	assert.Len(t, slugs, 3)
	assert.Contains(t, slugs, "alpha")
	assert.Contains(t, slugs, "beta")
	assert.Contains(t, slugs, "gamma")
}

// ============================================================================
// Router Metadata Store Tests
// ============================================================================

func TestRouterMetadataStore_StoreAndGet(t *testing.T) {
	store := &RouterMetadataStore{}

	meta := &RouterMetadata{
		RouterSlug:  "test-router",
		PoolName:    "default",
		SourceModel: "gpt-4",
		TargetModel: "claude-3-opus",
	}

	store.StoreMetadata("key-1", meta)

	// GetMetadata MUST return and remove the metadata
	retrieved := store.GetMetadata("key-1")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test-router", retrieved.RouterSlug)
	assert.Equal(t, "gpt-4", retrieved.SourceModel)

	// Second call MUST return nil (already removed)
	retrieved = store.GetMetadata("key-1")
	assert.Nil(t, retrieved, "GetMetadata MUST remove metadata after retrieval")
}

func TestRouterMetadataStore_PeekMetadata(t *testing.T) {
	store := &RouterMetadataStore{}

	meta := &RouterMetadata{
		RouterSlug: "test-router",
	}

	store.StoreMetadata("key-1", meta)

	// PeekMetadata MUST return without removing
	retrieved := store.PeekMetadata("key-1")
	assert.NotNil(t, retrieved)

	// Second peek MUST still work
	retrieved = store.PeekMetadata("key-1")
	assert.NotNil(t, retrieved, "PeekMetadata MUST NOT remove metadata")
}

func TestRouterMetadataStore_NonExistentKey(t *testing.T) {
	store := &RouterMetadataStore{}

	assert.Nil(t, store.GetMetadata("non-existent"))
	assert.Nil(t, store.PeekMetadata("non-existent"))
}

func TestGetRouterMetadataFromContext(t *testing.T) {
	// Without metadata
	ctx := context.Background()
	assert.Nil(t, GetRouterMetadataFromContext(ctx))

	// With metadata
	meta := &RouterMetadata{RouterSlug: "test"}
	ctx = context.WithValue(ctx, RouterMetadataKey, meta)
	retrieved := GetRouterMetadataFromContext(ctx)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test", retrieved.RouterSlug)
}

// ============================================================================
// Selection Result Tests
// ============================================================================

func TestVendorSelection_ContainsPoolInfo(t *testing.T) {
	svc := NewModelRouterService(nil)

	vendors := []database.PoolVendor{createTestVendor("openai", 1, true, nil)}
	pool := createTestPool("My Pool", "gpt-*", "round_robin", vendors)
	pool.Pool.Priority = 42
	router := createTestRouter("test-router", []*CompiledPool{pool})

	svc.routerMutex.Lock()
	svc.routers["test-router"] = router
	svc.routerMutex.Unlock()

	selection, err := svc.SelectVendor("test-router", "gpt-4")
	require.NoError(t, err)

	// SPECIFICATION: Selection MUST include pool information for analytics
	assert.NotNil(t, selection.Pool)
	assert.Equal(t, "My Pool", selection.Pool.Name)
	assert.Equal(t, "gpt-*", selection.Pool.ModelPattern)
	assert.Equal(t, 42, selection.Pool.Priority)
}
