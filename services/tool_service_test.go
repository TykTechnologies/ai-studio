package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gosimple/slug"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForTools(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestToolService(t *testing.T) {
	db := setupTestDBForTools(t)
	service := NewService(db)

	// Test CreateTool
	tool, err := service.CreateTool("Test Tool", "Description", models.ToolTypeREST, "OAS Spec", 8, "apiKey", "secret")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.NotZero(t, tool.ID)

	// Test GetToolByID
	fetchedTool, err := service.GetToolByID(tool.ID)
	assert.NoError(t, err)
	assert.Equal(t, tool.ID, fetchedTool.ID)
	assert.Equal(t, tool.Name, fetchedTool.Name)
	assert.Equal(t, tool.Description, fetchedTool.Description)
	assert.Equal(t, tool.ToolType, fetchedTool.ToolType)
	assert.Equal(t, tool.PrivacyScore, fetchedTool.PrivacyScore)

	// Test UpdateTool
	updatedTool, err := service.UpdateTool(tool.ID, "Updated Tool", "Updated Description", models.ToolTypeREST, "Updated OAS Spec", 9, "updatedApiKey", "updatedSecret")
	assert.NoError(t, err)
	assert.Equal(t, tool.ID, updatedTool.ID)
	assert.Equal(t, "Updated Tool", updatedTool.Name)
	assert.Equal(t, "Updated Description", updatedTool.Description)
	assert.Equal(t, 9, updatedTool.PrivacyScore)

	// Test GetToolByName
	namedTool, err := service.GetToolByName("Updated Tool")
	assert.NoError(t, err)
	assert.Equal(t, tool.ID, namedTool.ID)

	// Test GetAllTools
	allTools, _, _, err := service.GetAllTools(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, allTools, 1)
	assert.Equal(t, tool.ID, allTools[0].ID)

	// Test GetToolsByType
	typedTools, err := service.GetToolsByType(models.ToolTypeREST)
	assert.NoError(t, err)
	assert.Len(t, typedTools, 1)
	assert.Equal(t, tool.ID, typedTools[0].ID)

	// Test GetToolsByPrivacyScoreMin
	minScoreTools, err := service.GetToolsByPrivacyScoreMin(8)
	assert.NoError(t, err)
	assert.Len(t, minScoreTools, 1)
	assert.Equal(t, tool.ID, minScoreTools[0].ID)

	// Test GetToolsByPrivacyScoreMax
	maxScoreTools, err := service.GetToolsByPrivacyScoreMax(9)
	assert.NoError(t, err)
	assert.Len(t, maxScoreTools, 1)
	assert.Equal(t, tool.ID, maxScoreTools[0].ID)

	// Test GetToolsByPrivacyScoreRange
	rangeScoreTools, err := service.GetToolsByPrivacyScoreRange(8, 10)
	assert.NoError(t, err)
	assert.Len(t, rangeScoreTools, 1)
	assert.Equal(t, tool.ID, rangeScoreTools[0].ID)

	// Test SearchTools
	searchResults, err := service.SearchTools("Updated")
	assert.NoError(t, err)
	assert.Len(t, searchResults, 1)
	assert.Equal(t, tool.ID, searchResults[0].ID)

	// Test AddOperationToTool
	err = service.AddOperationToTool(tool.ID, "GET")
	assert.NoError(t, err)

	// Test GetToolOperations
	operations, err := service.GetToolOperations(tool.ID)
	assert.NoError(t, err)
	assert.Len(t, operations, 1)
	assert.Equal(t, "GET", operations[0])

	// Test RemoveOperationFromTool
	err = service.RemoveOperationFromTool(tool.ID, "GET")
	assert.NoError(t, err)

	operations, err = service.GetToolOperations(tool.ID)
	assert.NoError(t, err)
	assert.Len(t, operations, 0)

	// Test DeleteTool
	err = service.DeleteTool(tool.ID)
	assert.NoError(t, err)

	// Verify tool is deleted
	_, err = service.GetToolByID(tool.ID)
	assert.Error(t, err)
}

func TestToolServiceErrorCases(t *testing.T) {
	db := setupTestDBForTools(t)
	service := NewService(db)

	// Test GetToolByID with non-existent ID
	_, err := service.GetToolByID(9999)
	assert.Error(t, err)

	// Test UpdateTool with non-existent ID
	_, err = service.UpdateTool(9999, "Non-existent Tool", "Description", models.ToolTypeREST, "OAS Spec", 8, "authKey", "authValue")
	assert.Error(t, err)

	// Test GetToolByName with non-existent name
	_, err = service.GetToolByName("Non-existent Tool")
	assert.Error(t, err)

	// Test DeleteTool with non-existent ID
	err = service.DeleteTool(9999)
	assert.Error(t, err)

	// Test AddOperationToTool with non-existent ID
	err = service.AddOperationToTool(9999, "GET")
	assert.Error(t, err)

	// Test RemoveOperationFromTool with non-existent ID
	err = service.RemoveOperationFromTool(9999, "GET")
	assert.Error(t, err)

	// Test GetToolOperations with non-existent ID
	_, err = service.GetToolOperations(9999)
	assert.Error(t, err)
}

func TestToolService_MultipleTool(t *testing.T) {
	db := setupTestDBForTools(t)
	service := NewService(db)

	// Create multiple tools
	tool1, _ := service.CreateTool("Tool 1", "Description 1", models.ToolTypeREST, "OAS Spec 1", 7, "authKey", "authValue")
	tool2, _ := service.CreateTool("Tool 2", "Description 2", models.ToolTypeREST, "OAS Spec 2", 8, "authKey", "authValue")
	tool3, _ := service.CreateTool("Tool 3", "Description 3", "GraphQL", "OAS Spec 3", 9, "authKey", "authValue")

	// Test GetAllTools
	allTools, _, _, err := service.GetAllTools(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, allTools, 3)
	assert.ElementsMatch(t, []uint{tool1.ID, tool2.ID, tool3.ID}, []uint{allTools[0].ID, allTools[1].ID, allTools[2].ID})

	// Test GetToolsByType
	restTools, err := service.GetToolsByType(models.ToolTypeREST)
	assert.NoError(t, err)
	assert.Len(t, restTools, 2)
	assert.ElementsMatch(t, []uint{tool1.ID, tool2.ID}, []uint{restTools[0].ID, restTools[1].ID})

	// Test GetToolsByPrivacyScoreMin
	minScoreTools, err := service.GetToolsByPrivacyScoreMin(8)
	assert.NoError(t, err)
	assert.Len(t, minScoreTools, 2)
	assert.ElementsMatch(t, []uint{tool2.ID, tool3.ID}, []uint{minScoreTools[0].ID, minScoreTools[1].ID})

	// Test GetToolsByPrivacyScoreMax
	maxScoreTools, err := service.GetToolsByPrivacyScoreMax(8)
	assert.NoError(t, err)
	assert.Len(t, maxScoreTools, 2)
	assert.ElementsMatch(t, []uint{tool1.ID, tool2.ID}, []uint{maxScoreTools[0].ID, maxScoreTools[1].ID})

	// Test GetToolsByPrivacyScoreRange
	rangeScoreTools, err := service.GetToolsByPrivacyScoreRange(8, 8)
	assert.NoError(t, err)
	assert.Len(t, rangeScoreTools, 1)
	assert.Equal(t, tool2.ID, rangeScoreTools[0].ID)

	// Test SearchTools
	searchResults, err := service.SearchTools("Description")
	assert.NoError(t, err)
	assert.Len(t, searchResults, 3)

	// Test operations on multiple tools
	err = service.AddOperationToTool(tool1.ID, "GET")
	assert.NoError(t, err)
	err = service.AddOperationToTool(tool1.ID, "POST")
	assert.NoError(t, err)
	err = service.AddOperationToTool(tool2.ID, "GET")
	assert.NoError(t, err)

	ops1, _ := service.GetToolOperations(tool1.ID)
	ops2, _ := service.GetToolOperations(tool2.ID)
	ops3, _ := service.GetToolOperations(tool3.ID)

	assert.Len(t, ops1, 2)
	assert.Len(t, ops2, 1)
	assert.Len(t, ops3, 0)

	assert.ElementsMatch(t, []string{"GET", "POST"}, ops1)
	assert.Equal(t, "GET", ops2[0])
}

// TestToolService_GetToolBySlug tests the GetToolBySlug function
func TestToolService_GetToolBySlug(t *testing.T) {
	db := setupTestDBForTools(t)
	service := NewService(db)

	// Create a tool
	tool, err := service.CreateTool("Test Tool", "Description", models.ToolTypeREST, "OAS Spec", 8, "apiKey", "secret")
	assert.NoError(t, err)

	// Look up by slug
	fetchedTool, err := service.GetToolBySlug("test-tool")
	assert.NoError(t, err)
	assert.Equal(t, tool.ID, fetchedTool.ID)
	assert.Equal(t, tool.Name, fetchedTool.Name)
}

// TestToolService_GetToolBySlug_WithVersionNumber is the key test that verifies the bug fix.
// Previously, tools with version numbers (dots) in their names couldn't be found because:
// - The proxy used slug.Make() which replaces dots with hyphens: "fdx-v6-2-0-customer-api"
// - The DB query used LOWER(REPLACE(name, ' ', '-')) which preserved dots: "fdx-v6.2.0-customer-api"
// Now both use slug.Make() for consistency.
func TestToolService_GetToolBySlug_WithVersionNumber(t *testing.T) {
	db := setupTestDBForTools(t)
	service := NewService(db)

	testCases := []struct {
		toolName     string
		lookupSlug   string
		description  string
	}{
		{
			toolName:    "FDX V6.2.0 Customer API",
			lookupSlug:  "fdx-v6-2-0-customer-api",
			description: "Tool with version number containing dots",
		},
		{
			toolName:    "API v1.0.0",
			lookupSlug:  "api-v1-0-0",
			description: "API with semantic version",
		},
		{
			toolName:    "Tool.With.Dots",
			lookupSlug:  "tool-with-dots",
			description: "Tool name with dots instead of spaces",
		},
		{
			toolName:    "OpenAPI Spec v2.1",
			lookupSlug:  "openapi-spec-v2-1",
			description: "Tool with spaces and version",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// Create the tool
			tool, err := service.CreateTool(tc.toolName, "Test description", models.ToolTypeREST, "OAS Spec", 5, "apiKey", "secret")
			assert.NoError(t, err)

			// Verify the stored slug matches what we expect
			assert.Equal(t, tc.lookupSlug, tool.Slug, "Stored slug should match expected slug")

			// Verify the stored slug matches what slug.Make() produces
			// This is critical because the proxy uses slug.Make() to generate lookup slugs
			assert.Equal(t, slug.Make(tc.toolName), tool.Slug, "Stored slug should match slug.Make() output")

			// Look up by the same slug that the proxy would use
			proxySlug := slug.Make(tc.toolName)
			fetchedTool, err := service.GetToolBySlug(proxySlug)
			assert.NoError(t, err, "Should be able to find tool by slug generated with slug.Make()")
			assert.Equal(t, tool.ID, fetchedTool.ID)
			assert.Equal(t, tool.Name, fetchedTool.Name)

			// Also verify direct lookup works
			fetchedTool2, err := service.GetToolBySlug(tc.lookupSlug)
			assert.NoError(t, err)
			assert.Equal(t, tool.ID, fetchedTool2.ID)
		})
	}
}

// TestToolService_GetToolBySlug_NotFound tests that GetToolBySlug returns an error
// when the tool doesn't exist
func TestToolService_GetToolBySlug_NotFound(t *testing.T) {
	db := setupTestDBForTools(t)
	service := NewService(db)

	// Try to find a non-existent tool
	_, err := service.GetToolBySlug("non-existent-tool")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool not found")
}

// TestToolService_GetToolBySlug_AfterNameUpdate verifies that GetToolBySlug
// works correctly after a tool's name is updated
func TestToolService_GetToolBySlug_AfterNameUpdate(t *testing.T) {
	db := setupTestDBForTools(t)
	service := NewService(db)

	// Create a tool
	tool, err := service.CreateTool("Original Name", "Description", models.ToolTypeREST, "OAS Spec", 8, "apiKey", "secret")
	assert.NoError(t, err)
	assert.Equal(t, "original-name", tool.Slug)

	// Verify original slug lookup works
	fetchedTool, err := service.GetToolBySlug("original-name")
	assert.NoError(t, err)
	assert.Equal(t, tool.ID, fetchedTool.ID)

	// Update the tool name
	updatedTool, err := service.UpdateTool(tool.ID, "New Name v2.0", "Updated Description", models.ToolTypeREST, "Updated OAS Spec", 9, "apiKey", "secret")
	assert.NoError(t, err)
	assert.Equal(t, "new-name-v2-0", updatedTool.Slug)

	// Verify new slug lookup works
	fetchedTool2, err := service.GetToolBySlug("new-name-v2-0")
	assert.NoError(t, err)
	assert.Equal(t, tool.ID, fetchedTool2.ID)

	// Verify old slug no longer works
	_, err = service.GetToolBySlug("original-name")
	assert.Error(t, err)
}
