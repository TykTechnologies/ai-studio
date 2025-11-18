//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToolCatalogueEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create ToolCatalogue
	createInput := ToolCatalogueInput{
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
				ShortDescription: "Short description",
				LongDescription:  "Long description",
				Icon:             "icon.png",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/tool-catalogues", createInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var createResponse map[string]ToolCatalogueResponse
	err := json.Unmarshal(w.Body.Bytes(), &createResponse)
	assert.NoError(t, err)
	assert.Contains(t, createResponse, "data")

	catalogueID := createResponse["data"].ID

	// Test Get ToolCatalogue
	w = performRequest(api.router, "GET", "/api/v1/tool-catalogues/"+catalogueID, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var getResponse map[string]ToolCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &getResponse)
	assert.NoError(t, err)
	assert.Contains(t, getResponse, "data")
	assert.Equal(t, "Test Tool Catalogue", getResponse["data"].Attributes.Name)

	// Test List ToolCatalogues
	w = performRequest(api.router, "GET", "/api/v1/tool-catalogues", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update ToolCatalogue
	updateInput := ToolCatalogueInput{
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
				ShortDescription: "Updated short description",
				LongDescription:  "Updated long description",
				Icon:             "updated-icon.png",
			},
		},
	}

	w = performRequest(api.router, "PATCH", "/api/v1/tool-catalogues/"+catalogueID, updateInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Delete ToolCatalogue
	w = performRequest(api.router, "DELETE", "/api/v1/tool-catalogues/"+catalogueID, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify deletion
	w = performRequest(api.router, "GET", "/api/v1/tool-catalogues/"+catalogueID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetOperationDetailFromSpec(t *testing.T) {
	// Test the getOperationDetailFromSpec function
	testSpec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0.0"},
		"servers": [{"url": "https://example.com"}],
		"paths": {
			"/test": {
				"get": {
					"operationId": "testGetOperation",
					"summary": "Test GET operation",
					"description": "This is a test GET operation",
					"parameters": [
						{
							"name": "testParam",
							"in": "query",
							"required": true,
							"schema": {"type": "string"}
						}
					]
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
	assert.Equal(t, "POST", operationDetailPut.Method)
	assert.Equal(t, "/update", operationDetailPut.Path)
}
