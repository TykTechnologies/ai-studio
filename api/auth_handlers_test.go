package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/go-mail/mail"
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

func (m *mockMailer) Reset() {
	m.sentEmails = make([]*mail.Message, 0)
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

type AuthHandlersTestSuite struct {
	suite.Suite
	api         *API
	authService *auth.AuthService
	db          *gorm.DB
	service     *services.Service
	mockMailer  *mockMailer
}

func (suite *AuthHandlersTestSuite) SetupTest() {
	suite.db = setupTestDB(suite.T())
	suite.service = services.NewService(suite.db)
	suite.mockMailer = newMockMailer()
	config := &auth.Config{
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
		TestMode:            false,
		SMTPHost:            "testhost",
	}
	suite.authService = auth.NewAuthService(config, suite.mockMailer, suite.service)
	suite.api = &API{
		service: suite.service,
		auth:    suite.authService,
		router:  gin.Default(),
	}
	suite.api.setupRoutes()
}

func (suite *AuthHandlersTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	if err == nil {
		sqlDB.Close()
	}
}

func TestAuthHandlersSuite(t *testing.T) {
	t.Skip()
	suite.Run(t, new(AuthHandlersTestSuite))
}

func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (suite *AuthHandlersTestSuite) TestLoginHandler() {
	suite.Run("Successful Password Reset", func() {
		defer suite.mockMailer.Reset()
		user := &models.User{Email: "reset@example.com", Password: "oldhashed"}
		suite.db.Create(user)

		resetToken, err := generateRandomToken()
		assert.NoError(suite.T(), err)

		user.ResetToken = resetToken
		user.ResetTokenExpiry = time.Now().Add(time.Hour)
		suite.db.Save(user)

		resetPasswordInput := ResetPasswordInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Token    string `json:"token"`
					Password string `json:"password"`
				} `json:"attributes"`
			}{
				Type: "reset_password",
				Attributes: struct {
					Token    string `json:"token"`
					Password string `json:"password"`
				}{
					Token:    resetToken,
					Password: "NewPassword123!",
				},
			},
		}

		w := performRequest(suite.api.router, "POST", "/auth/reset-password", resetPasswordInput)
		assert.Equal(suite.T(), http.StatusOK, w.Code)
		if w.Code != http.StatusOK {
			fmt.Println(w.Body.String())
		}

		var response map[string]string
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		fmt.Println(w.Body.String())
		assert.Equal(suite.T(), "Password reset successful", response["message"])

		// Verify that the password was updated
		var updatedUser models.User
		suite.db.First(&updatedUser, user.ID)
		err = bcrypt.CompareHashAndPassword([]byte(updatedUser.Password), []byte("NewPassword123!"))
		assert.NoError(suite.T(), err)
		assert.Empty(suite.T(), updatedUser.ResetToken)
	})

	suite.Run("Invalid Credentials", func() {
		defer suite.mockMailer.Reset()
		loginInput := LoginInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Email    string `json:"email"`
					Password string `json:"password"`
				} `json:"attributes"`
			}{
				Type: "login",
				Attributes: struct {
					Email    string `json:"email"`
					Password string `json:"password"`
				}{
					Email:    "nonexistent@example.com",
					Password: "wrongpassword",
				},
			},
		}

		w := performRequest(suite.api.router, "POST", "/auth/login", loginInput)
		assert.Equal(suite.T(), http.StatusUnauthorized, w.Code)

		var response ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		if err != nil {
			fmt.Println(w.Body.String())
		}
		if len(response.Errors) > 0 {
			assert.Equal(suite.T(), "Unauthorized", response.Errors[0].Title)
		}

	})
}

func (suite *AuthHandlersTestSuite) TestRegisterHandler() {
	suite.Run("Successful Registration", func() {
		defer suite.mockMailer.Reset()
		registerInput := RegisterInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Email      string `json:"email"`
					Name       string `json:"name"`
					Password   string `json:"password"`
					WithPortal bool   `json:"with_portal"`
					WithChat   bool   `json:"with_chat"`
				} `json:"attributes"`
			}{
				Type: "register",
				Attributes: struct {
					Email      string `json:"email"`
					Name       string `json:"name"`
					Password   string `json:"password"`
					WithPortal bool   `json:"with_portal"`
					WithChat   bool   `json:"with_chat"`
				}{
					Email:      "newuser@example.com",
					Name:       "New User",
					Password:   "Password123!",
					WithPortal: true,
					WithChat:   true,
				},
			},
		}

		w := performRequest(suite.api.router, "POST", "/auth/register", registerInput)
		assert.Equal(suite.T(), http.StatusCreated, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "User registered successfully", response["message"])

		assert.Equal(suite.T(), 1, len(suite.mockMailer.sentEmails))
		if len(suite.mockMailer.sentEmails) != 1 {
			for _, email := range suite.mockMailer.sentEmails {
				fmt.Println(email.GetHeader("Subject")[0])
			}
		}
		if len(suite.mockMailer.sentEmails) > 0 {
			assert.Equal(suite.T(), "admin@example.com", suite.mockMailer.sentEmails[0].GetHeader("To")[0])
		}
	})

	suite.Run("Registration with Weak Password", func() {
		defer suite.mockMailer.Reset()
		registerInput := RegisterInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Email      string `json:"email"`
					Name       string `json:"name"`
					Password   string `json:"password"`
					WithPortal bool   `json:"with_portal"`
					WithChat   bool   `json:"with_chat"`
				} `json:"attributes"`
			}{
				Type: "register",
				Attributes: struct {
					Email      string `json:"email"`
					Name       string `json:"name"`
					Password   string `json:"password"`
					WithPortal bool   `json:"with_portal"`
					WithChat   bool   `json:"with_chat"`
				}{
					Email:      "weakpass@example.com",
					Name:       "Weak Pass",
					Password:   "weak",
					WithPortal: true,
					WithChat:   true,
				},
			},
		}

		w := performRequest(suite.api.router, "POST", "/auth/register", registerInput)
		assert.Equal(suite.T(), http.StatusBadRequest, w.Code)

		var response ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Contains(suite.T(), response.Errors[0].Detail, "password must be at least 8 characters long")
	})
}

func (suite *AuthHandlersTestSuite) TestForgotPasswordHandler() {
	suite.Run("Successful Forgot Password Request", func() {
		defer suite.mockMailer.Reset()
		user := &models.User{Email: "forgetful@example.com", Password: "somehashedpassword"}
		suite.db.Create(user)

		forgotPasswordInput := ForgotPasswordInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Email string `json:"email"`
				} `json:"attributes"`
			}{
				Type: "forgot_password",
				Attributes: struct {
					Email string `json:"email"`
				}{
					Email: "forgetful@example.com",
				},
			},
		}

		w := performRequest(suite.api.router, "POST", "/auth/forgot-password", forgotPasswordInput)
		assert.Equal(suite.T(), http.StatusOK, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "Password reset email sent", response["message"])

		assert.Equal(suite.T(), 1, len(suite.mockMailer.sentEmails))
		if len(suite.mockMailer.sentEmails) > 0 {
			assert.Equal(suite.T(), "forgetful@example.com", suite.mockMailer.sentEmails[0].GetHeader("To")[0])
		}

	})

	suite.Run("Forgot Password for Non-existent User", func() {
		defer suite.mockMailer.Reset()
		forgotPasswordInput := ForgotPasswordInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Email string `json:"email"`
				} `json:"attributes"`
			}{
				Type: "forgot_password",
				Attributes: struct {
					Email string `json:"email"`
				}{
					Email: "nonexistent@example.com",
				},
			},
		}

		w := performRequest(suite.api.router, "POST", "/auth/forgot-password", forgotPasswordInput)
		assert.Equal(suite.T(), http.StatusInternalServerError, w.Code)

		var response ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "Internal Server Error", response.Errors[0].Title)
	})
}

func (suite *AuthHandlersTestSuite) TestResetPasswordHandler() {
	suite.Run("Successful Password Reset", func() {
		defer suite.mockMailer.Reset()
		user := &models.User{Email: "reset@example.com", Password: "oldhashed"}
		suite.db.Create(user)

		resetToken, _ := generateRandomToken()
		user.ResetToken = resetToken
		user.ResetTokenExpiry = time.Now().Add(time.Hour)
		suite.db.Save(user)

		resetPasswordInput := ResetPasswordInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Token    string `json:"token"`
					Password string `json:"password"`
				} `json:"attributes"`
			}{
				Type: "reset_password",
				Attributes: struct {
					Token    string `json:"token"`
					Password string `json:"password"`
				}{
					Token:    resetToken,
					Password: "NewPassword123!",
				},
			},
		}

		w := performRequest(suite.api.router, "POST", "/auth/reset-password", resetPasswordInput)
		assert.Equal(suite.T(), http.StatusOK, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "Password reset successful", response["message"])
	})

	suite.Run("Reset Password with Invalid Token", func() {
		defer suite.mockMailer.Reset()
		resetPasswordInput := ResetPasswordInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Token    string `json:"token"`
					Password string `json:"password"`
				} `json:"attributes"`
			}{
				Type: "reset_password",
				Attributes: struct {
					Token    string `json:"token"`
					Password string `json:"password"`
				}{
					Token:    "invalidtoken",
					Password: "NewPassword123!",
				},
			},
		}

		w := performRequest(suite.api.router, "POST", "/auth/reset-password", resetPasswordInput)
		assert.Equal(suite.T(), http.StatusBadRequest, w.Code)

		var response ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "Bad Request", response.Errors[0].Title)
		assert.Contains(suite.T(), response.Errors[0].Detail, "Invalid or expired reset token")
	})
}

func (suite *AuthHandlersTestSuite) TestVerifyEmailHandler() {
	suite.Run("Successful Email Verification", func() {
		defer suite.mockMailer.Reset()
		user := &models.User{Email: "verify@example.com", VerificationToken: "validtoken", EmailVerified: false}
		suite.db.Create(user)

		w := performRequest(suite.api.router, "GET", "/auth/verify-email?token=validtoken", nil)
		assert.Equal(suite.T(), http.StatusOK, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "Email verified successfully", response["message"])

		suite.db.First(user, user.ID)
		assert.True(suite.T(), user.EmailVerified)
	})

	suite.Run("Email Verification with Invalid Token", func() {
		defer suite.mockMailer.Reset()
		w := performRequest(suite.api.router, "GET", "/auth/verify-email?token=invalidtoken", nil)
		assert.Equal(suite.T(), http.StatusInternalServerError, w.Code)

		var response ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "Internal Server Error", response.Errors[0].Title)
	})
}

func (suite *AuthHandlersTestSuite) TestResendVerificationHandler() {
	suite.Run("Successful Resend Verification", func() {
		defer suite.mockMailer.Reset()
		user := &models.User{Email: "resend@example.com", EmailVerified: false}
		suite.db.Create(user)

		resendVerificationInput := ResendVerificationInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Email string `json:"email"`
				} `json:"attributes"`
			}{
				Type: "resend_verification",
				Attributes: struct {
					Email string `json:"email"`
				}{
					Email: "resend@example.com",
				},
			},
		}

		w := performRequest(suite.api.router, "POST", "/auth/resend-verification", resendVerificationInput)
		assert.Equal(suite.T(), http.StatusOK, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "Verification email resent", response["message"])

		assert.Equal(suite.T(), 1, len(suite.mockMailer.sentEmails))
		if len(suite.mockMailer.sentEmails) > 0 {
			assert.Equal(suite.T(), "resend@example.com", suite.mockMailer.sentEmails[0].GetHeader("To")[0])
		}
	})

	suite.Run("Resend Verification for Already Verified User", func() {
		defer suite.mockMailer.Reset()
		user := &models.User{Email: "alreadyverified@example.com", EmailVerified: true}
		suite.db.Create(user)

		resendVerificationInput := ResendVerificationInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Email string `json:"email"`
				} `json:"attributes"`
			}{
				Type: "resend_verification",
				Attributes: struct {
					Email string `json:"email"`
				}{
					Email: "alreadyverified@example.com",
				},
			},
		}

		w := performRequest(suite.api.router, "POST", "/auth/resend-verification", resendVerificationInput)
		assert.Equal(suite.T(), http.StatusInternalServerError, w.Code)

		var response ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "Internal Server Error", response.Errors[0].Title)
		assert.Contains(suite.T(), response.Errors[0].Detail, "email is already verified")
	})
}

func (suite *AuthHandlersTestSuite) TestLogoutHandler() {
	suite.Run("Successful Logout", func() {
		defer suite.mockMailer.Reset()
		user := &models.User{Email: "logout@example.com", SessionToken: "validtoken"}
		suite.db.Create(user)

		// Mock the gin.Context to include the user
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", user)

		suite.api.handleLogout(c)

		assert.Equal(suite.T(), http.StatusOK, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "Logout successful", response["message"])

		// Check if the session token is cleared
		suite.db.First(user, user.ID)
		assert.Empty(suite.T(), user.SessionToken)
	})
}
