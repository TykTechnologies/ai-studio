package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestToolEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create Tool
	createToolInput := ToolInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name         string `json:"name"`
				Description  string `json:"description"`
				ToolType     string `json:"tool_type"`
				OASSpec      string `json:"oas_spec"`
				PrivacyScore int    `json:"privacy_score"`

				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			} `json:"attributes"`
		}{
			Type: "tools",
			Attributes: struct {
				Name         string `json:"name"`
				Description  string `json:"description"`
				ToolType     string `json:"tool_type"`
				OASSpec      string `json:"oas_spec"`
				PrivacyScore int    `json:"privacy_score"`

				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			}{
				Name:         "Test Tool",
				Description:  "A test tool",
				ToolType:     models.ToolTypeREST,
				OASSpec:      `{"openapi": "3.0.0"}`,
				PrivacyScore: 8,
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/tools", createToolInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]ToolResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Tool", response["data"].Attributes.Name)

	toolID := response["data"].ID

	// Test Get Tool
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tools/%s", toolID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Tool
	updateToolInput := ToolInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name         string `json:"name"`
				Description  string `json:"description"`
				ToolType     string `json:"tool_type"`
				OASSpec      string `json:"oas_spec"`
				PrivacyScore int    `json:"privacy_score"`

				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			} `json:"attributes"`
		}{
			Type: "tools",
			Attributes: struct {
				Name         string `json:"name"`
				Description  string `json:"description"`
				ToolType     string `json:"tool_type"`
				OASSpec      string `json:"oas_spec"`
				PrivacyScore int    `json:"privacy_score"`

				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			}{
				Name:         "Updated Tool",
				Description:  "An updated test tool",
				ToolType:     models.ToolTypeREST,
				OASSpec:      `{"openapi": "3.0.1"}`,
				PrivacyScore: 9,
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tools/%s", toolID), updateToolInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Tools
	w = performRequest(api.router, "GET", "/api/v1/tools", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]ToolResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse["data"], 1)
	assert.Equal(t, "Updated Tool", listResponse["data"][0].Attributes.Name)

	// Test Get Tools by Type
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tools/by-type?type=%s", models.ToolTypeREST), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var typeResponse map[string][]ToolResponse
	err = json.Unmarshal(w.Body.Bytes(), &typeResponse)
	assert.NoError(t, err)
	assert.Len(t, typeResponse["data"], 1)
	assert.Equal(t, "Updated Tool", typeResponse["data"][0].Attributes.Name)

	// Test Search Tools
	w = performRequest(api.router, "GET", "/api/v1/tools/search?query=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]ToolResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse["data"], 1)
	assert.Equal(t, "Updated Tool", searchResponse["data"][0].Attributes.Name)

	// Test Delete Tool
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/tools/%s", toolID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify tool is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tools/%s", toolID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestToolEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Get non-existent tool
	w := performRequest(api.router, "GET", "/api/v1/tools/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Update non-existent tool
	updateToolInput := ToolInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name           string `json:"name"`
				Description    string `json:"description"`
				ToolType       string `json:"tool_type"`
				OASSpec        string `json:"oas_spec"`
				PrivacyScore   int    `json:"privacy_score"`
				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			} `json:"attributes"`
		}{
			Type: "tools",
			Attributes: struct {
				Name           string `json:"name"`
				Description    string `json:"description"`
				ToolType       string `json:"tool_type"`
				OASSpec        string `json:"oas_spec"`
				PrivacyScore   int    `json:"privacy_score"`
				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			}{
				Name:         "Updated Tool",
				Description:  "An updated test tool",
				ToolType:     models.ToolTypeREST,
				OASSpec:      `{"openapi": "3.0.1"}`,
				PrivacyScore: 9,
			},
		},
	}
	w = performRequest(api.router, "PATCH", "/api/v1/tools/999", updateToolInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Delete non-existent tool
	w = performRequest(api.router, "DELETE", "/api/v1/tools/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Create tool with invalid input
	invalidCreateToolInput := ToolInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name         string `json:"name"`
				Description  string `json:"description"`
				ToolType     string `json:"tool_type"`
				OASSpec      string `json:"oas_spec"`
				PrivacyScore int    `json:"privacy_score"`

				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			} `json:"attributes"`
		}{
			Type: "tools",
			Attributes: struct {
				Name         string `json:"name"`
				Description  string `json:"description"`
				ToolType     string `json:"tool_type"`
				OASSpec      string `json:"oas_spec"`
				PrivacyScore int    `json:"privacy_score"`

				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			}{
				Name:         "",
				Description:  "",
				ToolType:     "",
				OASSpec:      "",
				PrivacyScore: -1,
			},
		},
	}
	w = performRequest(api.router, "POST", "/api/v1/tools", invalidCreateToolInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Get tools by invalid type
	w = performRequest(api.router, "GET", "/api/v1/tools/by-type?type=INVALID_TYPE", nil)
	assert.Equal(t, http.StatusOK, w.Code) // This should return an empty list, not an error

	var emptyResponse map[string][]ToolResponse
	err := json.Unmarshal(w.Body.Bytes(), &emptyResponse)
	assert.NoError(t, err)
	assert.Len(t, emptyResponse["data"], 0)
}
