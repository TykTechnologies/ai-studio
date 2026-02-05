package models

import (
	"testing"

	"github.com/gosimple/slug"
	"github.com/stretchr/testify/assert"
)

func TestTool_NewTool(t *testing.T) {
	tool := NewTool()
	assert.NotNil(t, tool)
}

func TestTool_Create(t *testing.T) {
	db := setupTestDB(t)

	tool := &Tool{
		Name:         "Test Tool",
		Description:  "This is a test tool",
		ToolType:     ToolTypeREST,
		PrivacyScore: 8,
	}
	err := tool.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, tool.ID)
}

func TestTool_Get(t *testing.T) {
	db := setupTestDB(t)

	tool := &Tool{
		Name:         "Test Tool",
		Description:  "This is a test tool",
		ToolType:     ToolTypeREST,
		PrivacyScore: 8,
	}
	err := tool.Create(db)
	assert.NoError(t, err)

	fetchedTool := &Tool{}
	err = fetchedTool.Get(db, tool.ID)
	assert.NoError(t, err)
	assert.Equal(t, tool.ID, fetchedTool.ID)
	assert.Equal(t, tool.Name, fetchedTool.Name)
	assert.Equal(t, tool.Description, fetchedTool.Description)
	assert.Equal(t, tool.ToolType, fetchedTool.ToolType)
	assert.Equal(t, tool.PrivacyScore, fetchedTool.PrivacyScore)
}

func TestTool_Update(t *testing.T) {
	db := setupTestDB(t)

	tool := &Tool{
		Name:         "Test Tool",
		Description:  "This is a test tool",
		ToolType:     ToolTypeREST,
		PrivacyScore: 8,
	}
	err := tool.Create(db)
	assert.NoError(t, err)

	tool.Name = "Updated Tool Name"
	tool.Description = "Updated description"
	tool.PrivacyScore = 9
	err = tool.Update(db)
	assert.NoError(t, err)

	fetchedTool := &Tool{}
	err = fetchedTool.Get(db, tool.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Tool Name", fetchedTool.Name)
	assert.Equal(t, "Updated description", fetchedTool.Description)
	assert.Equal(t, 9, fetchedTool.PrivacyScore)
}

func TestTool_Delete(t *testing.T) {
	db := setupTestDB(t)

	tool := &Tool{
		Name:         "Test Tool",
		Description:  "This is a test tool",
		ToolType:     ToolTypeREST,
		PrivacyScore: 8,
	}
	err := tool.Create(db)
	assert.NoError(t, err)

	err = tool.Delete(db)
	assert.NoError(t, err)

	fetchedTool := &Tool{}
	err = fetchedTool.Get(db, tool.ID)
	assert.Error(t, err) // Should return an error as the tool is deleted
}

func TestTool_GetByName(t *testing.T) {
	db := setupTestDB(t)

	tool := &Tool{
		Name:         "Unique Tool Name",
		Description:  "This is a unique tool",
		ToolType:     ToolTypeREST,
		PrivacyScore: 8,
	}
	err := tool.Create(db)
	assert.NoError(t, err)

	fetchedTool := &Tool{}
	err = fetchedTool.GetByName(db, "Unique Tool Name")
	assert.NoError(t, err)
	assert.Equal(t, tool.ID, fetchedTool.ID)
	assert.Equal(t, tool.Name, fetchedTool.Name)
	assert.Equal(t, tool.Description, fetchedTool.Description)
}

func TestTools_GetAll(t *testing.T) {
	db := setupTestDB(t)

	tools := []*Tool{
		{Name: "Tool 1", Description: "Description 1", ToolType: ToolTypeREST, PrivacyScore: 7},
		{Name: "Tool 2", Description: "Description 2", ToolType: ToolTypeREST, PrivacyScore: 8},
		{Name: "Tool 3", Description: "Description 3", ToolType: ToolTypeREST, PrivacyScore: 9},
	}
	for _, tool := range tools {
		err := tool.Create(db)
		assert.NoError(t, err)
	}

	var fetchedTools Tools
	_, _, err := fetchedTools.GetAll(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, fetchedTools, 3)
}

func TestTools_GetByType(t *testing.T) {
	db := setupTestDB(t)

	tools := []*Tool{
		{Name: "Tool 1", Description: "Description 1", ToolType: ToolTypeREST, PrivacyScore: 7},
		{Name: "Tool 2", Description: "Description 2", ToolType: ToolTypeREST, PrivacyScore: 8},
		{Name: "Tool 3", Description: "Description 3", ToolType: "OTHER", PrivacyScore: 9},
	}
	for _, tool := range tools {
		err := tool.Create(db)
		assert.NoError(t, err)
	}

	var fetchedTools Tools
	err := fetchedTools.GetByType(db, ToolTypeREST)
	assert.NoError(t, err)
	assert.Len(t, fetchedTools, 2)
}

func TestTools_GetByPrivacyScoreMin(t *testing.T) {
	db := setupTestDB(t)

	tools := []*Tool{
		{Name: "Tool 1", Description: "Description 1", ToolType: ToolTypeREST, PrivacyScore: 7},
		{Name: "Tool 2", Description: "Description 2", ToolType: ToolTypeREST, PrivacyScore: 8},
		{Name: "Tool 3", Description: "Description 3", ToolType: ToolTypeREST, PrivacyScore: 9},
	}
	for _, tool := range tools {
		err := tool.Create(db)
		assert.NoError(t, err)
	}

	var fetchedTools Tools
	err := fetchedTools.GetByPrivacyScoreMin(db, 8)
	assert.NoError(t, err)
	assert.Len(t, fetchedTools, 2)
}

func TestTools_GetByPrivacyScoreMax(t *testing.T) {
	db := setupTestDB(t)

	tools := []*Tool{
		{Name: "Tool 1", Description: "Description 1", ToolType: ToolTypeREST, PrivacyScore: 7},
		{Name: "Tool 2", Description: "Description 2", ToolType: ToolTypeREST, PrivacyScore: 8},
		{Name: "Tool 3", Description: "Description 3", ToolType: ToolTypeREST, PrivacyScore: 9},
	}
	for _, tool := range tools {
		err := tool.Create(db)
		assert.NoError(t, err)
	}

	var fetchedTools Tools
	err := fetchedTools.GetByPrivacyScoreMax(db, 8)
	assert.NoError(t, err)
	assert.Len(t, fetchedTools, 2)
}

func TestTools_GetByPrivacyScoreRange(t *testing.T) {
	db := setupTestDB(t)

	tools := []*Tool{
		{Name: "Tool 1", Description: "Description 1", ToolType: ToolTypeREST, PrivacyScore: 6},
		{Name: "Tool 2", Description: "Description 2", ToolType: ToolTypeREST, PrivacyScore: 8},
		{Name: "Tool 3", Description: "Description 3", ToolType: ToolTypeREST, PrivacyScore: 10},
	}
	for _, tool := range tools {
		err := tool.Create(db)
		assert.NoError(t, err)
	}

	var fetchedTools Tools
	err := fetchedTools.GetByPrivacyScoreRange(db, 7, 8)
	assert.NoError(t, err)
	assert.Len(t, fetchedTools, 1)
	assert.Equal(t, "Tool 2", fetchedTools[0].Name)
}

func TestTools_Search(t *testing.T) {
	db := setupTestDB(t)

	tools := []*Tool{
		{Name: "REST API Tool", Description: "A tool for REST APIs", ToolType: ToolTypeREST, PrivacyScore: 7},
		{Name: "GraphQL Tool", Description: "A tool for GraphQL APIs", ToolType: "GraphQL", PrivacyScore: 8},
		{Name: "gRPC Tool", Description: "A tool for gRPC APIs", ToolType: "gRPC", PrivacyScore: 9},
	}
	for _, tool := range tools {
		err := tool.Create(db)
		assert.NoError(t, err)
	}

	var fetchedTools Tools
	err := fetchedTools.Search(db, "REST")
	assert.NoError(t, err)
	assert.Len(t, fetchedTools, 1)
	assert.Equal(t, "REST API Tool", fetchedTools[0].Name)

	err = fetchedTools.Search(db, "API")
	assert.NoError(t, err)
	assert.Len(t, fetchedTools, 3)
}

func TestTool_AddOperation(t *testing.T) {
	tool := &Tool{
		Name:                "Test Tool",
		Description:         "This is a test tool",
		ToolType:            ToolTypeREST,
		AvailableOperations: "GET,POST",
	}

	tool.AddOperation("PUT")
	assert.Equal(t, "GET,POST,PUT", tool.AvailableOperations)

	// Adding an existing operation should not change anything
	tool.AddOperation("GET")
	assert.Equal(t, "GET,POST,PUT", tool.AvailableOperations)

	// Adding to an empty list
	emptyTool := &Tool{}
	emptyTool.AddOperation("GET")
	assert.Equal(t, "GET", emptyTool.AvailableOperations)
}

func TestTool_RemoveOperation(t *testing.T) {
	tool := &Tool{
		Name:                "Test Tool",
		Description:         "This is a test tool",
		ToolType:            ToolTypeREST,
		AvailableOperations: "GET,POST,PUT",
	}

	tool.RemoveOperation("POST")
	assert.Equal(t, "GET,PUT", tool.AvailableOperations)

	// Removing a non-existent operation should not change anything
	tool.RemoveOperation("DELETE")
	assert.Equal(t, "GET,PUT", tool.AvailableOperations)

	// Removing the last operation
	tool.RemoveOperation("GET")
	tool.RemoveOperation("PUT")
	assert.Equal(t, "", tool.AvailableOperations)
}

func TestTool_GetOperations(t *testing.T) {
	tool := &Tool{
		Name:                "Test Tool",
		Description:         "This is a test tool",
		ToolType:            ToolTypeREST,
		AvailableOperations: "GET,POST,PUT",
	}

	operations := tool.GetOperations()
	assert.Equal(t, []string{"GET", "POST", "PUT"}, operations)

	// Test with empty operations
	emptyTool := &Tool{}
	emptyOperations := emptyTool.GetOperations()
	assert.Equal(t, []string{}, emptyOperations)
}

// TestTool_SlugComputedOnCreate verifies that the slug is automatically computed
// from the tool name when the tool is created (via BeforeSave hook)
func TestTool_SlugComputedOnCreate(t *testing.T) {
	db := setupTestDB(t)

	tool := &Tool{
		Name:         "My Test Tool",
		Description:  "This is a test tool",
		ToolType:     ToolTypeREST,
		PrivacyScore: 8,
	}
	err := tool.Create(db)
	assert.NoError(t, err)

	// Verify slug was computed correctly
	assert.Equal(t, "my-test-tool", tool.Slug)

	// Verify it's persisted in the database
	fetchedTool := &Tool{}
	err = fetchedTool.Get(db, tool.ID)
	assert.NoError(t, err)
	assert.Equal(t, "my-test-tool", fetchedTool.Slug)
}

// TestTool_SlugWithVersionNumber verifies that tools with version numbers
// (containing dots) get their slugs computed correctly. This is the key bug fix test.
// Previously, "FDX V6.2.0 Customer API" would fail to be looked up because:
// - The proxy used slug.Make() -> "fdx-v6-2-0-customer-api" (dots replaced with hyphens)
// - The DB query used LOWER(REPLACE(name, ' ', '-')) -> "fdx-v6.2.0-customer-api" (dots preserved)
// Now both use slug.Make() for consistency.
func TestTool_SlugWithVersionNumber(t *testing.T) {
	db := setupTestDB(t)

	testCases := []struct {
		name         string
		expectedSlug string
	}{
		{"FDX V6.2.0 Customer API", "fdx-v6-2-0-customer-api"},
		{"API v1.0.0", "api-v1-0-0"},
		{"Tool_With_Underscores", "tool_with_underscores"}, // underscores are preserved
		{"Tool.With.Dots", "tool-with-dots"},
		{"Tool   With   Multiple   Spaces", "tool-with-multiple-spaces"},
		{"UPPERCASE TOOL", "uppercase-tool"},
		{"MixedCase Tool Name", "mixedcase-tool-name"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tool := &Tool{
				Name:         tc.name,
				Description:  "Test tool",
				ToolType:     ToolTypeREST,
				PrivacyScore: 5,
			}
			err := tool.Create(db)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedSlug, tool.Slug, "Slug mismatch for tool name: %s", tc.name)

			// Verify the slug matches what slug.Make() produces (ensuring consistency)
			assert.Equal(t, slug.Make(tc.name), tool.Slug, "Slug should match slug.Make() output")
		})
	}
}

// TestTool_SlugUpdatedOnNameChange verifies that the slug is recomputed
// when the tool name is updated
func TestTool_SlugUpdatedOnNameChange(t *testing.T) {
	db := setupTestDB(t)

	tool := &Tool{
		Name:         "Original Name",
		Description:  "Test tool",
		ToolType:     ToolTypeREST,
		PrivacyScore: 5,
	}
	err := tool.Create(db)
	assert.NoError(t, err)
	assert.Equal(t, "original-name", tool.Slug)

	// Update the name
	tool.Name = "New Tool Name v2.0"
	err = tool.Update(db)
	assert.NoError(t, err)

	// Verify slug was recomputed
	assert.Equal(t, "new-tool-name-v2-0", tool.Slug)

	// Verify it's persisted in the database
	fetchedTool := &Tool{}
	err = fetchedTool.Get(db, tool.ID)
	assert.NoError(t, err)
	assert.Equal(t, "new-tool-name-v2-0", fetchedTool.Slug)
}

// TestTool_SlugLookupByColumn verifies that tools can be looked up by their slug column
func TestTool_SlugLookupByColumn(t *testing.T) {
	db := setupTestDB(t)

	// Create a tool with a version number in the name
	tool := &Tool{
		Name:         "FDX V6.2.0 Customer API",
		Description:  "Financial Data Exchange API",
		ToolType:     ToolTypeREST,
		PrivacyScore: 8,
	}
	err := tool.Create(db)
	assert.NoError(t, err)

	// Look up by slug column directly (simulating GetToolBySlug behavior)
	var fetchedTool Tool
	err = db.Where("slug = ?", "fdx-v6-2-0-customer-api").First(&fetchedTool).Error
	assert.NoError(t, err)
	assert.Equal(t, tool.ID, fetchedTool.ID)
	assert.Equal(t, tool.Name, fetchedTool.Name)

	// Verify that the lookup slug matches what slug.Make() produces
	// This is the key consistency check - the proxy uses slug.Make() to generate
	// the lookup slug, so it must match what's stored in the database
	lookupSlug := slug.Make("FDX V6.2.0 Customer API")
	assert.Equal(t, "fdx-v6-2-0-customer-api", lookupSlug)

	var foundTool Tool
	err = db.Where("slug = ?", lookupSlug).First(&foundTool).Error
	assert.NoError(t, err)
	assert.Equal(t, tool.ID, foundTool.ID)
}

// TestTool_EmptyNameSlug verifies behavior with empty tool names
func TestTool_EmptyNameSlug(t *testing.T) {
	db := setupTestDB(t)

	tool := &Tool{
		Name:         "",
		Description:  "Tool with empty name",
		ToolType:     ToolTypeREST,
		PrivacyScore: 5,
	}
	err := tool.Create(db)
	assert.NoError(t, err)
	assert.Equal(t, "", tool.Slug) // Empty name produces empty slug
}
