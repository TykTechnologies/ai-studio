package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeQuotas answers the App quota calls the way the enterprise engine does.
type fakeQuotas struct {
	entitlements.Service
	set     *entitlements.TeamAppQuotaInput
	deleted uint
}

func (f *fakeQuotas) GetTeamAppQuota(_ *gorm.DB, id uint) (*entitlements.TeamAppQuota, error) {
	if id == 404 {
		return nil, entitlements.ErrTeamNotFound
	}
	q := &entitlements.TeamAppQuota{GroupID: id, DefaultMax: 1}
	if f.set != nil {
		q.Set, q.MaxAppsPerMember, q.Unlimited = true, f.set.MaxAppsPerMember, f.set.Unlimited
	}
	return q, nil
}

func (f *fakeQuotas) SetTeamAppQuota(_ *gorm.DB, _ uint, in entitlements.TeamAppQuotaInput) error {
	if !in.Unlimited && (in.MaxAppsPerMember == nil || *in.MaxAppsPerMember < 0) {
		return entitlements.ErrInvalidQuota
	}
	f.set = &in
	return nil
}

func (f *fakeQuotas) DeleteTeamAppQuota(_ *gorm.DB, id uint) error {
	f.deleted = id
	return nil
}

func (f *fakeQuotas) UserAppQuota(_ *gorm.DB, _ uint) (int64, int64, bool, error) {
	return 2, 3, false, nil
}

func quotaCall(handler func(*gin.Context), method, body string, user *models.User, params gin.Params) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	if user != nil {
		c.Set("user", user)
	}
	handler(c)
	c.Writer.WriteHeaderNow() // a bare c.Status reaches the recorder only once flushed
	return w
}

func TestAppQuotaHandlers(t *testing.T) {
	api, _, service := setupTestAPIForCommonTests(t)
	fake := &fakeQuotas{Service: entitlements.NewCommunityService()}
	service.Entitlements = fake
	user := &models.User{ID: 7}
	team := gin.Params{{Key: "id", Value: "5"}}

	w := quotaCall(api.getMyAppQuota, http.MethodGet, "", user, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"used":2,"max":3,"unlimited":false}`, w.Body.String())

	w = quotaCall(api.setTeamAppQuota, http.MethodPut, `{"max_apps_per_member":5}`, user, team)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got entitlements.TeamAppQuota
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.Set)
	assert.Equal(t, 5, *got.MaxAppsPerMember)

	w = quotaCall(api.setTeamAppQuota, http.MethodPut, `{"max_apps_per_member":-1}`, user, team)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = quotaCall(api.setTeamAppQuota, http.MethodPut, `{`, user, team)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = quotaCall(api.getTeamAppQuota, http.MethodGet, "", user, gin.Params{{Key: "id", Value: "404"}})
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = quotaCall(api.deleteTeamAppQuota, http.MethodDelete, "", user, team)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, uint(5), fake.deleted)
}

// Community Edition has no App quota: every endpoint answers 402.
func TestAppQuotaHandlersCommunity(t *testing.T) {
	api, _, service := setupTestAPIForCommonTests(t)
	service.Entitlements = entitlements.NewCommunityService()
	user := &models.User{ID: 7}
	team := gin.Params{{Key: "id", Value: "5"}}
	for name, w := range map[string]*httptest.ResponseRecorder{
		"me":     quotaCall(api.getMyAppQuota, http.MethodGet, "", user, nil),
		"get":    quotaCall(api.getTeamAppQuota, http.MethodGet, "", user, team),
		"put":    quotaCall(api.setTeamAppQuota, http.MethodPut, `{"unlimited":true}`, user, team),
		"delete": quotaCall(api.deleteTeamAppQuota, http.MethodDelete, "", user, team),
	} {
		assert.Equal(t, http.StatusPaymentRequired, w.Code, name)
	}
}
