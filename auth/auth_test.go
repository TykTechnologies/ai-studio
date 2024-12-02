package auth_test

import (
	"strings"

	"github.com/go-mail/mail"

	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mockMailer struct {
	sentEmails []*mail.Message
}

func (m *mockMailer) DialAndSend(msg ...*mail.Message) error {
	m.sentEmails = append(m.sentEmails, msg...)
	return nil
}

func newMockMailer() *mockMailer {
	return &mockMailer{
		sentEmails: make([]*mail.Message, 0),
	}
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
	mockMailer := newMockMailer()
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
		SMTPHost:            "dummy.host.com",
	}
	suite.authService = auth.NewAuthService(&config, mockMailer, suite.service)
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
		user := &models.User{Email: "test@example.com", Password: string(hashedPassword)}
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
		assert.Contains(suite.T(), w.Body.String(), "Unauthorized")
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

		err = suite.authService.Logout(c)

		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), http.StatusOK, w.Code)
		assert.Contains(suite.T(), w.Header().Get("Set-Cookie"), "session=;")
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

		mockMailer := suite.authService.Mailer.(*mockMailer)
		mockMailer.sentEmails = nil // Clear previous emails

		err = suite.authService.ResetPassword("test@example.com")

		assert.NoError(suite.T(), err)

		var updatedUser models.User
		suite.db.First(&updatedUser, user.ID)
		assert.NotEmpty(suite.T(), updatedUser.ResetToken)
		assert.False(suite.T(), updatedUser.ResetTokenExpiry.IsZero())

		// Check if email was sent
		assert.Equal(suite.T(), 1, len(mockMailer.sentEmails))
		if len(mockMailer.sentEmails) > 0 {
			assert.Equal(suite.T(), "test@example.com", mockMailer.sentEmails[0].GetHeader("To")[0])
			assert.Equal(suite.T(), "Password Reset", mockMailer.sentEmails[0].GetHeader("Subject")[0])
		}
	})

	suite.Run("Reset password failure - user not found", func() {
		mockMailer := suite.authService.Mailer.(*mockMailer)
		mockMailer.sentEmails = nil // Clear previous emails

		err := suite.authService.ResetPassword("nonexistent@example.com")

		assert.Error(suite.T(), err)
		assert.Equal(suite.T(), 0, len(mockMailer.sentEmails))
	})
}
func (suite *AuthServiceTestSuite) TestRegister() {
	suite.Run("Successful registration", func() {
		err := suite.authService.Register("test@example.com", "Test User", "Password123!")
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
		err := suite.authService.Register("test@example.com", "Test User", "weak")

		assert.Error(suite.T(), err)
		assert.Contains(suite.T(), strings.ToLower(err.Error()), "password must be at least 8 characters long")
	})

	suite.Run("Registration failure - registration not allowed", func() {
		suite.authService.Config.RegistrationAllowed = false
		defer func() {
			suite.authService.Config.RegistrationAllowed = true
		}()

		err := suite.authService.Register("test@example.com", "Test User", "Password123!")

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
