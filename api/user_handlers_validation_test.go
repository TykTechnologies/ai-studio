package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression: POST /api/v1/users with a flat JSON body (no JSON:API envelope)
// used to return 201 and create a user with an empty email and name.
func TestCreateUser_RejectsFlatBody(t *testing.T) {
	api, _, _ := setupTestAPIWithAdminUser(t)
	db := api.service.DB

	var before int64
	require.NoError(t, db.Model(&models.User{}).Count(&before).Error)

	flat := map[string]interface{}{
		"email":    "flat@example.com",
		"name":     "Flat Body",
		"password": "password123",
	}
	w := performRequest(api.router, "POST", "/api/v1/users", flat)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var after int64
	require.NoError(t, db.Model(&models.User{}).Count(&after).Error)
	assert.Equal(t, before, after, "no user row must be created from a flat body")

	var empty int64
	require.NoError(t, db.Model(&models.User{}).Where("email = ''").Count(&empty).Error)
	assert.Zero(t, empty, "no user with an empty email may exist")
}

func TestCreateUser_RequiresEmailAndName(t *testing.T) {
	api, _, _ := setupTestAPIWithAdminUser(t)

	cases := map[string]map[string]interface{}{
		"missing email": {"name": "No Email", "password": "password123"},
		"missing name":  {"email": "noname@example.com", "password": "password123"},
		"bad email":     {"email": "not-an-email", "name": "Bad", "password": "password123"},
	}
	for name, attrs := range cases {
		t.Run(name, func(t *testing.T) {
			body := map[string]interface{}{"data": map[string]interface{}{"type": "users", "attributes": attrs}}
			w := performRequest(api.router, "POST", "/api/v1/users", body)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
}

// PATCH with a flat body must not blank the user's email and name.
func TestUpdateUser_RejectsFlatBody(t *testing.T) {
	api, _, admin := setupTestAPIWithAdminUser(t)
	db := api.service.DB

	flat := map[string]interface{}{"name": "Renamed"}
	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/users/%d", admin.ID), flat)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var fresh models.User
	require.NoError(t, db.First(&fresh, admin.ID).Error)
	assert.Equal(t, admin.Email, fresh.Email)
	assert.Equal(t, admin.Name, fresh.Name)
}
