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
	fmt.Println(w.Body.String())
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
