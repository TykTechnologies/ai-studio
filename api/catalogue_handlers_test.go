package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
)

func TestCatalogueEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create Catalogue
	createCatalogueInput := CatalogueInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "catalogues",
			Attributes: struct {
				Name string `json:"name"`
			}{
				Name: "Test Catalogue",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/catalogues", createCatalogueInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]CatalogueResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Catalogue", response["data"].Attributes.Name)

	catalogueID := response["data"].ID

	// Test Get Catalogue
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/catalogues/%s", catalogueID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Catalogue
	updateCatalogueInput := CatalogueInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "catalogues",
			Attributes: struct {
				Name string `json:"name"`
			}{
				Name: "Updated Catalogue",
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/catalogues/%s", catalogueID), updateCatalogueInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Catalogues
	w = performRequest(api.router, "GET", "/api/v1/catalogues", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Search Catalogues
	w = performRequest(api.router, "GET", "/api/v1/catalogues/search?name=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]CatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse["data"], 1)
	assert.Equal(t, "Updated Catalogue", searchResponse["data"][0].Attributes.Name)

	// Test Add LLM to Catalogue
	createLLMInput := LLMInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string   `json:"name"`
				APIKey           string   `json:"api_key"`
				APIEndpoint      string   `json:"api_endpoint"`
				PrivacyScore     int      `json:"privacy_score"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				LogoURL          string   `json:"logo_url"`
				Vendor           string   `json:"vendor"`
				Active           bool     `json:"active"`
				Filters          []uint   `json:"filters"`
				DefaultModel     string   `json:"default_model"`
				AllowedModels    []string `json:"allowed_models"`
				MonthlyBudget    *float64 `json:"monthly_budget"`
				BudgetStartDate  *string  `json:"budget_start_date"`
			} `json:"attributes"`
		}{
			Type: "llms",
			Attributes: struct {
				Name             string   `json:"name"`
				APIKey           string   `json:"api_key"`
				APIEndpoint      string   `json:"api_endpoint"`
				PrivacyScore     int      `json:"privacy_score"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				LogoURL          string   `json:"logo_url"`
				Vendor           string   `json:"vendor"`
				Active           bool     `json:"active"`
				Filters          []uint   `json:"filters"`
				DefaultModel     string   `json:"default_model"`
				AllowedModels    []string `json:"allowed_models"`
				MonthlyBudget    *float64 `json:"monthly_budget"`
				BudgetStartDate  *string  `json:"budget_start_date"`
			}{
				Name:             "Test LLM",
				APIKey:           "test-api-key",
				APIEndpoint:      "https://api.test.com",
				PrivacyScore:     75,
				ShortDescription: "A test LLM",
				LongDescription:  "This is a test LLM for API testing",
				LogoURL:          "https://testllm.com/logo.png",
				Vendor:           "Test Vendor",
			},
		},
	}

	w = performRequest(api.router, "POST", "/api/v1/llms", createLLMInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var llmResponse map[string]LLMResponse
	err = json.Unmarshal(w.Body.Bytes(), &llmResponse)
	assert.NoError(t, err)
	llmID := llmResponse["data"].ID

	addLLMToCatalogueInput := CatalogueLLMInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "llms",
			ID:   llmID,
		},
	}

	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/catalogues/%s/llms", catalogueID), addLLMToCatalogueInput)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test List Catalogue LLMs
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/catalogues/%s/llms", catalogueID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var llmsResponse map[string][]LLMResponse
	err = json.Unmarshal(w.Body.Bytes(), &llmsResponse)
	assert.NoError(t, err)
	assert.Len(t, llmsResponse["data"], 1)
	assert.Equal(t, "Test LLM", llmsResponse["data"][0].Attributes.Name)

	// Test Remove LLM from Catalogue
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/catalogues/%s/llms/%s", catalogueID, llmID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify LLM is removed
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/catalogues/%s/llms", catalogueID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &llmsResponse)
	assert.NoError(t, err)
	assert.Len(t, llmsResponse["data"], 0)

	// Test Delete Catalogue
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/catalogues/%s", catalogueID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify catalogue is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/catalogues/%s", catalogueID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGroupCatalogueAssociation(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create a group
	group, err := api.service.CreateGroup("Test Group", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)

	// Create a catalogue
	catalogue, err := api.service.CreateCatalogue("Test Catalogue")
	assert.NoError(t, err)

	// Test Add Catalogue to Group
	addCatalogueInput := GroupCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "catalogues",
			ID:   strconv.FormatUint(uint64(catalogue.ID), 10),
		},
	}

	w := performRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%d/catalogues", group.ID), addCatalogueInput)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test List Group Catalogues
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%d/catalogues", group.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var cataloguesResponse map[string][]CatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &cataloguesResponse)
	assert.NoError(t, err)
	assert.Len(t, cataloguesResponse["data"], 1)
	assert.Equal(t, catalogue.Name, cataloguesResponse["data"][0].Attributes.Name)

	// Test Remove Catalogue from Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%d/catalogues/%d", group.ID, catalogue.ID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify catalogue is removed
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%d/catalogues", group.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &cataloguesResponse)
	assert.NoError(t, err)
	assert.Len(t, cataloguesResponse["data"], 0)
}

func TestUserAccessibleCatalogues(t *testing.T) {
	api, _ := setupTestAPI(t)

	user, err := api.service.CreateUser(services.UserDTO{
		Email:                "test@example.com",
		Name:                 "Test User",
		Password:             "password123",
		IsAdmin:              false,
		ShowChat:             true,
		ShowPortal:           true,
		EmailVerified:        true,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
		Groups:               []uint{},
	})
	assert.NoError(t, err)

	// Create a group
	group, err := api.service.CreateGroup("Test Group", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)

	// Add user to group
	err = api.service.AddUserToGroup(user.ID, group.ID)
	assert.NoError(t, err)

	// Create a catalogue
	catalogue, err := api.service.CreateCatalogue("Test Catalogue")
	assert.NoError(t, err)

	// Add catalogue to group
	err = api.service.AddCatalogueToGroup(catalogue.ID, group.ID)
	assert.NoError(t, err)

	// Test Get User Accessible Catalogues
	w := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%d/catalogues", user.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]UserAccessibleCataloguesResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"].Attributes.Catalogues, 1)
	assert.Equal(t, catalogue.Name, response["data"].Attributes.Catalogues[0].Attributes.Name)
}
