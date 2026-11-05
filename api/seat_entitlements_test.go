package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refusingSeats allows everything like the CE stub but refuses every guarded
// user change, as the enterprise engine does over a seat limit.
type refusingSeats struct {
	entitlements.Service
	refuse string
	calls  int
}

func (r *refusingSeats) GuardUserChange(tx *gorm.DB, fn func(*gorm.DB) error) error {
	r.calls++
	if r.refuse != "" {
		return entitlements.NewExceededError(r.refuse)
	}
	return fn(tx)
}

func assertQuota409(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	code, detail := quotaError(t, w)
	assert.Equal(t, entitlements.CodeQuotaExceeded, code)
	assert.Equal(t, want, detail)
}

// Every admin-initiated seat change answers a seat refusal with 409
// QUOTA_EXCEEDED and the engine's message, not 400 or 500.
func TestSeatRefusalsReachTheClientAs409(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)
	ent := &refusingSeats{Service: entitlements.NewCommunityService()}
	service.Entitlements = ent
	admin := &models.User{Email: "seat-admin@example.com", Name: "Admin", IsAdmin: true, EmailVerified: true}
	require.NoError(t, admin.Create(db))
	member := &models.User{Email: "seat-member@example.com", Name: "Member", EmailVerified: true}
	require.NoError(t, member.Create(db))
	team := &models.Group{Name: "seat-team"}
	require.NoError(t, db.Create(team).Error)
	ent.refuse = "Your organisation has reached its licensed limit of 2 users with admin access."

	call := func(method, path, body string, params gin.Params, handler func(*gin.Context)) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = params
		c.Set("user", admin)
		handler(c)
		return w
	}

	t.Run("register", func(t *testing.T) {
		w := call(http.MethodPost, "/auth/register",
			`{"data":{"type":"register","attributes":{"email":"new-seat@example.com","name":"New","password":"Str0ng-Passw0rd!","with_portal":true}}}`,
			nil, api.handleRegister)
		assertQuota409(t, w, ent.refuse)
		var n int64
		require.NoError(t, db.Model(&models.User{}).Where("email = ?", "new-seat@example.com").Count(&n).Error)
		assert.Zero(t, n)
	})

	t.Run("create user", func(t *testing.T) {
		w := call(http.MethodPost, "/users",
			`{"data":{"type":"users","attributes":{"email":"created-seat@example.com","name":"Created","password":"Str0ng-Passw0rd!","show_portal":true}}}`,
			nil, api.createUser)
		assertQuota409(t, w, ent.refuse)
	})

	t.Run("update user", func(t *testing.T) {
		w := call(http.MethodPatch, "/users/x",
			`{"data":{"type":"users","attributes":{"email":"seat-member@example.com","name":"Member","show_portal":true}}}`,
			gin.Params{{Key: "id", Value: jsonUint(member.ID)}}, api.updateUser)
		assertQuota409(t, w, ent.refuse)
	})

	t.Run("add team member", func(t *testing.T) {
		if !config.IsEnterprise() {
			t.Skip("team management is an Enterprise feature")
		}
		w := call(http.MethodPost, "/groups/x/users", `{"data":{"type":"users","id":"`+jsonUint(member.ID)+`"}}`,
			gin.Params{{Key: "id", Value: jsonUint(team.ID)}}, api.addUserToGroup)
		assertQuota409(t, w, ent.refuse)
	})

	t.Run("replace team members", func(t *testing.T) {
		if !config.IsEnterprise() {
			t.Skip("team management is an Enterprise feature")
		}
		w := call(http.MethodPut, "/groups/x/users",
			`{"data":{"type":"group-users","attributes":{"members":[`+jsonUint(member.ID)+`]}}}`,
			gin.Params{{Key: "id", Value: jsonUint(team.ID)}}, api.updateGroupUsers)
		assertQuota409(t, w, ent.refuse)
	})

	t.Run("role binding", func(t *testing.T) {
		w := call(http.MethodPost, "/rbac/bindings",
			`{"subject_type":"group","subject_id":`+jsonUint(team.ID)+`,"role_id":1}`,
			nil, api.createRoleBinding)
		assertQuota409(t, w, ent.refuse)
	})

	t.Run("re-enable user", func(t *testing.T) {
		require.NoError(t, models.SetDisabled(db, member.ID, true))
		w := call(http.MethodPost, "/users/x/enable", "",
			gin.Params{{Key: "id", Value: jsonUint(member.ID)}}, api.enableUser)
		assertQuota409(t, w, ent.refuse)
	})
}
