package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroupEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create Group
	createGroupInput := GroupInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "groups",
			Attributes: struct {
				Name string `json:"name"`
			}{
				Name: "Test Group",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/groups", createGroupInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]GroupResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Group", response["data"].Attributes.Name)

	groupID := response["data"].ID

	// Test Get Group
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Group
	updateGroupInput := GroupInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "groups",
			Attributes: struct {
				Name string `json:"name"`
			}{
				Name: "Updated Group",
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/groups/%s", groupID), updateGroupInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Groups
	w = performRequest(api.router, "GET", "/api/v1/groups", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Add User to Group
	createUserInput := UserInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Email                string `json:"email"`
				Name                 string `json:"name"`
				Password             string `json:"password,omitempty"`
				IsAdmin              bool   `json:"is_admin"`
				ShowChat             bool   `json:"show_chat"`
				ShowPortal           bool   `json:"show_portal"`
				EmailVerified        bool   `json:"email_verified"`
				NotificationsEnabled bool   `json:"notifications_enabled"`
				AccessToSSOConfig    bool   `json:"access_to_sso_config"`
			} `json:"attributes"`
		}{
			Type: "users",
			Attributes: struct {
				Email                string `json:"email"`
				Name                 string `json:"name"`
				Password             string `json:"password,omitempty"`
				IsAdmin              bool   `json:"is_admin"`
				ShowChat             bool   `json:"show_chat"`
				ShowPortal           bool   `json:"show_portal"`
				EmailVerified        bool   `json:"email_verified"`
				NotificationsEnabled bool   `json:"notifications_enabled"`
				AccessToSSOConfig    bool   `json:"access_to_sso_config"`
			}{
				Email:             "groupuser@example.com",
				Name:              "Group User",
				Password:          "password123",
				IsAdmin:           false,
				ShowChat:          false,
				ShowPortal:        false,
				EmailVerified:     false,
				AccessToSSOConfig: false,
			},
		},
	}

	w = performRequest(api.router, "POST", "/api/v1/users", createUserInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var userResponse map[string]UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &userResponse)
	assert.NoError(t, err)
	userID := userResponse["data"].ID

	addUserToGroupInput := UserGroupInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "users",
			ID:   userID,
		},
	}

	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%s/users", groupID), addUserToGroupInput)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test List Group Users
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/users", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Remove User from Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%s/users/%s", groupID, userID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test Add DataCatalogue to Group
	dataCatalogue, err := api.service.CreateDataCatalogue("Test Data Catalogue", "Short Desc", "Long Desc", "icon.png")
	assert.NoError(t, err)

	addDataCatalogueInput := GroupDataCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "data-catalogues",
			ID:   strconv.FormatUint(uint64(dataCatalogue.ID), 10),
		},
	}

	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%s/data-catalogues", groupID), addDataCatalogueInput)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test List Group DataCatalogues
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/data-catalogues", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var dataCataloguesResponse map[string][]DataCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &dataCataloguesResponse)
	assert.NoError(t, err)
	assert.Len(t, dataCataloguesResponse["data"], 1)
	assert.Equal(t, dataCatalogue.Name, dataCataloguesResponse["data"][0].Attributes.Name)

	// Test Remove DataCatalogue from Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%s/data-catalogues/%d", groupID, dataCatalogue.ID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify DataCatalogue is removed
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/data-catalogues", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &dataCataloguesResponse)
	assert.NoError(t, err)
	assert.Len(t, dataCataloguesResponse["data"], 0)

	// Test Add ToolCatalogue to Group
	toolCatalogue, err := api.service.CreateToolCatalogue("Test Tool Catalogue", "Short Desc", "Long Desc", "icon.png")
	assert.NoError(t, err)

	addToolCatalogueInput := GroupToolCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "tool-catalogues",
			ID:   strconv.FormatUint(uint64(toolCatalogue.ID), 10),
		},
	}

	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%s/tool-catalogues", groupID), addToolCatalogueInput)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test List Group ToolCatalogues
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/tool-catalogues", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var toolCataloguesResponse map[string][]ToolCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &toolCataloguesResponse)
	assert.NoError(t, err)
	assert.Len(t, toolCataloguesResponse["data"], 1)
	assert.Equal(t, toolCatalogue.Name, toolCataloguesResponse["data"][0].Attributes.Name)

	// Test Remove ToolCatalogue from Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%s/tool-catalogues/%d", groupID, toolCatalogue.ID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify ToolCatalogue is removed
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/tool-catalogues", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &toolCataloguesResponse)
	assert.NoError(t, err)
	assert.Len(t, toolCataloguesResponse["data"], 0)

	// Test Delete Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%s", groupID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestGroupEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Add DataCatalogue to non-existent Group
	addDataCatalogueInput := GroupDataCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "data-catalogues",
			ID:   "1",
		},
	}
	w := performRequest(api.router, "POST", "/api/v1/groups/999/data-catalogues", addDataCatalogueInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Remove DataCatalogue from non-existent Group
	w = performRequest(api.router, "DELETE", "/api/v1/groups/999/data-catalogues/1", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Add ToolCatalogue to non-existent Group
	addToolCatalogueInput := GroupToolCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "tool-catalogues",
			ID:   "1",
		},
	}
	w = performRequest(api.router, "POST", "/api/v1/groups/999/tool-catalogues", addToolCatalogueInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Remove ToolCatalogue from non-existent Group
	w = performRequest(api.router, "DELETE", "/api/v1/groups/999/tool-catalogues/1", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Create a valid group for further testing
	group, err := api.service.CreateGroup("Test Group")
	assert.NoError(t, err)

	// Test Add non-existent DataCatalogue to Group
	addDataCatalogueInput = GroupDataCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "data-catalogues",
			ID:   "999",
		},
	}
	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%d/data-catalogues", group.ID), addDataCatalogueInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Remove non-existent DataCatalogue from Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%d/data-catalogues/999", group.ID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Add non-existent ToolCatalogue to Group
	addToolCatalogueInput = GroupToolCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "tool-catalogues",
			ID:   "999",
		},
	}
	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%d/tool-catalogues", group.ID), addToolCatalogueInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Remove non-existent ToolCatalogue from Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%d/tool-catalogues/999", group.ID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
