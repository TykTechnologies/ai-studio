package api

import (
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// fakeNotices answers the notice calls the way the enterprise engine does.
type fakeNotices struct {
	entitlements.Service
	dismissed []string
}

func (f *fakeNotices) Notices(*gorm.DB, *models.User) ([]entitlements.Notice, error) {
	return []entitlements.Notice{{Key: "quota:apps:3", Label: "Apps", Used: 2, Max: 3, Message: "near"}}, nil
}

// DismissNotice accepts only the notice the user has, as the engine does.
func (f *fakeNotices) DismissNotice(_ *gorm.DB, _ *models.User, key string) error {
	if key != "quota:apps:3" {
		return entitlements.ErrUnknownNotice
	}
	f.dismissed = append(f.dismissed, key)
	return nil
}

func TestNoticeHandlers(t *testing.T) {
	api, _, service := setupTestAPIForCommonTests(t)
	fake := &fakeNotices{Service: entitlements.NewCommunityService()}
	service.Entitlements = fake
	user := &models.User{ID: 7}

	w := quotaCall(api.getMyNotices, http.MethodGet, "", user, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"data":[{"key":"quota:apps:3","label":"Apps","used":2,"max":3,"unlimited":false,"message":"near"}]}`, w.Body.String())

	w = quotaCall(api.dismissMyNotice, http.MethodPost, "", user, gin.Params{{Key: "key", Value: "quota:apps:3"}})
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, []string{"quota:apps:3"}, fake.dismissed)

	// A key that is not one of the user's notices is refused and stores
	// nothing.
	for _, key := range []string{"bad", "quota:bogus:1"} {
		w = quotaCall(api.dismissMyNotice, http.MethodPost, "", user, gin.Params{{Key: "key", Value: key}})
		assert.Equal(t, http.StatusBadRequest, w.Code, key)
	}
	assert.Equal(t, []string{"quota:apps:3"}, fake.dismissed)
}

// Community Edition has no notices: an empty list, and nothing to dismiss.
func TestNoticeHandlersCommunity(t *testing.T) {
	api, _, service := setupTestAPIForCommonTests(t)
	service.Entitlements = entitlements.NewCommunityService()
	user := &models.User{ID: 7}

	w := quotaCall(api.getMyNotices, http.MethodGet, "", user, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"data":[]}`, w.Body.String())

	w = quotaCall(api.dismissMyNotice, http.MethodPost, "", user, gin.Params{{Key: "key", Value: "quota:apps:3"}})
	assert.Equal(t, http.StatusPaymentRequired, w.Code)
}
