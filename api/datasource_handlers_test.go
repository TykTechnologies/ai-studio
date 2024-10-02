package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDatasourceEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create a user for testing
	user, err := api.service.CreateUser("test@example.com", "Test User", "password123", true)
	assert.NoError(t, err)

	// Test Create Datasource
	createDatasourceInput := DatasourceInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string   `json:"name"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				Icon             string   `json:"icon"`
				Url              string   `json:"url"`
				PrivacyScore     int      `json:"privacy_score"`
				UserID           uint     `json:"user_id"`
				Tags             []string `json:"tags"`
				DBConnString     string   `json:"db_conn_string"`
				DBSourceType     string   `json:"db_source_type"`
				DBConnAPIKey     string   `json:"db_conn_api_key"`
				DBName           string   `json:"db_name"`
				EmbedVendor      string   `json:"embed_vendor"`
				EmbedUrl         string   `json:"embed_url"`
				EmbedAPIKey      string   `json:"embed_api_key"`
				EmbedModel       string   `json:"embed_model"`
				Active           bool     `json:"active"`
			} `json:"attributes"`
		}{
			Type: "datasources",
			Attributes: struct {
				Name             string   `json:"name"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				Icon             string   `json:"icon"`
				Url              string   `json:"url"`
				PrivacyScore     int      `json:"privacy_score"`
				UserID           uint     `json:"user_id"`
				Tags             []string `json:"tags"`
				DBConnString     string   `json:"db_conn_string"`
				DBSourceType     string   `json:"db_source_type"`
				DBConnAPIKey     string   `json:"db_conn_api_key"`
				DBName           string   `json:"db_name"`
				EmbedVendor      string   `json:"embed_vendor"`
				EmbedUrl         string   `json:"embed_url"`
				EmbedAPIKey      string   `json:"embed_api_key"`
				EmbedModel       string   `json:"embed_model"`
				Active           bool     `json:"active"`
			}{
				Name:             "Test Datasource",
				ShortDescription: "Short description",
				LongDescription:  "Long description",
				Icon:             "icon.png",
				Url:              "https://example.com",
				PrivacyScore:     75,
				UserID:           user.ID,
				Tags:             []string{"tag1", "tag2"},
				DBConnString:     "test_conn_string",
				DBSourceType:     "test_source_type",
				DBConnAPIKey:     "test_api_key",
				EmbedVendor:      "test_vendor",
				EmbedUrl:         "https://embed.example.com",
				EmbedAPIKey:      "test_embed_api_key",
				EmbedModel:       "test_model",
				Active:           true,
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/datasources", createDatasourceInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]DatasourceResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Datasource", response["data"].Attributes.Name)

	datasourceID := response["data"].ID

	// Test Get Datasource
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/datasources/%s", datasourceID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Datasource
	updateDatasourceInput := DatasourceInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string   `json:"name"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				Icon             string   `json:"icon"`
				Url              string   `json:"url"`
				PrivacyScore     int      `json:"privacy_score"`
				UserID           uint     `json:"user_id"`
				Tags             []string `json:"tags"`
				DBConnString     string   `json:"db_conn_string"`
				DBSourceType     string   `json:"db_source_type"`
				DBConnAPIKey     string   `json:"db_conn_api_key"`
				DBName           string   `json:"db_name"`
				EmbedVendor      string   `json:"embed_vendor"`
				EmbedUrl         string   `json:"embed_url"`
				EmbedAPIKey      string   `json:"embed_api_key"`
				EmbedModel       string   `json:"embed_model"`
				Active           bool     `json:"active"`
			} `json:"attributes"`
		}{
			Type: "datasources",
			Attributes: struct {
				Name             string   `json:"name"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				Icon             string   `json:"icon"`
				Url              string   `json:"url"`
				PrivacyScore     int      `json:"privacy_score"`
				UserID           uint     `json:"user_id"`
				Tags             []string `json:"tags"`
				DBConnString     string   `json:"db_conn_string"`
				DBSourceType     string   `json:"db_source_type"`
				DBConnAPIKey     string   `json:"db_conn_api_key"`
				DBName           string   `json:"db_name"`
				EmbedVendor      string   `json:"embed_vendor"`
				EmbedUrl         string   `json:"embed_url"`
				EmbedAPIKey      string   `json:"embed_api_key"`
				EmbedModel       string   `json:"embed_model"`
				Active           bool     `json:"active"`
			}{
				Name:             "Updated Datasource",
				ShortDescription: "Updated short description",
				LongDescription:  "Updated long description",
				Icon:             "updated-icon.png",
				Url:              "https://updated-example.com",
				PrivacyScore:     80,
				UserID:           user.ID,
				Tags:             []string{"tag1", "tag2", "tag3"},
				DBConnString:     "updated_conn_string",
				DBSourceType:     "updated_source_type",
				DBConnAPIKey:     "updated_api_key",
				EmbedVendor:      "updated_vendor",
				EmbedUrl:         "https://updated-embed.example.com",
				EmbedAPIKey:      "updated_embed_api_key",
				EmbedModel:       "updated_model",
				Active:           false,
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/datasources/%s", datasourceID), updateDatasourceInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Datasources
	w = performRequest(api.router, "GET", "/api/v1/datasources", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Search Datasources
	w = performRequest(api.router, "GET", "/api/v1/datasources/search?query=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]DatasourceResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse["data"], 1)
	assert.Equal(t, "Updated Datasource", searchResponse["data"][0].Attributes.Name)

	// Test Get Datasources by Tag
	w = performRequest(api.router, "GET", "/api/v1/datasources/by-tag?tag=tag1", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var tagResponse map[string][]DatasourceResponse
	err = json.Unmarshal(w.Body.Bytes(), &tagResponse)
	assert.NoError(t, err)
	assert.Len(t, tagResponse["data"], 1)
	assert.Equal(t, "Updated Datasource", tagResponse["data"][0].Attributes.Name)

	// Test Delete Datasource
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/datasources/%s", datasourceID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}
