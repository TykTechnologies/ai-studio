package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToolCatalogueEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create ToolCatalogue
	createToolCatalogueInput := ToolCatalogueInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string `json:"name"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				Icon             string `json:"icon"`
			} `json:"attributes"`
		}{
			Type: "tool-catalogues",
			Attributes: struct {
				Name             string `json:"name"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				Icon             string `json:"icon"`
			}{
				Name:             "Test Tool Catalogue",
				ShortDescription: "A test tool catalogue",
				LongDescription:  "This is a test tool catalogue for API testing",
				Icon:             "test-icon.png",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/tool-catalogues", createToolCatalogueInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response ToolCatalogueResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Tool Catalogue", response.Attributes.Name)

	toolCatalogueID := response.ID

	// Test Get ToolCatalogue
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tool-catalogues/%s", toolCatalogueID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update ToolCatalogue
	updateToolCatalogueInput := ToolCatalogueInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string `json:"name"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				Icon             string `json:"icon"`
			} `json:"attributes"`
		}{
			Type: "tool-catalogues",
			Attributes: struct {
				Name             string `json:"name"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				Icon             string `json:"icon"`
			}{
				Name:             "Updated Tool Catalogue",
				ShortDescription: "An updated test tool catalogue",
				LongDescription:  "This is an updated test tool catalogue for API testing",
				Icon:             "updated-icon.png",
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tool-catalogues/%s", toolCatalogueID), updateToolCatalogueInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List ToolCatalogues
	w = performRequest(api.router, "GET", "/api/v1/tool-catalogues", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse []ToolCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse, 1)
	assert.Equal(t, "Updated Tool Catalogue", listResponse[0].Attributes.Name)

	// Test Search ToolCatalogues
	w = performRequest(api.router, "GET", "/api/v1/tool-catalogues/search?query=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse []ToolCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse, 1)
	assert.Equal(t, "Updated Tool Catalogue", searchResponse[0].Attributes.Name)

	// Test Delete ToolCatalogue
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/tool-catalogues/%s", toolCatalogueID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify tool catalogue is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tool-catalogues/%s", toolCatalogueID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestToolCatalogueEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Get non-existent tool catalogue
	w := performRequest(api.router, "GET", "/api/v1/tool-catalogues/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Update non-existent tool catalogue
	updateToolCatalogueInput := ToolCatalogueInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string `json:"name"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				Icon             string `json:"icon"`
			} `json:"attributes"`
		}{
			Type: "tool-catalogues",
			Attributes: struct {
				Name             string `json:"name"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				Icon             string `json:"icon"`
			}{
				Name: "Updated Tool Catalogue",
			},
		},
	}
	w = performRequest(api.router, "PATCH", "/api/v1/tool-catalogues/999", updateToolCatalogueInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Delete non-existent tool catalogue
	w = performRequest(api.router, "DELETE", "/api/v1/tool-catalogues/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Create tool catalogue with invalid input
	invalidCreateToolCatalogueInput := ToolCatalogueInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string `json:"name"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				Icon             string `json:"icon"`
			} `json:"attributes"`
		}{
			Type: "tool-catalogues",
			Attributes: struct {
				Name             string `json:"name"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				Icon             string `json:"icon"`
			}{
				Name: "",
			},
		},
	}
	w = performRequest(api.router, "POST", "/api/v1/tool-catalogues", invalidCreateToolCatalogueInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Search ToolCatalogues with empty query
	w = performRequest(api.router, "GET", "/api/v1/tool-catalogues/search", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetOperationDetailFromSpec(t *testing.T) {
	// Test the getOperationDetailFromSpec function to ensure method is always "POST"
	// This tests our change to always show POST method regardless of original OpenAPI method

	// Simple OpenAPI spec with GET operation for testing
	testSpec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0.0"},
		"servers": [{"url": "https://example.com"}],
		"paths": {
			"/test": {
				"get": {
					"operationId": "testGetOperation",
					"summary": "Test GET operation",
					"description": "This is a test GET operation"
				}
			}
		}
	}`

	operationDetail, err := getOperationDetailFromSpec([]byte(testSpec), "testGetOperation")
	assert.NoError(t, err)
	assert.Equal(t, "testGetOperation", operationDetail.OperationID)
	assert.Equal(t, "POST", operationDetail.Method) // Should always be POST, not GET
	assert.Equal(t, "/test", operationDetail.Path)
	assert.Equal(t, "This is a test GET operation", operationDetail.Description)

	// Test with another method - PUT should also become POST
	testSpecPut := `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0.0"},
		"servers": [{"url": "https://example.com"}],
		"paths": {
			"/update": {
				"put": {
					"operationId": "testPutOperation",
					"summary": "Test PUT operation",
					"description": "This is a test PUT operation"
				}
			}
		}
	}`

	operationDetailPut, err := getOperationDetailFromSpec([]byte(testSpecPut), "testPutOperation")
	assert.NoError(t, err)
	assert.Equal(t, "testPutOperation", operationDetailPut.OperationID)
	assert.Equal(t, "POST", operationDetailPut.Method) // Should always be POST, not PUT
	assert.Equal(t, "/update", operationDetailPut.Path)
	assert.Equal(t, "This is a test PUT operation", operationDetailPut.Description)
}
