package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create Tag
	createTagInput := TagInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "tags",
			Attributes: struct {
				Name string `json:"name"`
			}{
				Name: "Test Tag",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/tags", createTagInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]TagResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Tag", response["data"].Attributes.Name)

	tagID := response["data"].ID

	// Test Get Tag
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tags/%s", tagID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Tag
	updateTagInput := TagInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "tags",
			Attributes: struct {
				Name string `json:"name"`
			}{
				Name: "Updated Tag",
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tags/%s", tagID), updateTagInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Tags
	w = performRequest(api.router, "GET", "/api/v1/tags", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Search Tags
	w = performRequest(api.router, "GET", "/api/v1/tags/search?name=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]TagResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse["data"], 1)
	assert.Equal(t, "Updated Tag", searchResponse["data"][0].Attributes.Name)

	// Test Delete Tag
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/tags/%s", tagID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}
