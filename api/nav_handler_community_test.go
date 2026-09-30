//go:build !enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
)

func TestNavManifestHandler(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	cfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	r := NewAPI(service, true, authService, cfg, nil, emptyFile, nil).router

	mkUser := func(email string, admin, portal, chat bool) string {
		u := models.NewUser()
		u.Email = email
		u.Name = email
		u.Password = "hash"
		u.IsAdmin = admin
		u.EmailVerified = true
		u.ShowPortal = portal
		u.ShowChat = chat
		require.NoError(t, u.Create(db))
		return u.APIKey
	}
	get := func(key string) NavManifest {
		w := apitest.PerformAuthRequest(r, "GET", "/common/nav", nil, key)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var m NavManifest
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m))
		return m
	}

	admin := get(mkUser("admin@nav.test", true, true, true))
	assert.Equal(t, []string{"Admin", "AI Portal", "Chat"}, labels(admin.Surfaces))
	assert.Equal(t, "Overview", admin.Admin[0].Text)
	assert.Equal(t, "Plugins", admin.Admin[len(admin.Admin)-1].Text)
	assert.NotNil(t, find(admin.Admin, "llms"))
	assert.Nil(t, find(admin.Admin, "governance"), "Community Edition has no Governance")

	user := get(mkUser("dev@nav.test", false, true, false))
	assert.Equal(t, []string{"AI Portal"}, labels(user.Surfaces))
	assert.Empty(t, user.Admin, "no admin permissions, no admin menu")

	w := apitest.PerformRequest(r, "GET", "/common/nav", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
