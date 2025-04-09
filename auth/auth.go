package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/notifications"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Config struct {
	DB           *gorm.DB
	Service      services.ServiceInterface
	CookieName   string
	CookieSecure bool

	CookieDomain     string
	ResetTokenExpiry time.Duration
	FrontendURL      string

	CookieHTTPOnly         bool
	CookieSameSite         http.SameSite
	RegistrationAllowed    bool
	AdminEmail             string
	TestMode               bool
	AllowedRegisterDomains []string
	TIBAPISecret           string
	TIBEnabled             bool
}

// Ensure AuthService implements models.EmailSender
var _ models.EmailSender = (*AuthService)(nil)

type AuthService struct {
	Config              *Config
	DB                  *gorm.DB
	Service             services.ServiceInterface
	TokenStore          map[string]*models.User
	MailService         *notifications.MailService // Exported for testing
	NotificationService *services.NotificationService
}

func NewAuthService(config *Config, mailService *notifications.MailService, service services.ServiceInterface, notificationService *services.NotificationService) *AuthService {
	return &AuthService{
		Config:              config,
		MailService:         mailService,
		Service:             service,
		NotificationService: notificationService,
	}
}

func (a *AuthService) SetUserSession(c *gin.Context, user *models.User) error {
	token, err := a.generateToken()
	if err != nil {
		return err
	}

	expirationTime := time.Now().Add(6 * time.Hour) //TODO: get this from a config
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     a.Config.CookieName,
		Value:    token,
		Expires:  expirationTime,
		Secure:   a.Config.CookieSecure,
		HttpOnly: a.Config.CookieHTTPOnly,
		SameSite: a.Config.CookieSameSite,
		Path:     "/",
		Domain:   a.Config.CookieDomain,
	})

	user.SessionToken = token
	if err := user.Update(a.Config.DB); err != nil {
		return err
	}

	c.Set("user", user)
	return nil
}

func (a *AuthService) Login(c *gin.Context, email, password string) error {
	user, err := a.Config.Service.AuthenticateUser(email, password)
	if err != nil {
		if errors.Is(err, services.EmailNotVerifiedError) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Email unverified, please verify email or contact your administrator"})
			return fmt.Errorf("unauthorized: %w", err)
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return fmt.Errorf("unauthorized: %w", err)
	}

	return a.SetUserSession(c, user)
}

func (a *AuthService) GetAuthenticatedUser(c *gin.Context) *models.User {
	// Try to get auth from cookie first
	cookie, err := c.Cookie(a.Config.CookieName)
	if err == nil {
		// Cookie exists, validate it
		user := &models.User{}
		if err := a.Config.DB.Where("session_token = ?", cookie).First(user).Error; err == nil {
			return user
		}
	}

	// Try to get token from query parameter
	token := c.Query("token")
	if token != "" {
		user, err := a.Config.Service.GetUserByAPIKey(token)
		if err == nil && user.EmailVerified {
			return user
		}
	}

	// Try to get token from Authorization header
	authHeader := c.Request.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 {
			apiKey := parts[1]
			user, err := a.Config.Service.GetUserByAPIKey(apiKey)
			if err == nil && user.EmailVerified {
				return user
			}
		}
	}

	return nil
}

func (a *AuthService) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := a.GetAuthenticatedUser(c)
		if user != nil {
			c.Set("user", user)
			c.Next()
			return
		}

		// In test mode, allow the request to proceed
		if a.Config.TestMode {
			c.Next()
			return
		}

		// No valid authentication found
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
	}
}

func (a *AuthService) AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.Config.TestMode {
			c.Next()
			return
		}
		u, ok := c.Get("user")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		user := u.(*models.User)
		if !user.IsAdmin {
			slog.Error("user is not admin", "user", user.Name)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (a *AuthService) SSOOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.Config.TestMode {
			c.Next()
			return
		}

		u, ok := c.Get("user")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		user, ok := u.(*models.User)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		if !user.AccessToSSOConfig {
			slog.Error("user is not allowed", "user", user.Name)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
			c.Abort()
			return
		}

		c.Next()
	}
}
func (a *AuthService) LoadUserFromContext(c *gin.Context) (*models.User, error) {
	userInterface, exists := c.Get("user")
	if !exists {
		return nil, errors.New("user not found in context")
	}

	user, ok := userInterface.(*models.User)
	if !ok {
		return nil, errors.New("invalid user type in context")
	}

	return user, nil
}

func (a *AuthService) Logout(c *gin.Context) error {
	user, err := a.LoadUserFromContext(c)
	if err != nil {
		return err
	}

	user.SessionToken = ""
	user.Update(a.Config.DB)

	for _, cookie := range c.Request.Cookies() {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     cookie.Name,
			Value:    "",
			Expires:  time.Now().Add(-1 * time.Hour),
			Path:     "/",
			Domain:   a.Config.CookieDomain,
			Secure:   a.Config.CookieSecure,
			HttpOnly: a.Config.CookieHTTPOnly,
			SameSite: a.Config.CookieSameSite,
		})
	}

	return nil
}

func (a *AuthService) ResetPassword(email string) error {
	user := &models.User{}
	if err := user.GetByEmail(a.Config.DB, email); err != nil {
		return err
	}

	resetToken, err := a.generateToken()
	if err != nil {
		return err
	}

	user.ResetToken = resetToken
	user.ResetTokenExpiry = time.Now().Add(a.Config.ResetTokenExpiry)

	if err := user.Update(a.Config.DB); err != nil {
		return err
	}

	resetLink := fmt.Sprintf("%s/reset-password?token=%s", a.Config.FrontendURL, resetToken)

	emailBody := ""
	tmpl, err := template.ParseFiles("./templates/reset.tmpl")
	if err != nil {
		emailBody = fmt.Sprintf("Click the following link to reset your password: %s", resetLink)
	} else {
		var buf bytes.Buffer
		err = tmpl.Execute(&buf, map[string]string{
			"ResetLink": resetLink,
			"Name":      user.Name,
		})
		if err != nil {
			emailBody = fmt.Sprintf("Click the following link to reset your password: %s", resetLink)
		} else {
			emailBody = buf.String()
		}
	}

	if err := a.SendEmail(user.Email, "Password Reset", emailBody); err != nil {
		return fmt.Errorf("failed to send password reset email: %w", err)
	}

	return nil
}

func (a *AuthService) ValidateResetToken(token string) (*models.User, error) {
	user := &models.User{}
	if err := a.Config.DB.Where("reset_token = ?", token).First(user).Error; err != nil {
		return nil, err
	}

	if time.Now().After(user.ResetTokenExpiry) {
		return nil, errors.New("reset token has expired")
	}

	return user, nil
}

func (a *AuthService) UpdatePassword(user *models.User, oldPassword, newPassword string) error {
	if oldPassword == newPassword {
		return errors.New("new password must be different from the old password")
	}

	if err := a.ValidatePasswordComplexity(newPassword); err != nil {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.Password = string(hashedPassword)
	user.ResetToken = ""
	user.ResetTokenExpiry = time.Time{}

	return user.Update(a.Config.DB)
}

func (a *AuthService) ValidatePasswordComplexity(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters long")
	}

	hasUppercase := regexp.MustCompile(`[A-Z]`).MatchString(password)
	hasLowercase := regexp.MustCompile(`[a-z]`).MatchString(password)
	hasNumbers := regexp.MustCompile(`[0-9]`).MatchString(password)
	hasSpecialChars := regexp.MustCompile(`[!@#$%^&*(),.?":{}|<>]`).MatchString(password)

	if !hasUppercase || !hasLowercase || !hasNumbers || !hasSpecialChars {
		return errors.New("password must contain at least one uppercase letter, one lowercase letter, one number, and one special character")
	}

	return nil
}

func (a *AuthService) Register(email, name, password string, showPortal, showChat bool) error {
	if !a.Config.RegistrationAllowed {
		return errors.New("registration is currently disabled")
	}

	if len(a.Config.AllowedRegisterDomains) > 0 {
		parts := strings.Split(email, "@")
		if len(parts) != 2 {
			return fmt.Errorf("invalid email address")
		}

		domain := strings.Replace(parts[1], "@", "", 1)
		cont := false
		for _, allowed := range a.Config.AllowedRegisterDomains {
			if strings.ToLower(domain) == strings.ToLower(allowed) {
				cont = true
				break
			}
		}

		if !cont {
			return fmt.Errorf("registration is not permitted")
		}
	}

	existing, _ := a.Service.GetUserByEmail(email)
	if existing != nil {
		return errors.New("email already in use, please log in, verify emil, or reset password")
	}

	// Ensure default group exists
	defaultGroup, err := a.getDefaultGroup()
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			defaultGroup = &models.Group{
				Name: "Default",
			}
			if err := a.Config.DB.Create(defaultGroup).Error; err != nil {
				return fmt.Errorf("failed to create default group: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get default group: %w", err)
		}
	}

	if defaultGroup == nil {
		defaultGroup = &models.Group{
			Name: "Default",
		}
		if err := a.Config.DB.Create(defaultGroup).Error; err != nil {
			return fmt.Errorf("failed to create default group: %w", err)
		}
	}

	if err := a.ValidatePasswordComplexity(password); err != nil {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// Check if this is the first user
	var count int64
	if err := a.Config.DB.Model(&models.User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to count users: %w", err)
	}

	user := models.NewUser()
	user.Email = email
	user.Name = name
	user.Password = string(hashedPassword)
	user.ShowPortal = showPortal
	user.ShowChat = showChat

	if count == 0 {
		user.IsAdmin = true
		user.EmailVerified = true
		user.NotificationsEnabled = true
		user.AccessToSSOConfig = true
	}

	if err := user.Create(a.Config.DB); err != nil {
		return err
	}

	defaultGroup, err = a.getDefaultGroup()
	if err != nil {
		return fmt.Errorf("failed to get default group: %w", err)
	}

	if err := a.Config.Service.AddUserToGroup(user.ID, defaultGroup.ID); err != nil {
		return fmt.Errorf("failed to add user to default group: %w", err)
	}

	if err := a.sendVerificationEmail(user); err != nil {
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	if err := a.notifyAdmin(user); err != nil {
		// Log the error, but don't return it to prevent leaking information
		slog.Error("Failed to send admin notification:", "error", err)
	}

	return nil
}

func (a *AuthService) ResendVerificationEmail(email string) error {
	user := &models.User{}
	if err := user.GetByEmail(a.Config.DB, email); err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if user.EmailVerified {
		return errors.New("email is already verified")
	}

	verificationToken, err := a.generateToken()
	if err != nil {
		return fmt.Errorf("failed to generate verification token: %w", err)
	}

	user.VerificationToken = verificationToken
	if err := user.Update(a.Config.DB); err != nil {
		return fmt.Errorf("failed to update user with new verification token: %w", err)
	}

	verificationLink := fmt.Sprintf("%s/verify-email?token=%s", a.Config.FrontendURL, verificationToken)
	emailBody := fmt.Sprintf("Click the following link to verify your email: %s", verificationLink)

	if err := a.SendEmail(user.Email, "Email Verification", emailBody); err != nil {
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	return nil
}

func (a *AuthService) VerifyEmail(token string) error {
	user := &models.User{}
	if err := a.Config.DB.Where("verification_token = ?", token).First(user).Error; err != nil {
		return fmt.Errorf("failed to find user with verification token: %w", err)
	}

	if user.EmailVerified {
		return errors.New("email is already verified")
	}

	user.EmailVerified = true
	user.VerificationToken = ""

	if err := user.Update(a.Config.DB); err != nil {
		return fmt.Errorf("failed to update user after email verification: %w", err)
	}

	return nil
}

func (a *AuthService) generateToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (a *AuthService) getDefaultGroup() (*models.Group, error) {
	var group models.Group
	if err := a.Config.DB.Where("name LIKE ?", "%Default%").First(&group).Error; err != nil {
		return nil, err
	}

	return &group, nil
}

func (a *AuthService) sendVerificationEmail(user *models.User) error {
	verificationToken, err := a.generateToken()
	if err != nil {
		return err
	}

	user.VerificationToken = verificationToken
	if err := user.Update(a.Config.DB); err != nil {
		return err
	}

	verificationLink := fmt.Sprintf("%s/auth/verify-email?token=%s", a.Config.FrontendURL, verificationToken)
	emailBody := fmt.Sprintf("Click the following link to verify your email: %s", verificationLink)

	return a.SendEmail(user.Email, "Email Verification", emailBody)
}

func (a *AuthService) notifyAdmin(user *models.User) error {
	data := map[string]interface{}{
		"Name":  user.Name,
		"Email": user.Email,
	}

	notificationID := fmt.Sprintf("new_user_%d_%d", user.ID, time.Now().UnixNano())
	return a.NotificationService.Notify(notificationID, "New User Registration on AI Portal", "admin-notify.tmpl", data, models.NotifyAdmins)
}

func (a *AuthService) SendEmail(to, subject, body string) error {
	return a.MailService.SendEmail(to, subject, body)
}
