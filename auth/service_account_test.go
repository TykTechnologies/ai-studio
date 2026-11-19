package auth_test

import (
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

func (suite *AuthServiceTestSuite) newServiceAccount() *models.User {
	user, err := suite.service.CreateUser(services.UserDTO{Name: "Operator", AccountType: models.AccountTypeService})
	require.NoError(suite.T(), err)
	return user
}

func (suite *AuthServiceTestSuite) requestContext(header http.Header, cookie *http.Cookie, query string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request, _ = http.NewRequest("GET", "/api/v1/llms"+query, nil)
	for k, v := range header {
		c.Request.Header[k] = v
	}
	if cookie != nil {
		c.Request.AddCookie(cookie)
	}
	return c
}

// A service account authenticates with its API key, in the header or as
// ?token=, and by no other means.
func (suite *AuthServiceTestSuite) TestServiceAccount_APIKeyOnly() {
	t := suite.T()
	svc := suite.newServiceAccount()

	c := suite.requestContext(http.Header{"Authorization": {"Bearer " + svc.APIKey}}, nil, "")
	got := suite.authService.GetAuthenticatedUser(c)
	require.NotNil(t, got, "the API key works")
	assert.Equal(t, svc.ID, got.ID)

	c = suite.requestContext(nil, nil, "?token="+svc.APIKey)
	got = suite.authService.GetAuthenticatedUser(c)
	require.NotNil(t, got)
	assert.Equal(t, svc.ID, got.ID)

	// No session can be created for it...
	c = suite.requestContext(nil, nil, "")
	assert.ErrorIs(t, suite.authService.SetUserSession(c, svc), auth.ErrServiceAccountSession)
	var stored models.User
	require.NoError(t, suite.db.First(&stored, svc.ID).Error)
	assert.Empty(t, stored.SessionToken)

	// ...and a session token minted by other means (an older build) is not
	// honored.
	require.NoError(t, suite.db.Model(&models.User{}).Where("id = ?", svc.ID).UpdateColumn("session_token", "minted-elsewhere").Error)
	c = suite.requestContext(nil, &http.Cookie{Name: "session", Value: "minted-elsewhere"}, "")
	assert.Nil(t, suite.authService.GetAuthenticatedUser(c))
}

// Password reset, reset tokens and verification email answer for a service
// account exactly as for an unknown address.
func (suite *AuthServiceTestSuite) TestServiceAccount_ResetAndVerificationAreSilent() {
	t := suite.T()
	svc := suite.newServiceAccount()

	unknownReset := suite.authService.ResetPassword("nobody@example.com")
	reset := suite.authService.ResetPassword(svc.Email)
	require.Error(t, reset)
	assert.ErrorIs(t, reset, gorm.ErrRecordNotFound)
	assert.Equal(t, unknownReset.Error(), reset.Error())
	var stored models.User
	require.NoError(t, suite.db.First(&stored, svc.ID).Error)
	assert.Empty(t, stored.ResetToken, "no reset token is issued")

	// A reset token planted by an older build is not accepted.
	require.NoError(t, suite.db.Model(&models.User{}).Where("id = ?", svc.ID).Updates(map[string]interface{}{
		"reset_token": "planted", "reset_token_expiry": time.Now().Add(time.Hour),
	}).Error)
	_, err := suite.authService.ValidateResetToken("planted")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	unknownResend := suite.authService.ResendVerificationEmail("nobody@example.com")
	resend := suite.authService.ResendVerificationEmail(svc.Email)
	require.Error(t, resend)
	assert.Equal(t, unknownResend.Error(), resend.Error())
}

func (suite *AuthServiceTestSuite) TestServiceAccount_PasswordLoginRefused() {
	t := suite.T()
	svc := suite.newServiceAccount()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/auth/login", nil)

	err := suite.authService.Login(c, svc.Email, "")
	assert.Error(t, err)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid email or password", "the same answer as a wrong password")
}
