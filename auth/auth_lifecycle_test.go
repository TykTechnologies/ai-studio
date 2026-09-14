package auth_test

import (
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// Account lifecycle: the disabled switch, the empty-key guard, the SSO
// key-liveness window, the auth_method marker and the last-used stamp.

func (suite *AuthServiceTestSuite) lifecycleContext(setup func(r *http.Request)) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)
	if setup != nil {
		setup(c.Request)
	}
	return c
}

func (suite *AuthServiceTestSuite) TestGetAuthenticatedUser_Lifecycle() {
	suite.authService.Config.SyncAPIKeyTouch = true

	suite.Run("disabled user is rejected on every credential", func() {
		user := &models.User{Email: "off@example.com", Password: "x", APIKey: "off-key", SessionToken: "off-cookie", EmailVerified: true, Disabled: true}
		require.NoError(suite.T(), suite.db.Create(user).Error)

		c := suite.lifecycleContext(func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "session", Value: "off-cookie"})
		})
		assert.Nil(suite.T(), suite.authService.GetAuthenticatedUser(c), "cookie")

		c = suite.lifecycleContext(func(r *http.Request) { r.Header.Set("Authorization", "Bearer off-key") })
		assert.Nil(suite.T(), suite.authService.GetAuthenticatedUser(c), "header")

		c = suite.lifecycleContext(nil)
		c.Request, _ = http.NewRequest("GET", "/?token=off-key", nil)
		assert.Nil(suite.T(), suite.authService.GetAuthenticatedUser(c), "query")
	})

	suite.Run("an empty key never matches a keyless user", func() {
		keyless := &models.User{Email: "keyless@example.com", Password: "", APIKey: "", EmailVerified: true, AuthSource: models.AuthSourceSSO}
		require.NoError(suite.T(), suite.db.Create(keyless).Error)

		// Header value set directly, bypassing the wire parser's trimming.
		c := suite.lifecycleContext(func(r *http.Request) { r.Header.Set("Authorization", "Bearer ") })
		assert.Nil(suite.T(), suite.authService.GetAuthenticatedUser(c))

		_, err := suite.service.GetUserByAPIKey("")
		assert.Error(suite.T(), err)
	})

	suite.Run("auth_method is recorded and key use is stamped", func() {
		user := &models.User{Email: "method@example.com", Password: "x", APIKey: "method-key", SessionToken: "method-cookie", EmailVerified: true}
		require.NoError(suite.T(), suite.db.Create(user).Error)

		c := suite.lifecycleContext(func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "session", Value: "method-cookie"})
		})
		require.NotNil(suite.T(), suite.authService.GetAuthenticatedUser(c))
		assert.Equal(suite.T(), auth.AuthMethodSession, c.GetString(auth.AuthMethodKey))

		c = suite.lifecycleContext(func(r *http.Request) { r.Header.Set("Authorization", "Bearer method-key") })
		require.NotNil(suite.T(), suite.authService.GetAuthenticatedUser(c))
		assert.Equal(suite.T(), auth.AuthMethodAPIKey, c.GetString(auth.AuthMethodKey))

		var stored models.User
		require.NoError(suite.T(), suite.db.First(&stored, user.ID).Error)
		require.NotNil(suite.T(), stored.APIKeyLastUsedAt, "key use must be stamped")
		assert.WithinDuration(suite.T(), time.Now(), *stored.APIKeyLastUsedAt, time.Minute)
	})

	suite.Run("SSO key liveness window", func() {
		fresh := time.Now().Add(-time.Hour)
		stale := time.Now().Add(-48 * time.Hour)
		users := []*models.User{
			{Email: "sso-fresh@example.com", APIKey: "sso-fresh", EmailVerified: true, AuthSource: models.AuthSourceSSO, LastLoginAt: &fresh, LastLoginMethod: models.LoginMethodSSO},
			{Email: "sso-stale@example.com", APIKey: "sso-stale", EmailVerified: true, AuthSource: models.AuthSourceSSO, LastLoginAt: &stale, LastLoginMethod: models.LoginMethodSSO},
			{Email: "sso-never@example.com", APIKey: "sso-never", EmailVerified: true, AuthSource: models.AuthSourceSSO},
			{Email: "sso-pw@example.com", APIKey: "sso-pw", EmailVerified: true, AuthSource: models.AuthSourceSSO, LastLoginAt: &fresh, LastLoginMethod: models.LoginMethodPassword},
			{Email: "local-stale@example.com", Password: "x", APIKey: "local-stale", EmailVerified: true, AuthSource: models.AuthSourceLocal, LastLoginAt: &stale, LastLoginMethod: models.LoginMethodPassword},
		}
		for _, u := range users {
			require.NoError(suite.T(), suite.db.Create(u).Error)
		}
		authWith := func(key string) *models.User {
			c := suite.lifecycleContext(func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+key) })
			return suite.authService.GetAuthenticatedUser(c)
		}

		suite.authService.Config.SSOAPIKeyLiveness = 24 * time.Hour
		assert.NotNil(suite.T(), authWith("sso-fresh"), "recent SSO login keeps the key alive")
		assert.Nil(suite.T(), authWith("sso-stale"), "stale SSO login rejects the key")
		assert.Nil(suite.T(), authWith("sso-never"), "no SSO login ever rejects the key")
		assert.Nil(suite.T(), authWith("sso-pw"), "a password login is not proof of IdP standing")
		assert.NotNil(suite.T(), authWith("local-stale"), "the window only applies to SSO-origin users")

		suite.authService.Config.SSOAPIKeyLiveness = 0
		assert.NotNil(suite.T(), authWith("sso-stale"), "0 disables the window")
		assert.NotNil(suite.T(), authWith("sso-never"))
	})
}

func (suite *AuthServiceTestSuite) TestLogin_Lifecycle() {
	hashed, err := bcrypt.GenerateFromPassword([]byte("Password1!"), bcrypt.DefaultCost)
	require.NoError(suite.T(), err)

	suite.Run("disabled account gets a distinct 401", func() {
		user := &models.User{Email: "disabled-login@example.com", Password: string(hashed), EmailVerified: true, Disabled: true}
		require.NoError(suite.T(), suite.db.Create(user).Error)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("POST", "/login", nil)
		err := suite.authService.Login(c, "disabled-login@example.com", "Password1!")
		assert.Error(suite.T(), err)
		assert.Equal(suite.T(), http.StatusUnauthorized, w.Code)
		assert.Contains(suite.T(), w.Body.String(), "Account disabled")
	})

	suite.Run("successful login stamps the password method", func() {
		user := &models.User{Email: "stamp-login@example.com", Password: string(hashed), EmailVerified: true}
		require.NoError(suite.T(), suite.db.Create(user).Error)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("POST", "/login", nil)
		require.NoError(suite.T(), suite.authService.Login(c, "stamp-login@example.com", "Password1!"))

		var stored models.User
		require.NoError(suite.T(), suite.db.First(&stored, user.ID).Error)
		require.NotNil(suite.T(), stored.LastLoginAt)
		assert.Equal(suite.T(), models.LoginMethodPassword, stored.LastLoginMethod)
	})

	suite.Run("password reset is refused for a disabled account", func() {
		user := &models.User{Email: "disabled-reset@example.com", Password: string(hashed), EmailVerified: true, Disabled: true, ResetToken: "tok", ResetTokenExpiry: time.Now().Add(time.Hour)}
		require.NoError(suite.T(), suite.db.Create(user).Error)

		assert.ErrorIs(suite.T(), suite.authService.ResetPassword("disabled-reset@example.com"), auth.ErrUserDisabled)
		_, err := suite.authService.ValidateResetToken("tok")
		assert.ErrorIs(suite.T(), err, auth.ErrUserDisabled)
	})
}
