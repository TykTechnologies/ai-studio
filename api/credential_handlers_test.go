package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCredentialEndpoints(t *testing.T) {
	t.Skip()
	api, _ := setupTestAPI(t)

	// Test Create Credential
	w := performRequest(api.router, "POST", "/api/v1/credentials", nil)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]CredentialResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEmpty(t, response["data"].Attributes.KeyID)
	assert.NotEmpty(t, response["data"].Attributes.Secret)
	assert.False(t, response["data"].Attributes.Active)

	credentialID := response["data"].ID

	// Test Get Credential by ID
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/credentials/%s", credentialID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Get Credential by Key ID
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/credentials/key/%s", response["data"].Attributes.KeyID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Credential
	updateCredentialInput := CredentialInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				KeyID  string `json:"key_id"`
				Secret string `json:"secret"`
				Active bool   `json:"active"`
			} `json:"attributes"`
		}{
			Type: "credentials",
			Attributes: struct {
				KeyID  string `json:"key_id"`
				Secret string `json:"secret"`
				Active bool   `json:"active"`
			}{
				Active: true,
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/credentials/%s", credentialID), updateCredentialInput)
	assert.Equal(t, http.StatusOK, w.Code)

	var updatedResponse map[string]CredentialResponse
	err = json.Unmarshal(w.Body.Bytes(), &updatedResponse)
	assert.NoError(t, err)
	assert.True(t, updatedResponse["data"].Attributes.Active)

	// Test Activate Credential
	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/credentials/%s/activate", credentialID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var activatedResponse map[string]CredentialResponse
	err = json.Unmarshal(w.Body.Bytes(), &activatedResponse)
	assert.NoError(t, err)
	assert.True(t, activatedResponse["data"].Attributes.Active)

	// Test Deactivate Credential
	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/credentials/%s/deactivate", credentialID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var deactivatedResponse map[string]CredentialResponse
	err = json.Unmarshal(w.Body.Bytes(), &deactivatedResponse)
	assert.NoError(t, err)
	assert.False(t, deactivatedResponse["data"].Attributes.Active)

	// Test List Credentials
	w = performRequest(api.router, "GET", "/api/v1/credentials", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]CredentialResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.NotEmpty(t, listResponse["data"])

	// Test List Active Credentials
	w = performRequest(api.router, "GET", "/api/v1/credentials/active", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var activeListResponse map[string][]CredentialResponse
	err = json.Unmarshal(w.Body.Bytes(), &activeListResponse)
	assert.NoError(t, err)
	// The number of active credentials may vary, so we don't assert on the length

	// Test Delete Credential
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/credentials/%s", credentialID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify credential is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/credentials/%s", credentialID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCredentialEndpoints_ErrorCases(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Get Credential with Invalid ID
	w := performRequest(api.router, "GET", "/api/v1/credentials/invalid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Get Credential with Non-existent ID
	w = performRequest(api.router, "GET", "/api/v1/credentials/999999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Get Credential by Invalid Key ID
	w = performRequest(api.router, "GET", "/api/v1/credentials/key/invalid", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Update Credential with Invalid ID
	updateCredentialInput := CredentialInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				KeyID  string `json:"key_id"`
				Secret string `json:"secret"`
				Active bool   `json:"active"`
			} `json:"attributes"`
		}{
			Type: "credentials",
			Attributes: struct {
				KeyID  string `json:"key_id"`
				Secret string `json:"secret"`
				Active bool   `json:"active"`
			}{
				Active: true,
			},
		},
	}
	w = performRequest(api.router, "PATCH", "/api/v1/credentials/invalid", updateCredentialInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Activate Credential with Invalid ID
	w = performRequest(api.router, "POST", "/api/v1/credentials/invalid/activate", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Deactivate Credential with Invalid ID
	w = performRequest(api.router, "POST", "/api/v1/credentials/invalid/deactivate", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Delete Credential with Invalid ID
	w = performRequest(api.router, "DELETE", "/api/v1/credentials/invalid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCredentialEndpoints_MultipleCredentials(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create multiple credentials
	createCredential := func() string {
		w := performRequest(api.router, "POST", "/api/v1/credentials", nil)
		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]CredentialResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		return response["data"].ID
	}

	credential1ID := createCredential()
	credential2ID := createCredential()
	credential3ID := createCredential()

	// Activate some credentials
	performRequest(api.router, "POST", fmt.Sprintf("/api/v1/credentials/%s/activate", credential1ID), nil)
	performRequest(api.router, "POST", fmt.Sprintf("/api/v1/credentials/%s/activate", credential3ID), nil)

	// Test List All Credentials
	w := performRequest(api.router, "GET", "/api/v1/credentials", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]CredentialResponse
	err := json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse["data"], 3)

	// Test List Active Credentials
	w = performRequest(api.router, "GET", "/api/v1/credentials/active", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var activeListResponse map[string][]CredentialResponse
	err = json.Unmarshal(w.Body.Bytes(), &activeListResponse)
	assert.NoError(t, err)
	assert.Len(t, activeListResponse["data"], 2)

	// Verify the correct credentials are active
	activeIDs := make([]string, len(activeListResponse["data"]))
	for i, cred := range activeListResponse["data"] {
		activeIDs[i] = cred.ID
	}
	assert.Contains(t, activeIDs, credential1ID)
	assert.Contains(t, activeIDs, credential3ID)
	assert.NotContains(t, activeIDs, credential2ID)
}
