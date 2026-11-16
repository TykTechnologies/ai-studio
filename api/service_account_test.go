package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

func serviceAccountBody(attrs map[string]any) map[string]any {
	a := map[string]any{"name": "Tyk Operator", "account_type": models.AccountTypeService}
	for k, v := range attrs {
		a[k] = v
	}
	return map[string]any{"data": map[string]any{"type": "users", "attributes": a}}
}

// createServiceAccountViaAPI creates a service account as admin and returns
// the created user and the key shown in the response.
func createServiceAccountViaAPI(t *testing.T, api *API, admin *models.User, attrs map[string]any) (*models.User, string) {
	t.Helper()
	w := apitest.PerformAuthRequest(api.router, "POST", "/api/v1/users", serviceAccountBody(attrs), admin.APIKey)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct{ Data UserResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	id, err := strconv.ParseUint(resp.Data.ID, 10, 64)
	require.NoError(t, err)
	user, err := api.service.GetUserByID(uint(id))
	require.NoError(t, err)
	return user, resp.Data.Attributes.APIKey
}

func TestServiceAccountAPI_CreateReturnsTheKeyOnce(t *testing.T) {
	api, admin, _ := setupAuthzAPI(t)

	user, key := createServiceAccountViaAPI(t, api, admin, nil)
	assert.True(t, user.IsServiceAccount())
	assert.NotEmpty(t, key, "the create response carries the key")
	assert.Equal(t, user.APIKey, key)
	assert.True(t, strings.HasSuffix(user.Email, "@"+models.ServiceAccountEmailDomain), user.Email)
	assert.Empty(t, user.Password)

	w := apitest.PerformAuthRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%d", user.ID), nil, admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct{ Data UserResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, models.AccountTypeService, resp.Data.Attributes.AccountType)
	assert.Equal(t, models.RoleServiceAccount, resp.Data.Attributes.Role)
	assert.Empty(t, resp.Data.Attributes.APIKey, "the key is shown once, at creation (.45)")
	assert.Equal(t, key[len(key)-4:], resp.Data.Attributes.APIKeyHint)

	w = apitest.PerformAuthRequest(api.router, "GET", "/api/v1/users?all=true", nil, admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), key, "no list carries a key")
	assert.NotContains(t, w.Body.String(), admin.APIKey, "not even the caller's own")
}

func TestServiceAccountAPI_CreateRefusals(t *testing.T) {
	api, admin, _ := setupAuthzAPI(t)
	for name, attrs := range map[string]map[string]any{
		"email":       {"email": "ops@example.com"},
		"password":    {"password": "Sup3r-secret-pw!"},
		"show_portal": {"show_portal": true},
		"show_chat":   {"show_chat": true},
	} {
		t.Run(name, func(t *testing.T) {
			w := apitest.PerformAuthRequest(api.router, "POST", "/api/v1/users", serviceAccountBody(attrs), admin.APIKey)
			assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
			assert.Equal(t, services.ServiceAccountCode, decodeErrorCode(t, w.Body.Bytes()))
		})
	}
	var n int64
	require.NoError(t, api.service.DB.Model(&models.User{}).Where("account_type = ?", models.AccountTypeService).Count(&n).Error)
	assert.Zero(t, n)
}

func TestServiceAccountAPI_ImmutableFields(t *testing.T) {
	api, admin, _ := setupAuthzAPI(t)
	svc, _ := createServiceAccountViaAPI(t, api, admin, nil)
	path := fmt.Sprintf("/api/v1/users/%d", svc.ID)
	patch := func(attrs map[string]any) map[string]any {
		return map[string]any{"data": map[string]any{"type": "users", "attributes": attrs}}
	}

	for name, attrs := range map[string]map[string]any{
		"account_type": {"account_type": "x"},
		"email":        {"email": "ops@example.com"},
		"password":     {"password": "Sup3r-secret-pw!"},
		"show_portal":  {"show_portal": true},
	} {
		t.Run(name, func(t *testing.T) {
			w := apitest.PerformAuthRequest(api.router, "PATCH", path, patch(attrs), admin.APIKey)
			assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
		})
	}
	stored, err := api.service.GetUserByID(svc.ID)
	require.NoError(t, err)
	assert.Equal(t, svc.Email, stored.Email)
	assert.True(t, stored.IsServiceAccount())
	assert.Empty(t, stored.Password)

	// A rename goes through.
	w := apitest.PerformAuthRequest(api.router, "PATCH", path, patch(map[string]any{"name": "Operator 2"}), admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// A person cannot become a service account.
	person := createTestUserWithSettings(t, api.service, "person@example.com", "Person", false, true, true, true, false)
	w = apitest.PerformAuthRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/users/%d", person.ID),
		patch(map[string]any{"account_type": models.AccountTypeService}), admin.APIKey)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
}

// The key reaches the management API with the account's own access (in
// Community the admin flag); Portal, Chat, portal analytics and the OAuth
// authorization flow answer 403 service_account; /common/me, /common/nav
// and the self-service key routes stay open.
func TestServiceAccountAPI_GateOutsideTheManagementAPI(t *testing.T) {
	api, admin, _ := setupAuthzAPI(t)
	_, key := createServiceAccountViaAPI(t, api, admin, map[string]any{"is_admin": true})

	w := apitest.PerformAuthRequest(api.router, "GET", "/api/v1/llms", nil, key)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// (skip_entitlements: the test service has no group access service.)
	for _, path := range []string{"/common/me?skip_entitlements=true", "/common/nav"} {
		w := apitest.PerformAuthRequest(api.router, "GET", path, nil, key)
		assert.Equal(t, http.StatusOK, w.Code, "%s: %s", path, w.Body.String())
	}

	for _, req := range []struct{ method, path string }{
		{"GET", "/common/apps"},
		{"POST", "/common/apps"},
		{"GET", "/common/catalog"},
		{"GET", "/common/history"},
		{"GET", "/common/agents"},
		{"GET", "/common/api/v1/notifications"},
		{"GET", "/common/me/preferences"},
		{"GET", "/analytics/token-usage-and-cost-for-app?app_id=1"},
		{"GET", "/analytics/budget-usage-for-app?app_id=1"},
		{"GET", "/oauth/authorize?client_id=x&response_type=code"},
		{"GET", "/oauth/consent_details?auth_req_id=x"},
		{"POST", "/oauth/submit_consent"},
	} {
		w := apitest.PerformAuthRequest(api.router, req.method, req.path, nil, key)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s: %s", req.method, req.path, w.Body.String())
		if w.Code == http.StatusForbidden {
			assert.Equal(t, services.ServiceAccountCode, decodeErrorCode(t, w.Body.Bytes()), "%s %s", req.method, req.path)
		}
	}

	// Self-service key roll works, and the new key is the one that works.
	w = apitest.PerformAuthRequest(api.router, "POST", "/common/me/api-key/roll", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w2 := apitest.PerformAuthRequest(api.router, "GET", "/api/v1/llms", nil, key)
	assert.Equal(t, http.StatusUnauthorized, w2.Code, "the old key stops working")

	// A person is unaffected by the gate.
	w = apitest.PerformAuthRequest(api.router, "GET", "/common/apps", nil, admin.APIKey)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestServiceAccountAPI_NoTeams(t *testing.T) {
	api, admin, _ := setupAuthzAPI(t)
	svc, _ := createServiceAccountViaAPI(t, api, admin, nil)
	group, err := models.GetOrCreateDefaultGroup(api.service.DB)
	require.NoError(t, err)

	// Team management routes are Enterprise (402 in Community); the
	// service layer refusal they reach is tested in services.
	w := apitest.PerformAuthRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%d/users", group.ID),
		map[string]any{"data": map[string]any{"type": "users", "id": fmt.Sprint(svc.ID)}}, admin.APIKey)
	if w.Code != http.StatusPaymentRequired {
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	}

	w = apitest.PerformAuthRequest(api.router, "POST", "/api/v1/users",
		serviceAccountBody(map[string]any{"groups": []uint{group.ID}}), admin.APIKey)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
}

// role_ids is checked before the user is created: a refusal leaves no user.
func TestUserAPI_RoleIDsRefusedBeforeCreate(t *testing.T) {
	api, admin, _ := setupAuthzAPI(t)
	if api.service.Authz().Enabled() {
		t.Skip("Community Edition behavior; the Enterprise case is in service_account_enterprise_test.go")
	}
	body := map[string]any{"data": map[string]any{"type": "users", "attributes": map[string]any{
		"email": "new@example.com", "name": "New", "password": "Sup3r-secret-pw!", "role_ids": []uint{1},
	}}}
	w := apitest.PerformAuthRequest(api.router, "POST", "/api/v1/users", body, admin.APIKey)
	assert.Equal(t, http.StatusPaymentRequired, w.Code, w.Body.String())
	var n int64
	require.NoError(t, api.service.DB.Model(&models.User{}).Where("email = ?", "new@example.com").Count(&n).Error)
	assert.Zero(t, n, "no user is left behind")
}
