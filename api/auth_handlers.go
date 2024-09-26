package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// @Summary Login user
// @Description Authenticate a user and return a session token
// @Tags auth
// @Accept json
// @Produce json
// @Param user body LoginInput true "Login credentials"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /auth/login [post]
func (a *API) handleLogin(c *gin.Context) {
	var input LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err := a.auth.Login(c, input.Data.Attributes.Email, input.Data.Attributes.Password)
	if err != nil {
		return
	}

	u, err := a.service.GetUserByEmail(input.Data.Attributes.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Login successful", "is_admin": u.IsAdmin})
}

// @Summary Register user
// @Description Register a new user
// @Tags auth
// @Accept json
// @Produce json
// @Param user body RegisterInput true "User registration details"
// @Success 201 {object} RegisterResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/register [post]
func (a *API) handleRegister(c *gin.Context) {
	var input RegisterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err := a.auth.Register(input.Data.Attributes.Email, input.Data.Attributes.Name, input.Data.Attributes.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "User registered successfully"})
}

// @Summary Logout user
// @Description Log out the current user
// @Tags auth
// @Accept json
// @Produce json
// @Success 200 {object} LogoutResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/logout [post]
// @Security BearerAuth
func (a *API) handleLogout(c *gin.Context) {
	err := a.auth.Logout(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logout successful"})
}

// @Summary Forgot password
// @Description Request a password reset
// @Tags auth
// @Accept json
// @Produce json
// @Param email body ForgotPasswordInput true "User's email"
// @Success 200 {object} ForgotPasswordResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/forgot-password [post]
func (a *API) handleForgotPassword(c *gin.Context) {
	var input ForgotPasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err := a.auth.ResetPassword(input.Data.Attributes.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password reset email sent"})
}

// @Summary Reset password
// @Description Reset user's password using a token
// @Tags auth
// @Accept json
// @Produce json
// @Param reset body ResetPasswordInput true "Password reset details"
// @Success 200 {object} ResetPasswordResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/reset-password [post]
func (a *API) handleResetPassword(c *gin.Context) {
	var input ResetPasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	user, err := a.auth.ValidateResetToken(input.Data.Attributes.Token)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid or expired reset token"}},
		})
		return
	}

	err = a.auth.UpdatePassword(user, "", input.Data.Attributes.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password reset successful"})
}

// @Summary Verify email
// @Description Verify user's email using a token
// @Tags auth
// @Accept json
// @Produce json
// @Param token query string true "Verification token"
// @Success 200 {object} VerifyEmailResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/verify-email [get]
func (a *API) handleVerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Verification token is required"}},
		})
		return
	}

	err := a.auth.VerifyEmail(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Email verified successfully"})
}

// @Summary Resend verification email
// @Description Resend the email verification link
// @Tags auth
// @Accept json
// @Produce json
// @Param email body ResendVerificationInput true "User's email"
// @Success 200 {object} ResendVerificationResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/resend-verification [post]
func (a *API) handleResendVerification(c *gin.Context) {
	var input ResendVerificationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err := a.auth.ResendVerificationEmail(input.Data.Attributes.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Verification email resent"})
}

// @Summary Get current user
// @Description Get the details of the currently logged-in user
// @Tags auth
// @Accept json
// @Produce json
// @Success 200 {object} UserResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/v1/me [get]
// @Security BearerAuth
func (a *API) handleMe(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.Status(http.StatusUnauthorized)
		return
	}

	u, ok := user.(*models.User)
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}

	response := UserResponse{
		Type: "user",
		ID:   strconv.Itoa(int(u.ID)),
		Attributes: struct {
			Email   string `json:"email"`
			Name    string `json:"name"`
			IsAdmin bool   `json:"is_admin"`
		}{
			Email:   u.Email,
			Name:    u.Name,
			IsAdmin: u.IsAdmin,
		},
	}

	c.JSON(http.StatusOK, response)
}
