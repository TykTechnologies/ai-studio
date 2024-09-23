package models

import (
	"testing"

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
