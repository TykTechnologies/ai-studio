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
				Name           string   `json:"name"`
				Description    string   `json:"description"`
				ToolType       string   `json:"tool_type"`
				OASSpec        string   `json:"oas_spec"`
				PrivacyScore   int      `json:"privacy_score"`
				AuthKey        string   `json:"auth_key"`
				AuthSchemaName string   `json:"auth_schema_name"`
				Operations     []string `json:"operations"`
				Namespace      string   `json:"namespace"`
			} `json:"attributes"`
		}{
			Type: "tools",
			Attributes: struct {
				Name           string   `json:"name"`
				Description    string   `json:"description"`
				ToolType       string   `json:"tool_type"`
				OASSpec        string   `json:"oas_spec"`
				PrivacyScore   int      `json:"privacy_score"`
				AuthKey        string   `json:"auth_key"`
				AuthSchemaName string   `json:"auth_schema_name"`
				Operations     []string `json:"operations"`
				Namespace      string   `json:"namespace"`
			}{
				Name:         "Test Tool",
				Description:  "A test tool",
				ToolType:     models.ToolTypeREST,
				OASSpec:      `{"openapi": "3.0.0"}`,
				PrivacyScore: 8,
				Operations:   []string{"operation1", "operation2"},
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
				Name           string   `json:"name"`
				Description    string   `json:"description"`
				ToolType       string   `json:"tool_type"`
				OASSpec        string   `json:"oas_spec"`
				PrivacyScore   int      `json:"privacy_score"`
				AuthKey        string   `json:"auth_key"`
				AuthSchemaName string   `json:"auth_schema_name"`
				Operations     []string `json:"operations"`
				Namespace      string   `json:"namespace"`
			} `json:"attributes"`
		}{
			Type: "tools",
			Attributes: struct {
				Name           string   `json:"name"`
				Description    string   `json:"description"`
				ToolType       string   `json:"tool_type"`
				OASSpec        string   `json:"oas_spec"`
				PrivacyScore   int      `json:"privacy_score"`
				AuthKey        string   `json:"auth_key"`
				AuthSchemaName string   `json:"auth_schema_name"`
				Operations     []string `json:"operations"`
				Namespace      string   `json:"namespace"`
			}{
				Name:         "Updated Tool",
				Description:  "An updated test tool",
				ToolType:     models.ToolTypeREST,
				OASSpec:      `{"openapi": "3.0.1"}`,
				PrivacyScore: 9,
				Operations:   []string{"operation1", "operation2", "operation3"},
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

func TestToolAuthKeyRedaction(t *testing.T) {
	api, _ := setupTestAPI(t)

	makeInput := func(name, authKey string) ToolInput {
		return ToolInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Name           string   `json:"name"`
					Description    string   `json:"description"`
					ToolType       string   `json:"tool_type"`
					OASSpec        string   `json:"oas_spec"`
					PrivacyScore   int      `json:"privacy_score"`
					AuthKey        string   `json:"auth_key"`
					AuthSchemaName string   `json:"auth_schema_name"`
					Operations     []string `json:"operations"`
					Namespace      string   `json:"namespace"`
				} `json:"attributes"`
			}{
				Type: "tools",
				Attributes: struct {
					Name           string   `json:"name"`
					Description    string   `json:"description"`
					ToolType       string   `json:"tool_type"`
					OASSpec        string   `json:"oas_spec"`
					PrivacyScore   int      `json:"privacy_score"`
					AuthKey        string   `json:"auth_key"`
					AuthSchemaName string   `json:"auth_schema_name"`
					Operations     []string `json:"operations"`
					Namespace      string   `json:"namespace"`
				}{
					Name:     name,
					AuthKey:  authKey,
					ToolType: models.ToolTypeREST,
					OASSpec:  `{"openapi": "3.0.0"}`,
				},
			},
		}
	}

	// Create a tool with an auth key
	w := performRequest(api.router, "POST", "/api/v1/tools", makeInput("SecureTool", "super-secret-key"))
	assert.Equal(t, http.StatusCreated, w.Code)

	var createResp map[string]ToolResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	toolID := createResp["data"].ID

	// GET single tool: auth_key must be redacted, has_auth_key must be true
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tools/%s", toolID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var getResp map[string]ToolResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &getResp))
	assert.Equal(t, "[redacted]", getResp["data"].Attributes.AuthKey, "auth_key must be redacted on GET")
	assert.True(t, getResp["data"].Attributes.HasAuthKey, "has_auth_key must be true when a key is set")

	// GET list: same masking applies
	w = performRequest(api.router, "GET", "/api/v1/tools", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var listResp map[string][]ToolResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	assert.Equal(t, "[redacted]", listResp["data"][0].Attributes.AuthKey, "auth_key must be redacted in list")
	assert.True(t, listResp["data"][0].Attributes.HasAuthKey, "has_auth_key must be true in list")

	// PATCH with [redacted] must NOT overwrite the stored key
	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tools/%s", toolID), makeInput("SecureTool", "[redacted]"))
	assert.Equal(t, http.StatusOK, w.Code)
	// Re-fetch and confirm key is still set
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tools/%s", toolID), nil)
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &getResp))
	assert.True(t, getResp["data"].Attributes.HasAuthKey, "key must still be set after PATCH with [redacted]")

	// PATCH with a new key value must update it (has_auth_key stays true)
	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tools/%s", toolID), makeInput("SecureTool", "new-secret-key"))
	assert.Equal(t, http.StatusOK, w.Code)
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tools/%s", toolID), nil)
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &getResp))
	assert.Equal(t, "[redacted]", getResp["data"].Attributes.AuthKey, "new key must also be redacted")
	assert.True(t, getResp["data"].Attributes.HasAuthKey, "has_auth_key must remain true after key update")

	// PATCH with empty string must clear the key
	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tools/%s", toolID), makeInput("SecureTool", ""))
	assert.Equal(t, http.StatusOK, w.Code)
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tools/%s", toolID), nil)
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &getResp))
	assert.False(t, getResp["data"].Attributes.HasAuthKey, "has_auth_key must be false after clearing key")
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
				Name           string   `json:"name"`
				Description    string   `json:"description"`
				ToolType       string   `json:"tool_type"`
				OASSpec        string   `json:"oas_spec"`
				PrivacyScore   int      `json:"privacy_score"`
				AuthKey        string   `json:"auth_key"`
				AuthSchemaName string   `json:"auth_schema_name"`
				Operations     []string `json:"operations"`
				Namespace      string   `json:"namespace"`
			} `json:"attributes"`
		}{
			Type: "tools",
			Attributes: struct {
				Name           string   `json:"name"`
				Description    string   `json:"description"`
				ToolType       string   `json:"tool_type"`
				OASSpec        string   `json:"oas_spec"`
				PrivacyScore   int      `json:"privacy_score"`
				AuthKey        string   `json:"auth_key"`
				AuthSchemaName string   `json:"auth_schema_name"`
				Operations     []string `json:"operations"`
				Namespace      string   `json:"namespace"`
			}{
				Name:         "Updated Tool",
				Description:  "An updated test tool",
				ToolType:     models.ToolTypeREST,
				OASSpec:      `{"openapi": "3.0.1"}`,
				PrivacyScore: 9,
				Operations:   []string{"operation1", "operation2", "operation3"},
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
				Name           string   `json:"name"`
				Description    string   `json:"description"`
				ToolType       string   `json:"tool_type"`
				OASSpec        string   `json:"oas_spec"`
				PrivacyScore   int      `json:"privacy_score"`
				AuthKey        string   `json:"auth_key"`
				AuthSchemaName string   `json:"auth_schema_name"`
				Operations     []string `json:"operations"`
				Namespace      string   `json:"namespace"`
			} `json:"attributes"`
		}{
			Type: "tools",
			Attributes: struct {
				Name           string   `json:"name"`
				Description    string   `json:"description"`
				ToolType       string   `json:"tool_type"`
				OASSpec        string   `json:"oas_spec"`
				PrivacyScore   int      `json:"privacy_score"`
				AuthKey        string   `json:"auth_key"`
				AuthSchemaName string   `json:"auth_schema_name"`
				Operations     []string `json:"operations"`
				Namespace      string   `json:"namespace"`
			}{
				Name:         "",
				Description:  "",
				ToolType:     "",
				OASSpec:      "",
				PrivacyScore: -1,
				Operations:   []string{},
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
