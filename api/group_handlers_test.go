//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestGroupEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create Group
	createGroupInput := GroupInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name           string `json:"name"`
				Members        []uint `json:"members"`
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			} `json:"attributes"`
		}{
			Type: "groups",
			Attributes: struct {
				Name           string `json:"name"`
				Members        []uint `json:"members"`
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			}{
				Name:           "Test Group",
				Members:        []uint{},
				Catalogues:     []uint{},
				DataCatalogues: []uint{},
				ToolCatalogues: []uint{},
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/groups", createGroupInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var createResponse map[string]GroupResponse
	err := json.Unmarshal(w.Body.Bytes(), &createResponse)
	assert.NoError(t, err)

	// Verify createGroup response
	assert.Equal(t, "Test Group", createResponse["data"].Attributes.Name)
	assert.Equal(t, "groups", createResponse["data"].Type)
	assert.NotEmpty(t, createResponse["data"].ID)
	// Verify empty arrays are present
	assert.Empty(t, createResponse["data"].Attributes.Users)
	assert.Empty(t, createResponse["data"].Attributes.Catalogues)
	assert.Empty(t, createResponse["data"].Attributes.DataCatalogues)
	assert.Empty(t, createResponse["data"].Attributes.ToolCatalogues)

	groupID := createResponse["data"].ID

	// Test Get Group
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var getResponse map[string]GroupResponse
	err = json.Unmarshal(w.Body.Bytes(), &getResponse)
	assert.NoError(t, err)

	// Verify getGroup response
	assert.Equal(t, groupID, getResponse["data"].ID)
	assert.Equal(t, "Test Group", getResponse["data"].Attributes.Name)
	assert.Equal(t, "groups", getResponse["data"].Type)
	// The related entities should be empty arrays but present
	assert.Empty(t, getResponse["data"].Attributes.Users)
	assert.Empty(t, getResponse["data"].Attributes.Catalogues)
	assert.Empty(t, getResponse["data"].Attributes.DataCatalogues)
	assert.Empty(t, getResponse["data"].Attributes.ToolCatalogues)

	// Test Update Group
	updateGroupInput := GroupInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name           string `json:"name"`
				Members        []uint `json:"members"`
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			} `json:"attributes"`
		}{
			Type: "groups",
			Attributes: struct {
				Name           string `json:"name"`
				Members        []uint `json:"members"`
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			}{
				Name:           "Updated Group",
				Members:        []uint{},
				Catalogues:     []uint{},
				DataCatalogues: []uint{},
				ToolCatalogues: []uint{},
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/groups/%s", groupID), updateGroupInput)
	assert.Equal(t, http.StatusOK, w.Code)

	var updateResponse map[string]GroupResponse
	err = json.Unmarshal(w.Body.Bytes(), &updateResponse)
	assert.NoError(t, err)

	// Verify updateGroup response
	assert.Equal(t, groupID, updateResponse["data"].ID)
	assert.Equal(t, "Updated Group", updateResponse["data"].Attributes.Name)
	assert.Equal(t, "groups", updateResponse["data"].Type)
	// The related entities should be empty arrays but present
	assert.Empty(t, updateResponse["data"].Attributes.Users)
	assert.Empty(t, updateResponse["data"].Attributes.Catalogues)
	assert.Empty(t, updateResponse["data"].Attributes.DataCatalogues)
	assert.Empty(t, updateResponse["data"].Attributes.ToolCatalogues)

	// Verify the group was actually updated by getting it again
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var verifyUpdateResponse map[string]GroupResponse
	err = json.Unmarshal(w.Body.Bytes(), &verifyUpdateResponse)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Group", verifyUpdateResponse["data"].Attributes.Name)

	// Test List Groups
	w = performRequest(api.router, "GET", "/api/v1/groups", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Add User to Group
	createUserInputs := []UserInput{
		{
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
					Groups               []uint `json:"groups"`
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
					Groups               []uint `json:"groups"`
				}{
					Email:                "groupuser@example.com",
					Name:                 "Group User",
					Password:             "password123",
					IsAdmin:              false,
					ShowChat:             false,
					ShowPortal:           false,
					EmailVerified:        false,
					NotificationsEnabled: false,
					AccessToSSOConfig:    false,
					Groups:               []uint{},
				},
			},
		},
	}

	userIDs := make([]uint, 0, len(createUserInputs))

	for _, userInput := range createUserInputs {
		w = performRequest(api.router, "POST", "/api/v1/users", userInput)
		assert.Equal(t, http.StatusCreated, w.Code)

		var userResponse map[string]UserResponse
		err := json.Unmarshal(w.Body.Bytes(), &userResponse)
		assert.NoError(t, err)
		userID := userResponse["data"].ID

		userIDUint, err := strconv.ParseUint(userID, 10, 32)
		assert.NoError(t, err)
		userIDs = append(userIDs, uint(userIDUint))
	}

	addUserToGroupInput := UserGroupInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "users",
			ID:   strconv.FormatUint(uint64(userIDs[0]), 10),
		},
	}

	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%s/users", groupID), addUserToGroupInput)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test List Group Users with pagination
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/users", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// Verify pagination headers are present
	assert.NotEmpty(t, w.Header().Get("X-Total-Count"))
	assert.NotEmpty(t, w.Header().Get("X-Total-Pages"))

	// Test Remove User from Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%s/users/%s", groupID, strconv.FormatUint(uint64(userIDs[0]), 10)), nil)
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
	group, err := api.service.CreateGroup("Test Group", []uint{}, []uint{}, []uint{}, []uint{})
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

func TestGroupCoreEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test completely malformed JSON for createGroup
	w := performRequest(api.router, "POST", "/api/v1/groups", []byte(`{malformed json`))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test getGroup with invalid ID
	w = performRequest(api.router, "GET", "/api/v1/groups/invalid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test getGroup with non-existent ID
	w = performRequest(api.router, "GET", "/api/v1/groups/999999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test updateGroup with invalid ID
	validUpdateGroupInput := GroupInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name           string `json:"name"`
				Members        []uint `json:"members"`
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			} `json:"attributes"`
		}{
			Type: "groups",
			Attributes: struct {
				Name           string `json:"name"`
				Members        []uint `json:"members"`
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			}{
				Name: "Updated Group",
			},
		},
	}
	w = performRequest(api.router, "PATCH", "/api/v1/groups/invalid", validUpdateGroupInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test updateGroup with non-existent ID
	w = performRequest(api.router, "PATCH", "/api/v1/groups/999999", validUpdateGroupInput)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Test malformed JSON for updateGroup
	// Create a valid group first
	createGroupInput := GroupInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name           string `json:"name"`
				Members        []uint `json:"members"`
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			} `json:"attributes"`
		}{
			Type: "groups",
			Attributes: struct {
				Name           string `json:"name"`
				Members        []uint `json:"members"`
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			}{
				Name: "Test Group For Error Tests",
			},
		},
	}
	w = performRequest(api.router, "POST", "/api/v1/groups", createGroupInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var createResponse map[string]GroupResponse
	err := json.Unmarshal(w.Body.Bytes(), &createResponse)
	assert.NoError(t, err)
	groupID := createResponse["data"].ID

	// Now test with malformed JSON
	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/groups/%s", groupID), []byte(`{malformed json`))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSerializeGroupsForList(t *testing.T) {
	_, db := setupTestAPI(t)

	// Create test groups directly in DB
	group1 := &models.Group{Name: "Group A"}
	group2 := &models.Group{Name: "Group B"}
	group3 := &models.Group{Name: "Group C"}

	err := db.Create(group1).Error
	assert.NoError(t, err)
	err = db.Create(group2).Error
	assert.NoError(t, err)
	err = db.Create(group3).Error
	assert.NoError(t, err)

	// Create member counts directly
	memberCounts := []models.GroupMemberCount{
		{GroupID: group1.ID, Count: 2},
		{GroupID: group2.ID, Count: 1},
		{GroupID: group3.ID, Count: 3},
	}

	// Create catalogues directly
	catalogue1 := &models.Catalogue{Name: "Catalogue 1"}
	catalogue2 := &models.Catalogue{Name: "Catalogue 2"}
	dataCatalogue1 := &models.DataCatalogue{Name: "Data Catalogue 1", ShortDescription: "Short Desc", LongDescription: "Long Desc", Icon: "icon.png"}
	dataCatalogue2 := &models.DataCatalogue{Name: "Data Catalogue 2", ShortDescription: "Short Desc", LongDescription: "Long Desc", Icon: "icon.png"}
	dataCatalogue3 := &models.DataCatalogue{Name: "Data Catalogue 3", ShortDescription: "Short Desc", LongDescription: "Long Desc", Icon: "icon.png"}
	toolCatalogue1 := &models.ToolCatalogue{Name: "Tool Catalogue 1", ShortDescription: "Short Desc", LongDescription: "Long Desc", Icon: "icon.png"}

	err = db.Create(catalogue1).Error
	assert.NoError(t, err)
	err = db.Create(catalogue2).Error
	assert.NoError(t, err)
	err = db.Create(dataCatalogue1).Error
	assert.NoError(t, err)
	err = db.Create(dataCatalogue2).Error
	assert.NoError(t, err)
	err = db.Create(dataCatalogue3).Error
	assert.NoError(t, err)
	err = db.Create(toolCatalogue1).Error
	assert.NoError(t, err)

	// Create associations directly
	err = db.Model(group1).Association("Catalogues").Append(catalogue1, catalogue2)
	assert.NoError(t, err)
	err = db.Model(group1).Association("DataCatalogues").Append(dataCatalogue1, dataCatalogue2)
	assert.NoError(t, err)
	err = db.Model(group1).Association("ToolCatalogues").Append(toolCatalogue1)
	assert.NoError(t, err)

	err = db.Model(group2).Association("DataCatalogues").Append(dataCatalogue3)
	assert.NoError(t, err)

	// Test the serialization
	groups := models.Groups{*group1, *group2, *group3}
	serialized := serializeGroupsForList(groups, memberCounts)

	// Check the counts match with what we set up
	assert.Len(t, serialized, 3)

	// Find group1 in serialized
	var serializedGroup1 GroupListResponse
	for _, s := range serialized {
		if s.ID == strconv.FormatUint(uint64(group1.ID), 10) {
			serializedGroup1 = s
			break
		}
	}

	// Verify group1 counts
	assert.Equal(t, 2, serializedGroup1.Attributes.UserCount)
	assert.Equal(t, 2, serializedGroup1.Attributes.CatalogueCount)
	assert.Equal(t, 2, serializedGroup1.Attributes.DataCatalogueCount)
	assert.Equal(t, 1, serializedGroup1.Attributes.ToolCatalogueCount)

	// Check the names were properly included
	assert.ElementsMatch(t, []string{"Catalogue 1", "Catalogue 2"}, serializedGroup1.Attributes.CatalogueNames)
	assert.ElementsMatch(t, []string{"Data Catalogue 1", "Data Catalogue 2"}, serializedGroup1.Attributes.DataCatalogueNames)
	assert.ElementsMatch(t, []string{"Tool Catalogue 1"}, serializedGroup1.Attributes.ToolCatalogueNames)
}

func TestUpdateGroupUsers(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create a test group
	createGroupInput := GroupInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name           string `json:"name"`
				Members        []uint `json:"members"`
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			} `json:"attributes"`
		}{
			Type: "groups",
			Attributes: struct {
				Name           string `json:"name"`
				Members        []uint `json:"members"`
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			}{
				Name:           "Test Group for updateGroupUsers",
				Members:        []uint{},
				Catalogues:     []uint{},
				DataCatalogues: []uint{},
				ToolCatalogues: []uint{},
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/groups", createGroupInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var createResponse map[string]GroupResponse
	err := json.Unmarshal(w.Body.Bytes(), &createResponse)
	assert.NoError(t, err)
	groupID := createResponse["data"].ID

	// Create test users
	createUserInputs := []UserInput{
		{
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
					Groups               []uint `json:"groups"`
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
					Groups               []uint `json:"groups"`
				}{
					Email:                "user1@example.com",
					Name:                 "User One",
					Password:             "password123",
					IsAdmin:              false,
					ShowChat:             false,
					ShowPortal:           false,
					EmailVerified:        false,
					NotificationsEnabled: false,
					AccessToSSOConfig:    false,
					Groups:               []uint{},
				},
			},
		},
		{
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
					Groups               []uint `json:"groups"`
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
					Groups               []uint `json:"groups"`
				}{
					Email:                "user2@example.com",
					Name:                 "User Two",
					Password:             "password123",
					IsAdmin:              false,
					ShowChat:             false,
					ShowPortal:           false,
					EmailVerified:        false,
					NotificationsEnabled: false,
					AccessToSSOConfig:    false,
					Groups:               []uint{},
				},
			},
		},
		{
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
					Groups               []uint `json:"groups"`
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
					Groups               []uint `json:"groups"`
				}{
					Email:                "user3@example.com",
					Name:                 "User Three",
					Password:             "password123",
					IsAdmin:              false,
					ShowChat:             false,
					ShowPortal:           false,
					EmailVerified:        false,
					NotificationsEnabled: false,
					AccessToSSOConfig:    false,
					Groups:               []uint{},
				},
			},
		},
	}

	userIDs := make([]uint, 0, len(createUserInputs))

	for _, userInput := range createUserInputs {
		w = performRequest(api.router, "POST", "/api/v1/users", userInput)
		assert.Equal(t, http.StatusCreated, w.Code)

		var userResponse map[string]UserResponse
		err := json.Unmarshal(w.Body.Bytes(), &userResponse)
		assert.NoError(t, err)
		userID := userResponse["data"].ID

		userIDUint, err := strconv.ParseUint(userID, 10, 32)
		assert.NoError(t, err)
		userIDs = append(userIDs, uint(userIDUint))
	}

	// Test adding the first two users to the group
	updateGroupUsersInput := GroupUsersInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Members []uint `json:"members"`
			} `json:"attributes"`
		}{
			Type: "group-users",
			Attributes: struct {
				Members []uint `json:"members"`
			}{
				Members: userIDs[:2], // Add first two users
			},
		},
	}

	w = performRequest(api.router, "PUT", fmt.Sprintf("/api/v1/groups/%s/users", groupID), updateGroupUsersInput)
	assert.Equal(t, http.StatusOK, w.Code)

	var updateResponse map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &updateResponse)
	assert.NoError(t, err)
	assert.Equal(t, "success", updateResponse["status"])

	// Verify the group has the first two users
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/users", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var usersResponse map[string][]UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &usersResponse)
	assert.NoError(t, err)
	assert.Len(t, usersResponse["data"], 2)

	// Test updating to only the third user
	updateGroupUsersInput = GroupUsersInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Members []uint `json:"members"`
			} `json:"attributes"`
		}{
			Type: "group-users",
			Attributes: struct {
				Members []uint `json:"members"`
			}{
				Members: userIDs[2:], // Only the third user
			},
		},
	}

	w = performRequest(api.router, "PUT", fmt.Sprintf("/api/v1/groups/%s/users", groupID), updateGroupUsersInput)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &updateResponse)
	assert.NoError(t, err)
	assert.Equal(t, "success", updateResponse["status"])

	// Verify the group has only the third user now
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/users", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &usersResponse)
	assert.NoError(t, err)
	assert.Len(t, usersResponse["data"], 1)

	// Convert the third user ID to string for comparison
	thirdUserIDStr := strconv.FormatUint(uint64(userIDs[2]), 10)
	assert.Equal(t, thirdUserIDStr, usersResponse["data"][0].ID)

	// Test error cases - invalid group ID
	w = performRequest(api.router, "PUT", "/api/v1/groups/invalid/users", updateGroupUsersInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test error cases - non-existent group ID
	w = performRequest(api.router, "PUT", "/api/v1/groups/999999/users", updateGroupUsersInput)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Test error cases - malformed request body
	w = performRequest(api.router, "PUT", fmt.Sprintf("/api/v1/groups/%s/users", groupID), []byte(`{malformed json`))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test updating with empty members list (should clear all users)
	updateGroupUsersInput = GroupUsersInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Members []uint `json:"members"`
			} `json:"attributes"`
		}{
			Type: "group-users",
			Attributes: struct {
				Members []uint `json:"members"`
			}{
				Members: []uint{}, // Empty list
			},
		},
	}

	w = performRequest(api.router, "PUT", fmt.Sprintf("/api/v1/groups/%s/users", groupID), updateGroupUsersInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify the group has no users
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/users", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &usersResponse)
	assert.NoError(t, err)
	assert.Len(t, usersResponse["data"], 0)

	// Cleanup - delete the test group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%s", groupID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestUpdateGroupCatalogues(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create a test group
	createGroupInput := GroupInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name           string `json:"name"`
				Members        []uint `json:"members"`
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			} `json:"attributes"`
		}{
			Type: "groups",
			Attributes: struct {
				Name           string `json:"name"`
				Members        []uint `json:"members"`
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			}{
				Name:           "Test Group for updateGroupCatalogues",
				Members:        []uint{},
				Catalogues:     []uint{},
				DataCatalogues: []uint{},
				ToolCatalogues: []uint{},
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/groups", createGroupInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var createResponse map[string]GroupResponse
	err := json.Unmarshal(w.Body.Bytes(), &createResponse)
	assert.NoError(t, err)
	groupID := createResponse["data"].ID

	catalogue, err := api.service.CreateCatalogue("Test Catalogue")
	assert.NoError(t, err)

	dataCatalogue, err := api.service.CreateDataCatalogue("Test Data Catalogue", "Short Desc", "Long Desc", "icon.png")
	assert.NoError(t, err)

	toolCatalogue, err := api.service.CreateToolCatalogue("Test Tool Catalogue", "Short Desc", "Long Desc", "icon.png")
	assert.NoError(t, err)

	updateGroupCataloguesInput := GroupCataloguesRequest{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			} `json:"attributes"`
		}{
			Type: "Group",
			Attributes: struct {
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			}{
				Catalogues:     []uint{catalogue.ID},
				DataCatalogues: []uint{dataCatalogue.ID},
				ToolCatalogues: []uint{toolCatalogue.ID},
			},
		},
	}

	w = performRequest(api.router, "PUT", fmt.Sprintf("/api/v1/groups/%s/catalogues", groupID), updateGroupCataloguesInput)
	assert.Equal(t, http.StatusOK, w.Code)

	var updateResponse map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &updateResponse)
	assert.NoError(t, err)
	assert.Equal(t, "success", updateResponse["status"])

	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var groupResponse map[string]GroupResponse
	err = json.Unmarshal(w.Body.Bytes(), &groupResponse)
	assert.NoError(t, err)
	assert.Len(t, groupResponse["data"].Attributes.Catalogues, 1)
	assert.Len(t, groupResponse["data"].Attributes.DataCatalogues, 1)
	assert.Len(t, groupResponse["data"].Attributes.ToolCatalogues, 1)

	catalogue2, err := api.service.CreateCatalogue("Test Catalogue 2")
	assert.NoError(t, err)

	updateGroupCataloguesInput = GroupCataloguesRequest{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			} `json:"attributes"`
		}{
			Type: "Group",
			Attributes: struct {
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			}{
				Catalogues:     []uint{catalogue2.ID},
				DataCatalogues: []uint{},
				ToolCatalogues: []uint{},
			},
		},
	}

	w = performRequest(api.router, "PUT", fmt.Sprintf("/api/v1/groups/%s/catalogues", groupID), updateGroupCataloguesInput)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &updateResponse)
	assert.NoError(t, err)
	assert.Equal(t, "success", updateResponse["status"])

	// Verify the group has only the new catalog now
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &groupResponse)
	assert.NoError(t, err)
	assert.Len(t, groupResponse["data"].Attributes.Catalogues, 1)
	assert.Equal(t, catalogue2.Name, groupResponse["data"].Attributes.Catalogues[0].Attributes.Name)
	assert.Len(t, groupResponse["data"].Attributes.DataCatalogues, 0)
	assert.Len(t, groupResponse["data"].Attributes.ToolCatalogues, 0)

	// Test error cases - invalid group ID
	w = performRequest(api.router, "PUT", "/api/v1/groups/invalid/catalogues", updateGroupCataloguesInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test error cases - non-existent group ID
	w = performRequest(api.router, "PUT", "/api/v1/groups/999999/catalogues", updateGroupCataloguesInput)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Test error cases - malformed request body
	w = performRequest(api.router, "PUT", fmt.Sprintf("/api/v1/groups/%s/catalogues", groupID), []byte(`{malformed json`))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test updating with empty catalog lists
	updateGroupCataloguesInput = GroupCataloguesRequest{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			} `json:"attributes"`
		}{
			Type: "Group",
			Attributes: struct {
				Catalogues     []uint `json:"catalogues"`
				DataCatalogues []uint `json:"data_catalogues"`
				ToolCatalogues []uint `json:"tool_catalogues"`
			}{
				Catalogues:     []uint{},
				DataCatalogues: []uint{},
				ToolCatalogues: []uint{},
			},
		},
	}

	w = performRequest(api.router, "PUT", fmt.Sprintf("/api/v1/groups/%s/catalogues", groupID), updateGroupCataloguesInput)
	assert.Equal(t, http.StatusOK, w.Code)

	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &groupResponse)
	assert.NoError(t, err)
	assert.Len(t, groupResponse["data"].Attributes.Catalogues, 0)
	assert.Len(t, groupResponse["data"].Attributes.DataCatalogues, 0)
	assert.Len(t, groupResponse["data"].Attributes.ToolCatalogues, 0)

	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%s", groupID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}
