package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelPriceEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create ModelPrice
	createModelPriceInput := ModelPriceInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				ModelName    string  `json:"model_name"`
				Vendor       string  `json:"vendor"`
				CPT          float64 `json:"cpt"`
				CPIT         float64 `json:"cpit"`
				CacheWritePT float64 `json:"cache_write_pt"`
				CacheReadPT  float64 `json:"cache_read_pt"`
				Currency     string  `json:"currency"`
			} `json:"attributes"`
		}{
			Type: "model-prices",
			Attributes: struct {
				ModelName    string  `json:"model_name"`
				Vendor       string  `json:"vendor"`
				CPT          float64 `json:"cpt"`
				CPIT         float64 `json:"cpit"`
				CacheWritePT float64 `json:"cache_write_pt"`
				CacheReadPT  float64 `json:"cache_read_pt"`
				Currency     string  `json:"currency"`
			}{
				ModelName:    "GPT-3",
				Vendor:       "OpenAI",
				CPT:          0.002,
				CPIT:         0.001,
				CacheWritePT: 0.0005,
				CacheReadPT:  0.0001,
				Currency:     "USD",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/model-prices", createModelPriceInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]ModelPriceResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "GPT-3", response["data"].Attributes.ModelName)
	assert.Equal(t, "OpenAI", response["data"].Attributes.Vendor)
	assert.Equal(t, 0.002, response["data"].Attributes.CPT)
	assert.Equal(t, 0.0005, response["data"].Attributes.CacheWritePT)
	assert.Equal(t, 0.0001, response["data"].Attributes.CacheReadPT)

	modelPriceID := response["data"].ID

	// Test Get ModelPrice
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/model-prices/%s", modelPriceID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update ModelPrice
	updateModelPriceInput := ModelPriceInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				ModelName    string  `json:"model_name"`
				Vendor       string  `json:"vendor"`
				CPT          float64 `json:"cpt"`
				CPIT         float64 `json:"cpit"`
				CacheWritePT float64 `json:"cache_write_pt"`
				CacheReadPT  float64 `json:"cache_read_pt"`
				Currency     string  `json:"currency"`
			} `json:"attributes"`
		}{
			Type: "model-prices",
			Attributes: struct {
				ModelName    string  `json:"model_name"`
				Vendor       string  `json:"vendor"`
				CPT          float64 `json:"cpt"`
				CPIT         float64 `json:"cpit"`
				CacheWritePT float64 `json:"cache_write_pt"`
				CacheReadPT  float64 `json:"cache_read_pt"`
				Currency     string  `json:"currency"`
			}{
				ModelName:    "GPT-3",
				Vendor:       "OpenAI",
				CPT:          0.003,
				CPIT:         0.0015,
				CacheWritePT: 0.0007,
				CacheReadPT:  0.0002,
				Currency:     "USD",
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/model-prices/%s", modelPriceID), updateModelPriceInput)
	assert.Equal(t, http.StatusOK, w.Code)

	var updateResponse map[string]ModelPriceResponse
	err = json.Unmarshal(w.Body.Bytes(), &updateResponse)
	assert.NoError(t, err)
	assert.Equal(t, 0.003, updateResponse["data"].Attributes.CPT)
	assert.Equal(t, 0.0007, updateResponse["data"].Attributes.CacheWritePT)
	assert.Equal(t, 0.0002, updateResponse["data"].Attributes.CacheReadPT)

	// Test List ModelPrices
	w = performRequest(api.router, "GET", "/api/v1/model-prices", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]ModelPriceResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse["data"], 1)
	assert.Equal(t, "GPT-3", listResponse["data"][0].Attributes.ModelName)

	// Test Get ModelPrices by Vendor
	w = performRequest(api.router, "GET", "/api/v1/model-prices/by-vendor?vendor=OpenAI", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var vendorResponse map[string][]ModelPriceResponse
	err = json.Unmarshal(w.Body.Bytes(), &vendorResponse)
	assert.NoError(t, err)
	assert.Len(t, vendorResponse["data"], 1)
	assert.Equal(t, "OpenAI", vendorResponse["data"][0].Attributes.Vendor)

	// Test Delete ModelPrice
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/model-prices/%s", modelPriceID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify ModelPrice is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/model-prices/%s", modelPriceID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestModelPriceEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Get non-existent ModelPrice
	w := performRequest(api.router, "GET", "/api/v1/model-prices/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Update non-existent ModelPrice
	updateModelPriceInput := ModelPriceInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				ModelName    string  `json:"model_name"`
				Vendor       string  `json:"vendor"`
				CPT          float64 `json:"cpt"`
				CPIT         float64 `json:"cpit"`
				CacheWritePT float64 `json:"cache_write_pt"`
				CacheReadPT  float64 `json:"cache_read_pt"`
				Currency     string  `json:"currency"`
			} `json:"attributes"`
		}{
			Type: "model-prices",
			Attributes: struct {
				ModelName    string  `json:"model_name"`
				Vendor       string  `json:"vendor"`
				CPT          float64 `json:"cpt"`
				CPIT         float64 `json:"cpit"`
				CacheWritePT float64 `json:"cache_write_pt"`
				CacheReadPT  float64 `json:"cache_read_pt"`
				Currency     string  `json:"currency"`
			}{
				ModelName:    "GPT-3",
				Vendor:       "OpenAI",
				CPT:          0.003,
				CPIT:         0.0015,
				CacheWritePT: 0.0007,
				CacheReadPT:  0.0002,
				Currency:     "USD",
			},
		},
	}
	w = performRequest(api.router, "PATCH", "/api/v1/model-prices/999", updateModelPriceInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Delete non-existent ModelPrice
	w = performRequest(api.router, "DELETE", "/api/v1/model-prices/999", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Test Create ModelPrice with invalid input
	invalidCreateModelPriceInput := ModelPriceInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				ModelName    string  `json:"model_name"`
				Vendor       string  `json:"vendor"`
				CPT          float64 `json:"cpt"`
				CPIT         float64 `json:"cpit"`
				CacheWritePT float64 `json:"cache_write_pt"`
				CacheReadPT  float64 `json:"cache_read_pt"`
				Currency     string  `json:"currency"`
			} `json:"attributes"`
		}{
			Type: "model-prices",
			Attributes: struct {
				ModelName    string  `json:"model_name"`
				Vendor       string  `json:"vendor"`
				CPT          float64 `json:"cpt"`
				CPIT         float64 `json:"cpit"`
				CacheWritePT float64 `json:"cache_write_pt"`
				CacheReadPT  float64 `json:"cache_read_pt"`
				Currency     string  `json:"currency"`
			}{
				ModelName:    "",
				Vendor:       "",
				CPT:          -1,
				CPIT:         0,
				CacheWritePT: 0,
				CacheReadPT:  0,
				Currency:     "USD",
			},
		},
	}
	w = performRequest(api.router, "POST", "/api/v1/model-prices", invalidCreateModelPriceInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Get ModelPrices by non-existent vendor
	w = performRequest(api.router, "GET", "/api/v1/model-prices/by-vendor?vendor=NonExistentVendor", nil)
	assert.Equal(t, http.StatusOK, w.Code) // This should return an empty list, not an error

	var emptyResponse map[string][]ModelPriceResponse
	err := json.Unmarshal(w.Body.Bytes(), &emptyResponse)
	assert.NoError(t, err)
	assert.Len(t, emptyResponse["data"], 0)
}

func TestGetOrCreateModelPriceByName(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test getting non-existent model price (should create new)
	w := performRequest(api.router, "GET", "/api/v1/model-prices/by-name?model_name=GPT-4", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]ModelPriceResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "GPT-4", response["data"].Attributes.ModelName)
	assert.Equal(t, 0.0, response["data"].Attributes.CPT)
	assert.Equal(t, 0.0, response["data"].Attributes.CPIT)
	assert.Equal(t, 0.0, response["data"].Attributes.CacheWritePT)
	assert.Equal(t, 0.0, response["data"].Attributes.CacheReadPT)
	assert.Equal(t, "USD", response["data"].Attributes.Currency)

	// Test getting existing model price
	w = performRequest(api.router, "GET", "/api/v1/model-prices/by-name?model_name=GPT-4", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var existingResponse map[string]ModelPriceResponse
	err = json.Unmarshal(w.Body.Bytes(), &existingResponse)
	assert.NoError(t, err)
	assert.Equal(t, response["data"].ID, existingResponse["data"].ID) // Should return same model price

	// Test with missing model name
	w = performRequest(api.router, "GET", "/api/v1/model-prices/by-name", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
