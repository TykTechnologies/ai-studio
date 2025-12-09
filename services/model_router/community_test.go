package model_router

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Community Service Tests
// ============================================================================
// SPECIFICATION: Community Edition model router service MUST return ErrEnterpriseFeature
// for all operations. This ensures users are clearly informed when attempting to use
// enterprise-only features.

func TestCommunityService_AllMethodsReturnEnterpriseError(t *testing.T) {
	// SPECIFICATION: newCommunityService() MUST return a valid Service implementation
	svc := newCommunityService()
	require.NotNil(t, svc, "newCommunityService() MUST return non-nil service")

	// Dummy router for tests requiring input
	testRouter := &models.ModelRouter{
		Name: "Test",
		Slug: "test",
		Pools: []*models.ModelPool{{
			Name:               "Pool",
			ModelPattern:       "*",
			SelectionAlgorithm: models.SelectionRoundRobin,
		}},
	}

	tests := []struct {
		name     string
		testFunc func() error
	}{
		{
			name: "CreateRouter",
			testFunc: func() error {
				return svc.CreateRouter(testRouter)
			},
		},
		{
			name: "GetRouter",
			testFunc: func() error {
				_, err := svc.GetRouter(1)
				return err
			},
		},
		{
			name: "GetRouterBySlug",
			testFunc: func() error {
				_, err := svc.GetRouterBySlug("test", "")
				return err
			},
		},
		{
			name: "UpdateRouter",
			testFunc: func() error {
				return svc.UpdateRouter(testRouter)
			},
		},
		{
			name: "DeleteRouter",
			testFunc: func() error {
				return svc.DeleteRouter(1)
			},
		},
		{
			name: "ListRouters",
			testFunc: func() error {
				_, _, _, err := svc.ListRouters(10, 1, false)
				return err
			},
		},
		{
			name: "ListRoutersByNamespace",
			testFunc: func() error {
				_, err := svc.ListRoutersByNamespace("default")
				return err
			},
		},
		{
			name: "GetActiveRouters",
			testFunc: func() error {
				_, err := svc.GetActiveRouters()
				return err
			},
		},
		{
			name: "GetActiveRoutersByNamespace",
			testFunc: func() error {
				_, err := svc.GetActiveRoutersByNamespace("default")
				return err
			},
		},
		{
			name: "ValidateRouter",
			testFunc: func() error {
				return svc.ValidateRouter(testRouter)
			},
		},
		{
			name: "ToggleRouterActive",
			testFunc: func() error {
				return svc.ToggleRouterActive(1, true)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.testFunc()
			// SPECIFICATION: All CE methods MUST return ErrEnterpriseFeature
			assert.ErrorIs(t, err, ErrEnterpriseFeature,
				"%s MUST return ErrEnterpriseFeature in Community Edition", tt.name)
		})
	}
}

// TestCommunityService_GetRouterReturnsNilRouter ensures GetRouter returns nil for the router value
func TestCommunityService_GetRouterReturnsNilRouter(t *testing.T) {
	svc := newCommunityService()

	router, err := svc.GetRouter(1)

	// SPECIFICATION: GetRouter in CE MUST return nil for the router and ErrEnterpriseFeature
	assert.Nil(t, router, "Router MUST be nil in Community Edition")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
}

// TestCommunityService_GetRouterBySlugReturnsNilRouter ensures GetRouterBySlug returns nil
func TestCommunityService_GetRouterBySlugReturnsNilRouter(t *testing.T) {
	svc := newCommunityService()

	router, err := svc.GetRouterBySlug("test", "")

	// SPECIFICATION: GetRouterBySlug in CE MUST return nil for the router
	assert.Nil(t, router, "Router MUST be nil in Community Edition")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
}

// TestCommunityService_ListRoutersReturnsEmptySlice ensures ListRouters returns empty values
func TestCommunityService_ListRoutersReturnsEmptySlice(t *testing.T) {
	svc := newCommunityService()

	routers, totalCount, totalPages, err := svc.ListRouters(10, 1, false)

	// SPECIFICATION: ListRouters in CE MUST return nil slice and zero counts
	assert.Nil(t, routers, "Routers MUST be nil in Community Edition")
	assert.Equal(t, int64(0), totalCount, "Total count MUST be 0 in Community Edition")
	assert.Equal(t, 0, totalPages, "Total pages MUST be 0 in Community Edition")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
}

// TestCommunityService_ListRoutersByNamespaceReturnsEmptySlice ensures empty results
func TestCommunityService_ListRoutersByNamespaceReturnsEmptySlice(t *testing.T) {
	svc := newCommunityService()

	routers, err := svc.ListRoutersByNamespace("default")

	// SPECIFICATION: ListRoutersByNamespace in CE MUST return nil slice
	assert.Nil(t, routers, "Routers MUST be nil in Community Edition")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
}

// TestCommunityService_GetActiveRoutersReturnsEmptySlice ensures empty results
func TestCommunityService_GetActiveRoutersReturnsEmptySlice(t *testing.T) {
	svc := newCommunityService()

	routers, err := svc.GetActiveRouters()

	// SPECIFICATION: GetActiveRouters in CE MUST return nil slice
	assert.Nil(t, routers, "Routers MUST be nil in Community Edition")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
}

// TestCommunityService_GetActiveRoutersByNamespaceReturnsEmptySlice ensures empty results
func TestCommunityService_GetActiveRoutersByNamespaceReturnsEmptySlice(t *testing.T) {
	svc := newCommunityService()

	routers, err := svc.GetActiveRoutersByNamespace("default")

	// SPECIFICATION: GetActiveRoutersByNamespace in CE MUST return nil slice
	assert.Nil(t, routers, "Routers MUST be nil in Community Edition")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
}

// TestErrEnterpriseFeature_ErrorMessage validates the error message content
func TestErrEnterpriseFeature_ErrorMessage(t *testing.T) {
	// SPECIFICATION: The enterprise feature error MUST contain helpful information
	// including mention of Enterprise Edition and pricing URL
	errMsg := ErrEnterpriseFeature.Error()

	assert.Contains(t, errMsg, "Enterprise Edition",
		"Error message MUST mention Enterprise Edition")
	assert.Contains(t, errMsg, "tyk.io",
		"Error message MUST contain pricing URL")
}

// TestCommunityService_ImplementsServiceInterface ensures type safety
func TestCommunityService_ImplementsServiceInterface(t *testing.T) {
	// SPECIFICATION: communityService MUST implement the Service interface
	var _ Service = newCommunityService()
	// If the above line compiles, the test passes
}
