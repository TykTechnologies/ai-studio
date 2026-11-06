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

// recordingEntitlements allows everything like the CE stub, records each
// GuardAppCreate, and refuses App creates when refuse is set.
type recordingEntitlements struct {
	entitlements.Service
	selfService []bool
	refuse      string
}

func (r *recordingEntitlements) GuardAppCreate(tx *gorm.DB, owner uint, selfService bool, fn func(*gorm.DB) error) error {
	r.selfService = append(r.selfService, selfService)
	if r.refuse != "" {
		return entitlements.NewExceededError(r.refuse)
	}
	return fn(tx)
}

func quotaError(t *testing.T, w *httptest.ResponseRecorder) (code, detail string) {
	t.Helper()
	var body struct {
		Errors []struct {
			Detail string `json:"detail"`
			Code   string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	require.Len(t, body.Errors, 1)
	return body.Errors[0].Code, body.Errors[0].Detail
}

// Portal creates are self-service; a refusal reaches the portal as 409
// QUOTA_EXCEEDED with the entitlements message, and no App is left behind.
func TestCreateUserAppGuardedAsSelfService(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)
	ent := &recordingEntitlements{Service: entitlements.NewCommunityService()}
	service.Entitlements = ent
	user := createTestUserWithSettings(t, service, "quota-portal@example.com", "Portal User", false, true, true, true, false)

	post := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/common/apps", bytes.NewBufferString(`{"name":"Quota App","description":"d","llm_ids":[],"data_source_ids":[],"tool_ids":[]}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)
		api.createUserApp(c)
		return w
	}

	w := post()
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	ent.refuse = "Your organisation has reached its licensed limit of 1 app."
	w = post()
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	code, detail := quotaError(t, w)
	assert.Equal(t, entitlements.CodeQuotaExceeded, code)
	assert.Equal(t, ent.refuse, detail)

	assert.Equal(t, []bool{true, true}, ent.selfService)
	var n int64
	require.NoError(t, db.Model(&models.App{}).Count(&n).Error)
	assert.Equal(t, int64(1), n)
}

// Admin creates are not self-service, and a refusal is a 409 too.
func TestAdminCreateAppGuardedNotSelfService(t *testing.T) {
	api, db := setupTestAPI(t)
	ent := &recordingEntitlements{Service: entitlements.NewCommunityService()}
	api.service.Entitlements = ent
	owner := &models.User{Email: "quota-admin@example.com", Name: "Admin", IsAdmin: true, EmailVerified: true}
	require.NoError(t, owner.Create(db))

	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/apps", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", owner)
		api.createApp(c)
		return w
	}
	body := `{"data":{"type":"app","attributes":{"name":"Admin App","user_id":` + jsonUint(owner.ID) + `}}}`
	nsBody := `{"data":{"type":"app","attributes":{"name":"NS App","namespace":"eu","user_id":` + jsonUint(owner.ID) + `}}}`

	w := post(body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	ent.refuse = "over the limit"
	for _, b := range []string{body, nsBody} {
		w = post(b)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		code, detail := quotaError(t, w)
		assert.Equal(t, entitlements.CodeQuotaExceeded, code)
		assert.Equal(t, "over the limit", detail)
	}
	assert.Equal(t, []bool{false, false, false}, ent.selfService)
}

func jsonUint(v uint) string {
	b, _ := json.Marshal(v)
	return string(b)
}
