package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create Filter
	createFilterInput := FilterInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Script      []byte `json:"script"`
			} `json:"attributes"`
		}{
			Type: "filters",
			Attributes: struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Script      []byte `json:"script"`
			}{
				Name:        "Test Filter",
				Description: "A test filter",
				Script:      []byte("function testFilter() { return true; }"),
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/filters", createFilterInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response FilterResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Filter", response.Attributes.Name)

	filterID := response.ID

	// Test Get Filter
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/filters/%s", filterID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Filter
	updateFilterInput := FilterInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Script      []byte `json:"script"`
			} `json:"attributes"`
		}{
			Type: "filters",
			Attributes: struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Script      []byte `json:"script"`
			}{
				Name:        "Updated Filter",
				Description: "An updated test filter",
				Script:      []byte("function updatedTestFilter() { return false; }"),
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/filters/%s", filterID), updateFilterInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Filters
	w = performRequest(api.router, "GET", "/api/v1/filters", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse []FilterResponse
	fmt.Println(w.Body.String())
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse, 1)
	assert.Equal(t, "Updated Filter", listResponse[0].Attributes.Name)

	// Test Delete Filter
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/filters/%s", filterID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify filter is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/filters/%s", filterID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFilterEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Get non-existent filter
	w := performRequest(api.router, "GET", "/api/v1/filters/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Update non-existent filter
	updateFilterInput := FilterInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Script      []byte `json:"script"`
			} `json:"attributes"`
		}{
			Type: "filters",
			Attributes: struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Script      []byte `json:"script"`
			}{
				Name:        "Updated Filter",
				Description: "An updated test filter",
				Script:      []byte("function updatedTestFilter() { return false; }"),
			},
		},
	}
	w = performRequest(api.router, "PATCH", "/api/v1/filters/999", updateFilterInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Delete non-existent filter
	w = performRequest(api.router, "DELETE", "/api/v1/filters/999", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Test Create filter with invalid input
	invalidCreateFilterInput := FilterInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Script      []byte `json:"script"`
			} `json:"attributes"`
		}{
			Type: "filters",
			Attributes: struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Script      []byte `json:"script"`
			}{
				Name:        "",
				Description: "",
				Script:      []byte{},
			},
		},
	}
	w = performRequest(api.router, "POST", "/api/v1/filters", invalidCreateFilterInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

}
