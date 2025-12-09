package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create a test LLM for vendor references
func createTestLLMForRouter(t *testing.T, db interface{ Create(interface{}) interface{ Error() error } }, name string) *LLM {
	llm := &LLM{
		Name:        name,
		Vendor:      "test-vendor",
		Active:      true,
		APIEndpoint: "https://api.test.com",
	}
	// Use raw gorm for simplicity
	return llm
}

// ============================================================================
// ModelRouter Tests
// ============================================================================

func TestModelRouter_NewModelRouter(t *testing.T) {
	router := NewModelRouter()
	assert.NotNil(t, router)
}

func TestModelRouter_Create(t *testing.T) {
	db := setupTestDB(t)

	// Create test LLM first
	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	err := db.Create(llm).Error
	require.NoError(t, err)

	// Create router with nested pools, vendors, and mappings
	router := &ModelRouter{
		Name:        "Test Router",
		Slug:        "test-router",
		Description: "A test router",
		APICompat:   "openai",
		Active:      true,
		Namespace:   "test-namespace",
		Pools: []*ModelPool{
			{
				Name:               "GPT Pool",
				ModelPattern:       "gpt-*",
				SelectionAlgorithm: SelectionRoundRobin,
				Priority:           100,
				Vendors: []*PoolVendor{
					{
						LLMID:  llm.ID,
						Weight: 1,
						Active: true,
						Mappings: []*ModelMapping{
							{
								SourceModel: "gpt-4",
								TargetModel: "gpt-4-turbo",
							},
						},
					},
				},
			},
		},
	}

	err = router.Create(db)
	require.NoError(t, err)

	// Verify router was created with ID
	assert.NotZero(t, router.ID, "Router MUST have an ID after creation")

	// Verify nested entities were created
	assert.NotZero(t, router.Pools[0].ID, "Pool MUST have an ID after creation")
	assert.NotZero(t, router.Pools[0].Vendors[0].ID, "Vendor MUST have an ID after creation")
	assert.NotZero(t, router.Pools[0].Vendors[0].Mappings[0].ID, "Mapping MUST have an ID after creation")

	// Verify foreign keys are set correctly
	assert.Equal(t, router.ID, router.Pools[0].RouterID, "Pool.RouterID MUST match Router.ID")
	assert.Equal(t, router.Pools[0].ID, router.Pools[0].Vendors[0].PoolID, "Vendor.PoolID MUST match Pool.ID")
	assert.Equal(t, router.Pools[0].Vendors[0].ID, router.Pools[0].Vendors[0].Mappings[0].VendorID, "Mapping.VendorID MUST match Vendor.ID")
}

func TestModelRouter_Get(t *testing.T) {
	db := setupTestDB(t)

	// Setup: Create LLM and router
	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	router := &ModelRouter{
		Name:      "Test Router",
		Slug:      "test-router",
		Active:    true,
		Namespace: "",
		Pools: []*ModelPool{
			{
				Name:               "Test Pool",
				ModelPattern:       "*",
				SelectionAlgorithm: SelectionRoundRobin,
				Priority:           0,
				Vendors: []*PoolVendor{
					{
						LLMID:  llm.ID,
						Weight: 1,
						Active: true,
						Mappings: []*ModelMapping{
							{SourceModel: "source", TargetModel: "target"},
						},
					},
				},
			},
		},
	}
	require.NoError(t, router.Create(db))

	// Act: Get the router by ID
	fetchedRouter := NewModelRouter()
	err := fetchedRouter.Get(db, router.ID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, router.Name, fetchedRouter.Name)
	assert.Equal(t, router.Slug, fetchedRouter.Slug)
	assert.Equal(t, router.Active, fetchedRouter.Active)

	// SPECIFICATION: Get MUST preload all nested relationships
	require.Len(t, fetchedRouter.Pools, 1, "Pools MUST be preloaded")
	require.Len(t, fetchedRouter.Pools[0].Vendors, 1, "Vendors MUST be preloaded")
	assert.NotNil(t, fetchedRouter.Pools[0].Vendors[0].LLM, "LLM MUST be preloaded")
	require.Len(t, fetchedRouter.Pools[0].Vendors[0].Mappings, 1, "Mappings MUST be preloaded")
}

func TestModelRouter_Get_NotFound(t *testing.T) {
	db := setupTestDB(t)

	// Act: Try to get non-existent router
	router := NewModelRouter()
	err := router.Get(db, 999)

	// Assert: MUST return error for non-existent router
	assert.Error(t, err, "Get MUST return error for non-existent router")
}

func TestModelRouter_GetBySlug(t *testing.T) {
	db := setupTestDB(t)

	// Setup
	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	router := &ModelRouter{
		Name:      "Test Router",
		Slug:      "unique-slug",
		Active:    true,
		Namespace: "prod",
		Pools: []*ModelPool{{
			Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}},
		}},
	}
	require.NoError(t, router.Create(db))

	// Act: Get by slug and namespace
	fetchedRouter := NewModelRouter()
	err := fetchedRouter.GetBySlug(db, "unique-slug", "prod")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, router.ID, fetchedRouter.ID)
	assert.Equal(t, "unique-slug", fetchedRouter.Slug)
	assert.Equal(t, "prod", fetchedRouter.Namespace)
}

func TestModelRouter_GetBySlug_SameSlugDifferentNamespace(t *testing.T) {
	db := setupTestDB(t)

	// SPECIFICATION: Same slug in different namespaces MUST be allowed
	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	router1 := &ModelRouter{
		Name: "Router 1", Slug: "shared-slug", Namespace: "namespace-a",
		Pools: []*ModelPool{{Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}}},
	}
	router2 := &ModelRouter{
		Name: "Router 2", Slug: "shared-slug", Namespace: "namespace-b",
		Pools: []*ModelPool{{Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}}},
	}

	require.NoError(t, router1.Create(db))
	require.NoError(t, router2.Create(db), "Same slug in different namespace MUST be allowed")

	// Verify we can fetch each independently
	fetched1 := NewModelRouter()
	require.NoError(t, fetched1.GetBySlug(db, "shared-slug", "namespace-a"))
	assert.Equal(t, router1.ID, fetched1.ID)

	fetched2 := NewModelRouter()
	require.NoError(t, fetched2.GetBySlug(db, "shared-slug", "namespace-b"))
	assert.Equal(t, router2.ID, fetched2.ID)
}

func TestModelRouter_Update(t *testing.T) {
	db := setupTestDB(t)

	// Setup
	llm1 := &LLM{Name: "LLM1", Vendor: "openai", Active: true, APIEndpoint: "https://api1.test.com"}
	llm2 := &LLM{Name: "LLM2", Vendor: "anthropic", Active: true, APIEndpoint: "https://api2.test.com"}
	require.NoError(t, db.Create(llm1).Error)
	require.NoError(t, db.Create(llm2).Error)

	router := &ModelRouter{
		Name: "Original Name", Slug: "test-router", Active: true,
		Pools: []*ModelPool{{
			Name: "Original Pool", ModelPattern: "gpt-*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{LLMID: llm1.ID, Weight: 1, Active: true}},
		}},
	}
	require.NoError(t, router.Create(db))
	originalPoolID := router.Pools[0].ID

	// Act: Update router with new pools
	router.Name = "Updated Name"
	router.Pools = []*ModelPool{
		{
			Name: "New Pool 1", ModelPattern: "claude-*", SelectionAlgorithm: SelectionWeighted, Priority: 100,
			Vendors: []*PoolVendor{{LLMID: llm2.ID, Weight: 2, Active: true}},
		},
		{
			Name: "New Pool 2", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin, Priority: 0,
			Vendors: []*PoolVendor{{LLMID: llm1.ID, Weight: 1, Active: true}},
		},
	}
	err := router.Update(db)
	require.NoError(t, err)

	// Assert: Verify update took effect
	fetchedRouter := NewModelRouter()
	require.NoError(t, fetchedRouter.Get(db, router.ID))

	assert.Equal(t, "Updated Name", fetchedRouter.Name)
	require.Len(t, fetchedRouter.Pools, 2, "Router MUST have 2 pools after update")
	assert.Equal(t, "New Pool 1", fetchedRouter.Pools[0].Name)

	// SPECIFICATION: Update MUST delete old pools and create new ones
	// Old pool should no longer exist
	var oldPoolCount int64
	db.Model(&ModelPool{}).Where("id = ?", originalPoolID).Count(&oldPoolCount)
	assert.Equal(t, int64(0), oldPoolCount, "Old pool MUST be deleted after update")
}

func TestModelRouter_Delete(t *testing.T) {
	db := setupTestDB(t)

	// Setup
	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	router := &ModelRouter{
		Name: "To Delete", Slug: "delete-me",
		Pools: []*ModelPool{{
			Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{
				LLMID: llm.ID, Weight: 1, Active: true,
				Mappings: []*ModelMapping{{SourceModel: "src", TargetModel: "tgt"}},
			}},
		}},
	}
	require.NoError(t, router.Create(db))
	routerID := router.ID
	poolID := router.Pools[0].ID
	vendorID := router.Pools[0].Vendors[0].ID
	mappingID := router.Pools[0].Vendors[0].Mappings[0].ID

	// Act
	err := router.Delete(db)
	require.NoError(t, err)

	// Assert: All nested entities MUST be cascade deleted
	var routerCount, poolCount, vendorCount, mappingCount int64
	db.Model(&ModelRouter{}).Where("id = ?", routerID).Count(&routerCount)
	db.Model(&ModelPool{}).Where("id = ?", poolID).Count(&poolCount)
	db.Model(&PoolVendor{}).Where("id = ?", vendorID).Count(&vendorCount)
	db.Model(&ModelMapping{}).Where("id = ?", mappingID).Count(&mappingCount)

	assert.Equal(t, int64(0), routerCount, "Router MUST be deleted")
	assert.Equal(t, int64(0), poolCount, "Pool MUST be cascade deleted")
	assert.Equal(t, int64(0), vendorCount, "Vendor MUST be cascade deleted")
	assert.Equal(t, int64(0), mappingCount, "Mapping MUST be cascade deleted")
}

// ============================================================================
// ModelRouters Collection Tests
// ============================================================================

func TestModelRouters_GetAll_Empty(t *testing.T) {
	db := setupTestDB(t)

	var routers ModelRouters
	totalCount, totalPages, err := routers.GetAll(db, 10, 1, false)

	require.NoError(t, err)
	assert.Equal(t, int64(0), totalCount)
	assert.Equal(t, 0, totalPages)
	assert.Len(t, routers, 0)
}

func TestModelRouters_GetAll_Pagination(t *testing.T) {
	db := setupTestDB(t)

	// Setup: Create 5 routers
	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	for i := 1; i <= 5; i++ {
		router := &ModelRouter{
			Name: "Router", Slug: "router-" + string(rune('0'+i)),
			Pools: []*ModelPool{{Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
				Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}}},
		}
		require.NoError(t, router.Create(db))
	}

	// Test page 1
	var page1 ModelRouters
	totalCount, totalPages, err := page1.GetAll(db, 2, 1, false)
	require.NoError(t, err)
	assert.Equal(t, int64(5), totalCount)
	assert.Equal(t, 3, totalPages)
	assert.Len(t, page1, 2)

	// Test page 2
	var page2 ModelRouters
	_, _, err = page2.GetAll(db, 2, 2, false)
	require.NoError(t, err)
	assert.Len(t, page2, 2)

	// Test page 3 (last page with 1 item)
	var page3 ModelRouters
	_, _, err = page3.GetAll(db, 2, 3, false)
	require.NoError(t, err)
	assert.Len(t, page3, 1)

	// Test all=true bypasses pagination
	var allRouters ModelRouters
	_, _, err = allRouters.GetAll(db, 2, 1, true)
	require.NoError(t, err)
	assert.Len(t, allRouters, 5, "all=true MUST return all routers")
}

func TestModelRouters_GetByNamespace(t *testing.T) {
	db := setupTestDB(t)

	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	// Create routers in different namespaces
	router1 := &ModelRouter{Name: "R1", Slug: "r1", Namespace: "prod",
		Pools: []*ModelPool{{Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}}}}
	router2 := &ModelRouter{Name: "R2", Slug: "r2", Namespace: "prod",
		Pools: []*ModelPool{{Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}}}}
	router3 := &ModelRouter{Name: "R3", Slug: "r3", Namespace: "dev",
		Pools: []*ModelPool{{Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}}}}

	require.NoError(t, router1.Create(db))
	require.NoError(t, router2.Create(db))
	require.NoError(t, router3.Create(db))

	// Test: Get by namespace
	var prodRouters ModelRouters
	err := prodRouters.GetByNamespace(db, "prod")
	require.NoError(t, err)
	assert.Len(t, prodRouters, 2, "MUST return only routers in 'prod' namespace")

	var devRouters ModelRouters
	err = devRouters.GetByNamespace(db, "dev")
	require.NoError(t, err)
	assert.Len(t, devRouters, 1, "MUST return only routers in 'dev' namespace")
}

func TestModelRouters_GetActiveRouters(t *testing.T) {
	db := setupTestDB(t)

	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	// Create active and inactive routers
	activeRouter := &ModelRouter{Name: "Active", Slug: "active", Active: true,
		Pools: []*ModelPool{{Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}}}}
	inactiveRouter := &ModelRouter{Name: "Inactive", Slug: "inactive", Active: false,
		Pools: []*ModelPool{{Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}}}}

	require.NoError(t, activeRouter.Create(db))
	require.NoError(t, inactiveRouter.Create(db))

	// Test
	var activeRouters ModelRouters
	err := activeRouters.GetActiveRouters(db)
	require.NoError(t, err)
	assert.Len(t, activeRouters, 1, "MUST return only active routers")
	assert.Equal(t, "Active", activeRouters[0].Name)
}

func TestModelRouters_GetActiveRoutersByNamespace(t *testing.T) {
	db := setupTestDB(t)

	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	// Create routers with different active states and namespaces
	r1 := &ModelRouter{Name: "R1", Slug: "r1", Active: true, Namespace: "prod",
		Pools: []*ModelPool{{Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}}}}
	r2 := &ModelRouter{Name: "R2", Slug: "r2", Active: false, Namespace: "prod",
		Pools: []*ModelPool{{Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}}}}
	r3 := &ModelRouter{Name: "R3", Slug: "r3", Active: true, Namespace: "dev",
		Pools: []*ModelPool{{Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}}}}

	require.NoError(t, r1.Create(db))
	require.NoError(t, r2.Create(db))
	require.NoError(t, r3.Create(db))

	// Test
	var routers ModelRouters
	err := routers.GetActiveRoutersByNamespace(db, "prod")
	require.NoError(t, err)
	assert.Len(t, routers, 1, "MUST return only active routers in specified namespace")
	assert.Equal(t, "R1", routers[0].Name)
}

// ============================================================================
// ModelPool Tests
// ============================================================================

func TestModelPool_Get(t *testing.T) {
	db := setupTestDB(t)

	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	router := &ModelRouter{
		Name: "Router", Slug: "router",
		Pools: []*ModelPool{{
			Name: "Test Pool", ModelPattern: "gpt-*", SelectionAlgorithm: SelectionWeighted, Priority: 50,
			Vendors: []*PoolVendor{{
				LLMID: llm.ID, Weight: 3, Active: true,
				Mappings: []*ModelMapping{{SourceModel: "gpt-4", TargetModel: "gpt-4-turbo"}},
			}},
		}},
	}
	require.NoError(t, router.Create(db))

	// Act
	pool := NewModelPool()
	err := pool.Get(db, router.Pools[0].ID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "Test Pool", pool.Name)
	assert.Equal(t, "gpt-*", pool.ModelPattern)
	assert.Equal(t, SelectionWeighted, pool.SelectionAlgorithm)
	assert.Equal(t, 50, pool.Priority)

	// SPECIFICATION: Pool.Get MUST preload vendors with LLM and mappings
	require.Len(t, pool.Vendors, 1)
	assert.NotNil(t, pool.Vendors[0].LLM)
	require.Len(t, pool.Vendors[0].Mappings, 1)
}

func TestModelPools_GetByRouterID_OrderedByPriority(t *testing.T) {
	db := setupTestDB(t)

	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	// Create router with pools in non-priority order
	router := &ModelRouter{
		Name: "Router", Slug: "router",
		Pools: []*ModelPool{
			{Name: "Low Priority", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin, Priority: 10,
				Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}},
			{Name: "High Priority", ModelPattern: "gpt-*", SelectionAlgorithm: SelectionRoundRobin, Priority: 100,
				Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}},
			{Name: "Medium Priority", ModelPattern: "claude-*", SelectionAlgorithm: SelectionRoundRobin, Priority: 50,
				Vendors: []*PoolVendor{{LLMID: llm.ID, Weight: 1, Active: true}}},
		},
	}
	require.NoError(t, router.Create(db))

	// Act
	var pools ModelPools
	err := pools.GetByRouterID(db, router.ID)

	// Assert: SPECIFICATION: Pools MUST be ordered by priority DESC
	require.NoError(t, err)
	require.Len(t, pools, 3)
	assert.Equal(t, "High Priority", pools[0].Name, "First pool MUST be highest priority")
	assert.Equal(t, "Medium Priority", pools[1].Name)
	assert.Equal(t, "Low Priority", pools[2].Name, "Last pool MUST be lowest priority")
}

// ============================================================================
// PoolVendor Tests
// ============================================================================

func TestPoolVendor_Get(t *testing.T) {
	db := setupTestDB(t)

	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	router := &ModelRouter{
		Name: "Router", Slug: "router",
		Pools: []*ModelPool{{
			Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{
				LLMID: llm.ID, Weight: 5, Active: true,
				Mappings: []*ModelMapping{
					{SourceModel: "src1", TargetModel: "tgt1"},
					{SourceModel: "src2", TargetModel: "tgt2"},
				},
			}},
		}},
	}
	require.NoError(t, router.Create(db))

	// Act
	vendor := NewPoolVendor()
	err := vendor.Get(db, router.Pools[0].Vendors[0].ID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, llm.ID, vendor.LLMID)
	assert.Equal(t, 5, vendor.Weight)
	assert.True(t, vendor.Active)

	// SPECIFICATION: Vendor.Get MUST preload LLM and Mappings
	assert.NotNil(t, vendor.LLM, "LLM MUST be preloaded")
	assert.Equal(t, "TestLLM", vendor.LLM.Name)
	require.Len(t, vendor.Mappings, 2, "Mappings MUST be preloaded")
}

func TestPoolVendors_GetActiveVendorsByPoolID(t *testing.T) {
	db := setupTestDB(t)

	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	router := &ModelRouter{
		Name: "Router", Slug: "router",
		Pools: []*ModelPool{{
			Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{
				{LLMID: llm.ID, Weight: 1, Active: true},
				{LLMID: llm.ID, Weight: 2, Active: true}, // Will be deactivated below
				{LLMID: llm.ID, Weight: 3, Active: true},
			},
		}},
	}
	require.NoError(t, router.Create(db))

	// Deactivate the second vendor by updating directly in DB
	// (GORM default:true prevents setting false via struct init)
	secondVendorID := router.Pools[0].Vendors[1].ID
	require.NoError(t, db.Model(&PoolVendor{}).Where("id = ?", secondVendorID).Update("active", false).Error)

	// Act
	var vendors PoolVendors
	err := vendors.GetActiveVendorsByPoolID(db, router.Pools[0].ID)

	// Assert: SPECIFICATION: MUST return only active vendors
	require.NoError(t, err)
	assert.Len(t, vendors, 2, "MUST return only active vendors")
	for _, v := range vendors {
		assert.True(t, v.Active, "All returned vendors MUST be active")
	}
}

// ============================================================================
// ModelMapping Tests
// ============================================================================

func TestModelMapping_Get(t *testing.T) {
	db := setupTestDB(t)

	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	router := &ModelRouter{
		Name: "Router", Slug: "router",
		Pools: []*ModelPool{{
			Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{
				LLMID: llm.ID, Weight: 1, Active: true,
				Mappings: []*ModelMapping{{SourceModel: "gpt-4", TargetModel: "gpt-4-turbo"}},
			}},
		}},
	}
	require.NoError(t, router.Create(db))

	// Act
	mapping := NewModelMapping()
	err := mapping.Get(db, router.Pools[0].Vendors[0].Mappings[0].ID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "gpt-4", mapping.SourceModel)
	assert.Equal(t, "gpt-4-turbo", mapping.TargetModel)
}

func TestModelMappings_GetByVendorID(t *testing.T) {
	db := setupTestDB(t)

	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	router := &ModelRouter{
		Name: "Router", Slug: "router",
		Pools: []*ModelPool{{
			Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{
				LLMID: llm.ID, Weight: 1, Active: true,
				Mappings: []*ModelMapping{
					{SourceModel: "model-a", TargetModel: "model-a-v2"},
					{SourceModel: "model-b", TargetModel: "model-b-v2"},
					{SourceModel: "model-c", TargetModel: "model-c-v2"},
				},
			}},
		}},
	}
	require.NoError(t, router.Create(db))

	// Act
	var mappings ModelMappings
	err := mappings.GetByVendorID(db, router.Pools[0].Vendors[0].ID)

	// Assert
	require.NoError(t, err)
	assert.Len(t, mappings, 3)
}

func TestModelMapping_GetMappingForModel(t *testing.T) {
	db := setupTestDB(t)

	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	router := &ModelRouter{
		Name: "Router", Slug: "router",
		Pools: []*ModelPool{{
			Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{
				LLMID: llm.ID, Weight: 1, Active: true,
				Mappings: []*ModelMapping{
					{SourceModel: "gpt-4", TargetModel: "gpt-4-turbo"},
					{SourceModel: "gpt-3.5", TargetModel: "gpt-3.5-turbo"},
				},
			}},
		}},
	}
	require.NoError(t, router.Create(db))
	vendorID := router.Pools[0].Vendors[0].ID

	// Test: Find existing mapping
	mapping := NewModelMapping()
	err := mapping.GetMappingForModel(db, vendorID, "gpt-4")
	require.NoError(t, err)
	assert.Equal(t, "gpt-4-turbo", mapping.TargetModel)

	// Test: Non-existent mapping
	mapping2 := NewModelMapping()
	err = mapping2.GetMappingForModel(db, vendorID, "non-existent")
	assert.Error(t, err, "MUST return error when mapping not found")
}

// ============================================================================
// Cascade Delete Tests
// ============================================================================

func TestCascadeDelete_Pool(t *testing.T) {
	db := setupTestDB(t)

	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	router := &ModelRouter{
		Name: "Router", Slug: "router",
		Pools: []*ModelPool{{
			Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{
				LLMID: llm.ID, Weight: 1, Active: true,
				Mappings: []*ModelMapping{{SourceModel: "src", TargetModel: "tgt"}},
			}},
		}},
	}
	require.NoError(t, router.Create(db))

	pool := router.Pools[0]
	vendorID := pool.Vendors[0].ID
	mappingID := pool.Vendors[0].Mappings[0].ID

	// Act: Delete pool using the Delete method
	err := pool.Delete(db)
	require.NoError(t, err)

	// Assert: Vendors and mappings MUST be cascade deleted
	var vendorCount, mappingCount int64
	db.Model(&PoolVendor{}).Where("id = ?", vendorID).Count(&vendorCount)
	db.Model(&ModelMapping{}).Where("id = ?", mappingID).Count(&mappingCount)

	assert.Equal(t, int64(0), vendorCount, "Vendor MUST be cascade deleted when pool is deleted")
	assert.Equal(t, int64(0), mappingCount, "Mapping MUST be cascade deleted when pool is deleted")
}

func TestCascadeDelete_Vendor(t *testing.T) {
	db := setupTestDB(t)

	llm := &LLM{Name: "TestLLM", Vendor: "openai", Active: true, APIEndpoint: "https://api.test.com"}
	require.NoError(t, db.Create(llm).Error)

	router := &ModelRouter{
		Name: "Router", Slug: "router",
		Pools: []*ModelPool{{
			Name: "Pool", ModelPattern: "*", SelectionAlgorithm: SelectionRoundRobin,
			Vendors: []*PoolVendor{{
				LLMID: llm.ID, Weight: 1, Active: true,
				Mappings: []*ModelMapping{
					{SourceModel: "src1", TargetModel: "tgt1"},
					{SourceModel: "src2", TargetModel: "tgt2"},
				},
			}},
		}},
	}
	require.NoError(t, router.Create(db))

	vendor := router.Pools[0].Vendors[0]
	vendorID := vendor.ID

	// Act: Delete vendor using the Delete method
	err := vendor.Delete(db)
	require.NoError(t, err)

	// Assert: Mappings MUST be cascade deleted
	var mappingCount int64
	db.Model(&ModelMapping{}).Where("vendor_id = ?", vendorID).Count(&mappingCount)
	assert.Equal(t, int64(0), mappingCount, "Mappings MUST be cascade deleted when vendor is deleted")
}
