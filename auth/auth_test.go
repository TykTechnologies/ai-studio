package auth_test

import (
	"errors"
	"strings"

	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/notifications"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMockMailService() *notifications.MailService {
	return notifications.NewTestMailService()
}

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

type AuthServiceTestSuite struct {
	suite.Suite
	authService *auth.AuthService
	db          *gorm.DB
	service     *services.Service
}

func (suite *AuthServiceTestSuite) SetupTest() {
	suite.db = setupTestDB(suite.T())
	suite.service = services.NewService(suite.db)
	notificationService := services.NewTestNotificationService(suite.db)
	config := auth.Config{
		DB:                  suite.db,
		Service:             suite.service,
		CookieName:          "session",
		CookieSecure:        true,
		CookieHTTPOnly:      true,
		CookieSameSite:      http.SameSiteStrictMode,
		ResetTokenExpiry:    time.Hour,
		FrontendURL:         "http://example.com",
		RegistrationAllowed: true,
		AdminEmail:          "admin@example.com",
	}
	mockMailService := newMockMailService()
	suite.authService = auth.NewAuthService(&config, mockMailService, suite.service, notificationService)
}

func (suite *AuthServiceTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	if err == nil {
		sqlDB.Close()
	}
}

func TestAuthServiceSuite(t *testing.T) {
	suite.Run(t, new(AuthServiceTestSuite))
}

func (suite *AuthServiceTestSuite) TestLogin() {
	suite.Run("Successful login", func() {
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
		user := &models.User{Email: "test@example.com", Password: string(hashedPassword), EmailVerified: true}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("POST", "/login", nil)

		err = suite.authService.Login(c, "test@example.com", "password123")

		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), http.StatusOK, w.Code)
		assert.Contains(suite.T(), w.Header().Get("Set-Cookie"), "session=")
	})
	suite.Run("Failed login - invalid credentials", func() {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("POST", "/login", nil)

		err := suite.authService.Login(c, "test@example.com", "wrongpassword")

		assert.Error(suite.T(), err)
		assert.Equal(suite.T(), http.StatusUnauthorized, w.Code)
		assert.Contains(suite.T(), err.Error(), "unauthorized")
		assert.Contains(suite.T(), w.Body.String(), "Invalid email or password")
	})
}

func (suite *AuthServiceTestSuite) TestLogout() {
	suite.Run("Successful logout", func() {
		user := &models.User{Email: "test@example.com", SessionToken: "valid_token"}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", user)
		c.Request, _ = http.NewRequest("POST", "/logout", nil)

		// Add some test cookies to the request
		c.Request.AddCookie(&http.Cookie{Name: "test_cookie1", Value: "value1"})
		c.Request.AddCookie(&http.Cookie{Name: "test_cookie2", Value: "value2"})
		c.Request.AddCookie(&http.Cookie{Name: suite.authService.Config.CookieName, Value: "session_value"})

		err = suite.authService.Logout(c)

		assert.NoError(suite.T(), err)

		// Check that all cookies are cleared
		cookies := w.Result().Cookies()
		assert.GreaterOrEqual(suite.T(), len(cookies), 3) // At least the 3 cookies we added

		// Verify all cookies are expired
		for _, cookie := range cookies {
			assert.Equal(suite.T(), "", cookie.Value, "Cookie value should be empty")
			assert.True(suite.T(), cookie.Expires.Before(time.Now()), "Cookie should be expired")
		}
	})

	suite.Run("Logout failure - user not in context", func() {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("POST", "/logout", nil)

		err := suite.authService.Logout(c)

		assert.Error(suite.T(), err)
	})
}
func (suite *AuthServiceTestSuite) TestResetPassword() {
	suite.Run("Successful password reset request", func() {
		user := &models.User{Email: "test@example.com"}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		err = suite.authService.ResetPassword("test@example.com")
		assert.NoError(suite.T(), err)

		var updatedUser models.User
		suite.db.First(&updatedUser, user.ID)
		assert.NotEmpty(suite.T(), updatedUser.ResetToken)
		assert.False(suite.T(), updatedUser.ResetTokenExpiry.IsZero())
	})

	suite.Run("Reset password failure - user not found", func() {
		err := suite.authService.ResetPassword("nonexistent@example.com")
		assert.Error(suite.T(), err)
	})
}
func (suite *AuthServiceTestSuite) TestRegister() {
	suite.Run("First user registration becomes admin with notifications", func() {
		// Register first user
		err := suite.authService.Register("admin@example.com", "Admin User", "Password123!", true, true)
		assert.NoError(suite.T(), err)

		var firstUser models.User
		err = suite.db.Where("email = ?", "admin@example.com").First(&firstUser).Error
		assert.NoError(suite.T(), err)
		assert.True(suite.T(), firstUser.IsAdmin, "First user should be admin")
		assert.True(suite.T(), firstUser.EmailVerified, "First user should have verified email")
		assert.True(suite.T(), firstUser.NotificationsEnabled, "First user should have notifications enabled")

		// Register second user
		err = suite.authService.Register("user@example.com", "Regular User", "Password123!", true, true)
		assert.NoError(suite.T(), err)

		var secondUser models.User
		err = suite.db.Where("email = ?", "user@example.com").First(&secondUser).Error
		assert.NoError(suite.T(), err)
		assert.False(suite.T(), secondUser.IsAdmin, "Second user should not be admin")
		assert.False(suite.T(), secondUser.EmailVerified, "Second user should not have verified email")
		assert.False(suite.T(), secondUser.NotificationsEnabled, "Second user should not have notifications enabled")
	})

	suite.Run("Successful registration", func() {
		err := suite.authService.Register("test@example.com", "Test User", "Password123!", true, true)
		assert.NoError(suite.T(), err)

		var user models.User
		err = suite.db.Where("email = ?", "test@example.com").First(&user).Error
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "Test User", user.Name)
		assert.NotEmpty(suite.T(), user.Password)

		// Check if user is added to the default group
		var defaultGroup models.Group
		err = suite.db.Where("name = ?", "Default").Preload("Users").First(&defaultGroup).Error
		assert.NoError(suite.T(), err)

		userFound := false
		for _, groupUser := range defaultGroup.Users {
			if groupUser.ID == user.ID {
				userFound = true
				break
			}
		}
		assert.True(suite.T(), userFound, "User should be in the default group")
	})

	suite.Run("Registration failure - weak password", func() {
		// Try to register with a weak password
		err := suite.authService.Register("weak-password@example.com", "Test User", "weak", true, true)

		// Verify the registration failed
		assert.Error(suite.T(), err)

		// Verify the user was not created in the database
		var user models.User
		result := suite.db.Where("email = ?", "weak-password@example.com").First(&user)
		assert.Error(suite.T(), result.Error)
		assert.True(suite.T(), errors.Is(result.Error, gorm.ErrRecordNotFound), "User should not exist in the database")
	})

	suite.Run("Registration failure - registration not allowed", func() {
		suite.authService.Config.RegistrationAllowed = false
		defer func() {
			suite.authService.Config.RegistrationAllowed = true
		}()

		err := suite.authService.Register("test@example.com", "Test User", "Password123!", true, true)

		assert.Error(suite.T(), err)
		assert.Contains(suite.T(), err.Error(), "registration is currently disabled")
	})
}

func (suite *AuthServiceTestSuite) TestVerifyEmail() {
	suite.Run("Successful email verification", func() {
		user := &models.User{Email: "test@example.com", VerificationToken: "valid_token", EmailVerified: false}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		err = suite.authService.VerifyEmail("valid_token")

		assert.NoError(suite.T(), err)

		var updatedUser models.User
		suite.db.First(&updatedUser, user.ID)
		assert.True(suite.T(), updatedUser.EmailVerified)
		assert.Empty(suite.T(), updatedUser.VerificationToken)
	})

	suite.Run("Email verification failure - invalid token", func() {
		err := suite.authService.VerifyEmail("invalid_token")

		assert.Error(suite.T(), err)
	})
}

func (suite *AuthServiceTestSuite) TestResendVerificationEmail() {
	suite.Run("Successful resend verification email", func() {
		user := &models.User{Email: "test@example.com", EmailVerified: false}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		err = suite.authService.ResendVerificationEmail("test@example.com")

		assert.NoError(suite.T(), err)

		var updatedUser models.User
		suite.db.First(&updatedUser, user.ID)
		assert.NotEmpty(suite.T(), updatedUser.VerificationToken)
	})

	suite.Run("Resend verification email failure - already verified", func() {
		user := &models.User{Email: "verified@example.com", EmailVerified: true}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		err = suite.authService.ResendVerificationEmail("verified@example.com")

		assert.Error(suite.T(), err)
		assert.Contains(suite.T(), err.Error(), "email is already verified")
	})
}

func (suite *AuthServiceTestSuite) TestValidatePasswordComplexity() {
	suite.Run("Valid password", func() {
		err := suite.authService.ValidatePasswordComplexity("Password123!")
		assert.NoError(suite.T(), err)
	})

	suite.Run("Invalid password - too short", func() {
		err := suite.authService.ValidatePasswordComplexity("Pass1!")
		assert.Error(suite.T(), err)
		assert.Contains(suite.T(), err.Error(), "password must be at least 8 characters long")
	})

	suite.Run("Invalid password - missing uppercase", func() {
		err := suite.authService.ValidatePasswordComplexity("password123!")
		assert.Error(suite.T(), err)
		assert.Contains(suite.T(), err.Error(), "password must contain at least one uppercase letter")
	})

	suite.Run("Invalid password - missing lowercase", func() {
		err := suite.authService.ValidatePasswordComplexity("PASSWORD123!")
		assert.Error(suite.T(), err)
		assert.Contains(suite.T(), err.Error(), "one lowercase letter")
	})

	suite.Run("Invalid password - missing number", func() {
		err := suite.authService.ValidatePasswordComplexity("Password!")
		assert.Error(suite.T(), err)
		assert.Contains(suite.T(), err.Error(), "one number")
	})

	suite.Run("Invalid password - missing special character", func() {
		err := suite.authService.ValidatePasswordComplexity("Password123")
		assert.Error(suite.T(), err)
		assert.Contains(suite.T(), err.Error(), "one special character")
	})
}

func (suite *AuthServiceTestSuite) TestValidateResetToken() {
	suite.Run("Valid reset token", func() {
		user := &models.User{Email: "test@example.com", ResetToken: "valid_token", ResetTokenExpiry: time.Now().Add(time.Hour)}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		validUser, err := suite.authService.ValidateResetToken("valid_token")

		assert.NoError(suite.T(), err)
		assert.NotNil(suite.T(), validUser)
		assert.Equal(suite.T(), user.ID, validUser.ID)
	})

	suite.Run("Invalid reset token", func() {
		validUser, err := suite.authService.ValidateResetToken("invalid_token")

		assert.Error(suite.T(), err)
		assert.Nil(suite.T(), validUser)
	})

	suite.Run("Expired reset token", func() {
		user := &models.User{Email: "test@example.com", ResetToken: "expired_token", ResetTokenExpiry: time.Now().Add(-time.Hour)}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		validUser, err := suite.authService.ValidateResetToken("expired_token")

		assert.Error(suite.T(), err)
		assert.Nil(suite.T(), validUser)
		assert.Contains(suite.T(), err.Error(), "reset token has expired")
	})
}
func (suite *AuthServiceTestSuite) TestUpdatePassword() {
	suite.Run("Successful password update", func() {
		initialPassword := "InitialPassword123!"
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(initialPassword), bcrypt.DefaultCost)
		user := &models.User{Email: "test@example.com", Password: string(hashedPassword)}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		newPassword := "NewPassword123!"
		err = suite.authService.UpdatePassword(user, initialPassword, newPassword)
		assert.NoError(suite.T(), err)

		var updatedUser models.User
		suite.db.First(&updatedUser, user.ID)

		// Verify that the old password no longer works
		err = bcrypt.CompareHashAndPassword([]byte(updatedUser.Password), []byte(initialPassword))
		assert.Error(suite.T(), err, "Old password should not work")

		// Verify that the new password works
		err = bcrypt.CompareHashAndPassword([]byte(updatedUser.Password), []byte(newPassword))
		assert.NoError(suite.T(), err, "New password should work")
	})

	suite.Run("Password update failure - weak password", func() {
		initialPassword := "InitialPassword123!"
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(initialPassword), bcrypt.DefaultCost)
		user := &models.User{Email: "test@example.com", Password: string(hashedPassword)}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		err = suite.authService.UpdatePassword(user, initialPassword, "weak")
		assert.Error(suite.T(), err)
		assert.Contains(suite.T(), strings.ToLower(err.Error()), "password must be at least 8 characters long")
	})
}

func (suite *AuthServiceTestSuite) TestSetUserSession() {
	suite.Run("Successfully set user session", func() {
		// Create a test user
		user := &models.User{Email: "session@example.com", Password: "password"}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		// Setup Gin context
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)

		// Call SetUserSession
		err = suite.authService.SetUserSession(c, user)
		assert.NoError(suite.T(), err)

		// Verify cookie was set
		cookieHeader := w.Header().Get("Set-Cookie")
		assert.Contains(suite.T(), cookieHeader, "session=")
		assert.Contains(suite.T(), cookieHeader, "HttpOnly")
		assert.Contains(suite.T(), cookieHeader, "SameSite=Strict")
		assert.Contains(suite.T(), cookieHeader, "Path=/")

		// Verify session token was saved to user
		var updatedUser models.User
		suite.db.First(&updatedUser, user.ID)
		assert.NotEmpty(suite.T(), updatedUser.SessionToken)

		// Verify user was set in context
		contextUser, exists := c.Get("user")
		assert.True(suite.T(), exists)
		assert.Equal(suite.T(), user, contextUser)
	})

	suite.Run("Session token update for existing user", func() {
		// Create a test user with existing session token
		user := &models.User{Email: "existing-session@example.com", Password: "password", SessionToken: "old-token"}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		// Setup Gin context
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)

		// Call SetUserSession
		err = suite.authService.SetUserSession(c, user)
		assert.NoError(suite.T(), err)

		// Verify session token was updated
		var updatedUser models.User
		suite.db.First(&updatedUser, user.ID)
		assert.NotEqual(suite.T(), "old-token", updatedUser.SessionToken)
	})
}

func (suite *AuthServiceTestSuite) TestGetAuthenticatedUser() {
	suite.Run("Get user from cookie", func() {
		// Create a test user with session token
		user := &models.User{Email: "cookie-auth@example.com", Password: "password", SessionToken: "cookie-token", EmailVerified: true}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		// Setup Gin context with cookie
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		c.Request.AddCookie(&http.Cookie{Name: "session", Value: "cookie-token"})

		// Call GetAuthenticatedUser
		authUser := suite.authService.GetAuthenticatedUser(c)

		// Verify correct user was returned
		assert.NotNil(suite.T(), authUser)
		assert.Equal(suite.T(), user.ID, authUser.ID)
		assert.Equal(suite.T(), user.Email, authUser.Email)
	})

	suite.Run("Get user from API key in query parameter", func() {
		// Create a test user with API key
		user := &models.User{Email: "apikey-auth@example.com", Password: "password", APIKey: "query-api-key", EmailVerified: true}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		// Setup Gin context with API key in query
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/?token=query-api-key", nil)

		// Call GetAuthenticatedUser
		authUser := suite.authService.GetAuthenticatedUser(c)

		// Verify correct user was returned
		assert.NotNil(suite.T(), authUser)
		assert.Equal(suite.T(), user.ID, authUser.ID)
		assert.Equal(suite.T(), user.Email, authUser.Email)
	})

	suite.Run("Get user from Authorization header", func() {
		// Create a test user with API key
		user := &models.User{Email: "header-auth@example.com", Password: "password", APIKey: "header-api-key", EmailVerified: true}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		// Setup Gin context with Authorization header
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		c.Request.Header.Set("Authorization", "Bearer header-api-key")

		// Call GetAuthenticatedUser
		authUser := suite.authService.GetAuthenticatedUser(c)

		// Verify correct user was returned
		assert.NotNil(suite.T(), authUser)
		assert.Equal(suite.T(), user.ID, authUser.ID)
		assert.Equal(suite.T(), user.Email, authUser.Email)
	})

	suite.Run("No authentication provided", func() {
		// Setup Gin context with no authentication
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)

		// Call GetAuthenticatedUser
		authUser := suite.authService.GetAuthenticatedUser(c)

		// Verify no user was returned
		assert.Nil(suite.T(), authUser)
	})

	suite.Run("Invalid cookie token", func() {
		// Setup Gin context with invalid cookie
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		c.Request.AddCookie(&http.Cookie{Name: "session", Value: "invalid-token"})

		// Call GetAuthenticatedUser
		authUser := suite.authService.GetAuthenticatedUser(c)

		// Verify no user was returned
		assert.Nil(suite.T(), authUser)
	})

	suite.Run("User with unverified email", func() {
		// Create a test user with API key but unverified email
		user := &models.User{Email: "unverified@example.com", Password: "password", APIKey: "unverified-api-key", EmailVerified: false}
		err := suite.db.Create(user).Error
		assert.NoError(suite.T(), err)

		// Setup Gin context with API key
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/?token=unverified-api-key", nil)

		// Call GetAuthenticatedUser
		authUser := suite.authService.GetAuthenticatedUser(c)

		// Verify no user was returned (email not verified)
		assert.Nil(suite.T(), authUser)
	})
}
