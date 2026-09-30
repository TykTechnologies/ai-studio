package api

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/sso"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

type noHostIdentity struct{}

func (noHostIdentity) Authenticate(*http.Request) (*services.HostIdentity, error) { return nil, nil }

// When a host application signs users in, Studio's SSO routes answer 404,
// so the identity provider pages must not be offered: show_sso_config (which
// gates the console's /admin/sso-profiles routes) and the nav manifest's
// entry follow it.
func TestShowSSOConfigFollowsLocalSignIn(t *testing.T) {
	if !sso.IsEnterpriseAvailable() {
		sso.RegisterEnterpriseFactory(func(*sso.Config, *gin.Engine, *gorm.DB, sso.NotificationService) sso.Service { return nil })
		t.Cleanup(func() { sso.RegisterEnterpriseFactory(nil) })
	}
	db := setupTestDB(t)
	u := &models.User{Email: "idp@example.com", IsAdmin: true, AccessToSSOConfig: true}
	perms := authz.NewSet(authz.Read("sso-profiles"))

	local := &API{service: services.NewService(db), config: &auth.Config{DB: db}}
	assert.True(t, local.showSSOConfig(u, perms), "Studio signs users in: identity providers are configurable")

	noConfig := &API{service: services.NewService(db)}
	assert.True(t, noConfig.showSSOConfig(u, perms), "no auth config is local sign-in")

	host := &API{service: services.NewService(db), config: &auth.Config{DB: db, HostAuth: noHostIdentity{}}}
	assert.False(t, host.showSSOConfig(u, perms), "the host signs users in: Studio's SSO is off")
}
