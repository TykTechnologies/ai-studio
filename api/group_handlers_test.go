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

	// Test List Group Users with pagination
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/users", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// Verify pagination headers are present
	assert.NotEmpty(t, w.Header().Get("X-Total-Count"))
	assert.NotEmpty(t, w.Header().Get("X-Total-Pages"))

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
	err = db.Model(group1).Association("DataCatalogues").Append(dataCatalogue1)
	assert.NoError(t, err)
	err = db.Model(group1).Association("ToolCatalogues").Append(toolCatalogue1)
	assert.NoError(t, err)

	err = db.Model(group2).Association("DataCatalogues").Append(dataCatalogue2, dataCatalogue3)
	assert.NoError(t, err)

	// Reload groups with associations
	var groups models.Groups
	err = db.Preload("Catalogues").Preload("DataCatalogues").Preload("ToolCatalogues").Find(&groups).Error
	assert.NoError(t, err)

	// Test cases
	tests := []struct {
		name         string
		groups       models.Groups
		memberCounts []models.GroupMemberCount
		expected     []GroupListResponse
	}{
		{
			name:         "empty groups list",
			groups:       models.Groups{},
			memberCounts: []models.GroupMemberCount{},
			expected:     []GroupListResponse{},
		},
		{
			name:         "multiple groups with varying members and catalogues",
			groups:       groups,
			memberCounts: memberCounts,
			expected: []GroupListResponse{
				{
					Type: "groups",
					ID:   strconv.FormatUint(uint64(group1.ID), 10),
					Attributes: struct {
						Name               string   `json:"name"`
						UserCount          int      `json:"user_count"`
						CatalogueCount     int      `json:"catalogue_count"`
						DataCatalogueCount int      `json:"data_catalogue_count"`
						ToolCatalogueCount int      `json:"tool_catalogue_count"`
						CatalogueNames     []string `json:"catalogue_names"`
						DataCatalogueNames []string `json:"data_catalogue_names"`
						ToolCatalogueNames []string `json:"tool_catalogue_names"`
					}{
						Name:               "Group A",
						UserCount:          2,
						CatalogueCount:     2,
						DataCatalogueCount: 1,
						ToolCatalogueCount: 1,
						CatalogueNames:     []string{"Catalogue 1", "Catalogue 2"},
						DataCatalogueNames: []string{"Data Catalogue 1"},
						ToolCatalogueNames: []string{"Tool Catalogue 1"},
					},
				},
				{
					Type: "groups",
					ID:   strconv.FormatUint(uint64(group2.ID), 10),
					Attributes: struct {
						Name               string   `json:"name"`
						UserCount          int      `json:"user_count"`
						CatalogueCount     int      `json:"catalogue_count"`
						DataCatalogueCount int      `json:"data_catalogue_count"`
						ToolCatalogueCount int      `json:"tool_catalogue_count"`
						CatalogueNames     []string `json:"catalogue_names"`
						DataCatalogueNames []string `json:"data_catalogue_names"`
						ToolCatalogueNames []string `json:"tool_catalogue_names"`
					}{
						Name:               "Group B",
						UserCount:          1,
						CatalogueCount:     0,
						DataCatalogueCount: 2,
						ToolCatalogueCount: 0,
						CatalogueNames:     []string{},
						DataCatalogueNames: []string{"Data Catalogue 2", "Data Catalogue 3"},
						ToolCatalogueNames: []string{},
					},
				},
				{
					Type: "groups",
					ID:   strconv.FormatUint(uint64(group3.ID), 10),
					Attributes: struct {
						Name               string   `json:"name"`
						UserCount          int      `json:"user_count"`
						CatalogueCount     int      `json:"catalogue_count"`
						DataCatalogueCount int      `json:"data_catalogue_count"`
						ToolCatalogueCount int      `json:"tool_catalogue_count"`
						CatalogueNames     []string `json:"catalogue_names"`
						DataCatalogueNames []string `json:"data_catalogue_names"`
						ToolCatalogueNames []string `json:"tool_catalogue_names"`
					}{
						Name:               "Group C",
						UserCount:          3,
						CatalogueCount:     0,
						DataCatalogueCount: 0,
						ToolCatalogueCount: 0,
						CatalogueNames:     []string{},
						DataCatalogueNames: []string{},
						ToolCatalogueNames: []string{},
					},
				},
			},
		},
		{
			name: "group with no members or catalogues",
			groups: models.Groups{
				{
					ID:   4,
					Name: "Group D",
				},
			},
			memberCounts: []models.GroupMemberCount{
				{GroupID: 4, Count: 0},
			},
			expected: []GroupListResponse{
				{
					Type: "groups",
					ID:   "4",
					Attributes: struct {
						Name               string   `json:"name"`
						UserCount          int      `json:"user_count"`
						CatalogueCount     int      `json:"catalogue_count"`
						DataCatalogueCount int      `json:"data_catalogue_count"`
						ToolCatalogueCount int      `json:"tool_catalogue_count"`
						CatalogueNames     []string `json:"catalogue_names"`
						DataCatalogueNames []string `json:"data_catalogue_names"`
						ToolCatalogueNames []string `json:"tool_catalogue_names"`
					}{
						Name:               "Group D",
						UserCount:          0,
						CatalogueCount:     0,
						DataCatalogueCount: 0,
						ToolCatalogueCount: 0,
						CatalogueNames:     []string{},
						DataCatalogueNames: []string{},
						ToolCatalogueNames: []string{},
					},
				},
			},
		},
	}

	// Run test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := serializeGroupsForList(tt.groups, tt.memberCounts)

			// Check if both slices have same length
			assert.Equal(t, len(tt.expected), len(result))

			// For empty group list, just check equality
			if len(tt.expected) == 0 {
				assert.Equal(t, tt.expected, result)
				return
			}

			// For each expected group, find matching group in result by ID and verify its properties
			for _, expected := range tt.expected {
				var found bool
				for _, actual := range result {
					if expected.ID == actual.ID {
						found = true
						// Check non-slice fields for exact equality
						assert.Equal(t, expected.Type, actual.Type)
						assert.Equal(t, expected.Attributes.Name, actual.Attributes.Name)
						assert.Equal(t, expected.Attributes.UserCount, actual.Attributes.UserCount)
						assert.Equal(t, expected.Attributes.CatalogueCount, actual.Attributes.CatalogueCount)
						assert.Equal(t, expected.Attributes.DataCatalogueCount, actual.Attributes.DataCatalogueCount)
						assert.Equal(t, expected.Attributes.ToolCatalogueCount, actual.Attributes.ToolCatalogueCount)
						assert.Equal(t, expected.Attributes.CatalogueNames, actual.Attributes.CatalogueNames)
						assert.Equal(t, expected.Attributes.DataCatalogueNames, actual.Attributes.DataCatalogueNames)
						assert.Equal(t, expected.Attributes.ToolCatalogueNames, actual.Attributes.ToolCatalogueNames)
						break
					}
				}
				assert.True(t, found, "Expected group with ID %s not found in result", expected.ID)
			}
		})
	}
}
