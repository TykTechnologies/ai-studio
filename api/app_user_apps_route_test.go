package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The user apps routes register the path parameter as :id (see api.go), so the
// handlers must read that name. These tests go through a router with the same
// route strings api.go uses: a handler called directly with hand-set params
// would accept whatever name the test chose and would not catch the mismatch.
//
// The app service has no interface to substitute (API.service is a concrete
// *services.Service), so the service here is the package's in-memory test
// fixture, the same one app_handlers_test.go and common_test.go use. No
// database server, container, or network listener is started.
const (
	userAppsRoute      = "/users/:id/apps"
	userAppsCountRoute = "/users/:id/apps/count"
)

type userAppsFixture struct {
	api    *API
	router *gin.Engine
}

func newUserAppsFixture(t *testing.T) *userAppsFixture {
	api, _ := setupTestAPI(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	v1.GET(userAppsRoute, api.getAppsByUserID)
	v1.GET(userAppsCountRoute, api.countAppsByUserID)

	return &userAppsFixture{api: api, router: router}
}

// get drives a handler through the router, which is what resolves the path
// parameter the handler then reads.
func (f *userAppsFixture) get(path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	f.router.ServeHTTP(w, req)
	return w
}

func createTestAppForUser(t *testing.T, fixture *userAppsFixture, userID uint, name string) *models.App {
	app := &models.App{
		Name:        name,
		Description: "Test App",
		UserID:      userID,
		IsActive:    true,
	}
	err := fixture.api.service.DB.Create(app).Error
	require.NoError(t, err)
	return app
}

// Case A: an existing numeric user ID returns 200 with that user's apps.
func TestUserAppsRouteNumericIDReturnsApps(t *testing.T) {
	fixture := newUserAppsFixture(t)

	user := createTestUser(t, fixture.api.service)
	appA := createTestAppForUser(t, fixture, user.ID, "Route App A")
	appB := createTestAppForUser(t, fixture, user.ID, "Route App B")

	w := fixture.get(fmt.Sprintf("/api/v1/users/%d/apps", user.ID))
	require.Equal(t, http.StatusOK, w.Code, "the registered param must reach the service: %s", w.Body.String())

	var response AppListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data, 2)

	names := make([]string, len(response.Data))
	for i, app := range response.Data {
		names[i] = app.Attributes.Name
	}
	assert.ElementsMatch(t, []string{appA.Name, appB.Name}, names)

	for _, app := range response.Data {
		assert.Equal(t, user.ID, app.Attributes.UserID, "the apps returned belong to the requested user")
	}
}

// Case B: a user with no apps returns 200 with an empty list and count 0.
func TestUserAppsRouteUserWithNoAppsReturnsEmptyListAndZeroCount(t *testing.T) {
	fixture := newUserAppsFixture(t)

	emptyUser := createTestUser(t, fixture.api.service)
	otherUser := createTestUser(t, fixture.api.service)
	createTestAppForUser(t, fixture, otherUser.ID, "Other User App")

	w := fixture.get(fmt.Sprintf("/api/v1/users/%d/apps", emptyUser.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var response AppListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Empty(t, response.Data, "the user with no apps gets an empty list, not another user's apps")
	assert.Contains(t, w.Body.String(), `"data":[]`, "an empty list is still a JSON array")

	wCount := fixture.get(fmt.Sprintf("/api/v1/users/%d/apps/count", emptyUser.ID))
	require.Equal(t, http.StatusOK, wCount.Code, wCount.Body.String())

	var countResponse struct {
		Count int64 `json:"count"`
	}
	require.NoError(t, json.Unmarshal(wCount.Body.Bytes(), &countResponse))
	assert.Equal(t, int64(0), countResponse.Count)

	// The same request for the user that does have an app still counts that
	// one, so the zero above is the per-user answer, not a broken lookup.
	wOther := fixture.get(fmt.Sprintf("/api/v1/users/%d/apps/count", otherUser.ID))
	require.Equal(t, http.StatusOK, wOther.Code, wOther.Body.String())

	var otherCount struct {
		Count int64 `json:"count"`
	}
	require.NoError(t, json.Unmarshal(wOther.Body.Bytes(), &otherCount))
	assert.Equal(t, int64(1), otherCount.Count)
}

// Case C: a nonnumeric ID is rejected with 400 before the service layer runs.
func TestUserAppsRouteNonNumericIDReturnsBadRequest(t *testing.T) {
	fixture := newUserAppsFixture(t)

	for _, path := range []string{"/api/v1/users/abc/apps", "/api/v1/users/abc/apps/count"} {
		w := fixture.get(path)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

		var response ErrorResponse
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Len(t, response.Errors, 1)
		assert.Equal(t, "Bad Request", response.Errors[0].Title)
		assert.Equal(t, "Invalid user ID", response.Errors[0].Detail)
	}
}

// Case D: the regression guard. The router must register the parameter as :id,
// and a numeric ID must reach the service. A handler that reads a different
// parameter name gets "" from gin, so ParseUint fails and Case A (and this
// case) 400s before the service layer runs.
func TestUserAppsRouteRegistersIDParamAndNumericIDReachesService(t *testing.T) {
	fixture := newUserAppsFixture(t)

	routes := fixture.router.Routes()
	registered := make([]string, len(routes))
	for i, r := range routes {
		registered[i] = r.Method + " " + r.Path
	}
	assert.Contains(t, registered, "GET /api/v1"+userAppsRoute, "the list route registers :id")
	assert.Contains(t, registered, "GET /api/v1"+userAppsCountRoute, "the count route registers :id")

	user := createTestUser(t, fixture.api.service)
	app := createTestAppForUser(t, fixture, user.ID, "Route Guard App")

	w := fixture.get(fmt.Sprintf("/api/v1/users/%d/apps", user.ID))
	require.Equal(t, http.StatusOK, w.Code, "a numeric ID must reach the service: %s", w.Body.String())

	var response AppListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data, 1)
	assert.Equal(t, app.Name, response.Data[0].Attributes.Name)
	assert.Equal(t, user.ID, response.Data[0].Attributes.UserID, "the value the service got is the registered :id param")
}
