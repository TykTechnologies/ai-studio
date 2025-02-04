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
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/go-mail/mail"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Dialer func(host string, port int, username, password string) *mail.Dialer
type Mailer interface {
	DialAndSend(m ...*mail.Message) error
}

type MailDialer interface {
	DialAndSend(m *mail.Message) error
}

type Config struct {
	DB           *gorm.DB
	Service      *services.Service
	CookieName   string
	CookieSecure bool

	CookieDomain     string
	ResetTokenExpiry time.Duration
	FrontendURL      string

	CookieHTTPOnly         bool
	CookieSameSite         http.SameSite
	RegistrationAllowed    bool
	AdminEmail             string
	FromEmail              string
	SMTPHost               string
	SMTPPort               int
	SMTPUsername           string
	SMTPPassword           string
	TestMode               bool
	AllowedRegisterDomains []string
}

type AuthService struct {
	Config     *Config
	DB         *gorm.DB
	Service    *services.Service
	TokenStore map[string]*models.User
	Mailer     Mailer
}

func NewAuthService(config *Config, mailer Mailer, service *services.Service) *AuthService {
	return &AuthService{
		Config:  config,
		Mailer:  mailer,
		Service: service,
	}
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

	token, err := a.generateToken()
	if err != nil {
		return err
	}

	expirationTime := time.Now().Add(1 * time.Hour)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     a.Config.CookieName,
		Value:    token,
		Expires:  expirationTime,
		Secure:   a.Config.CookieSecure,
		HttpOnly: a.Config.CookieHTTPOnly,
		SameSite: a.Config.CookieSameSite,
		Path:     "/",
	})

	user.SessionToken = token
	if err := user.Update(a.Config.DB); err != nil {
		return err
	}

	c.Set("user", user)
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

	user := &models.User{
		Email:      email,
		Name:       name,
		Password:   string(hashedPassword),
		ShowPortal: showPortal,
		ShowChat:   showChat,
	}

	if count == 0 {
		user.IsAdmin = true
		user.EmailVerified = true
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
		fmt.Printf("Failed to send admin notification: %v\n", err)
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
	subject := "New User Registration"
	body := fmt.Sprintf("A new user has registered:\nName: %s\nEmail: %s", user.Name, user.Email)
	tmpl, err := template.ParseFiles("./templates/admin-notify.tmpl")
	if err != nil {
		slog.Error("Failed to parse admin notification template", "error", err)
	} else {
		var buf bytes.Buffer
		err = tmpl.Execute(&buf, map[string]string{
			"Name":  user.Name,
			"Email": user.Email,
		})
		if err != nil {
			slog.Error("Failed to execute admin notification template", "error", err)
		} else {
			body = buf.String()
		}
	}

	return a.SendEmail(a.Config.AdminEmail, subject, body)
}
func (a *AuthService) SendEmail(to, subject, body string) error {
	fmt.Println("Sending email to: ", to)
	m := mail.NewMessage()
	m.SetHeader("From", a.Config.FromEmail)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	if a.Config.SMTPHost == "" {
		slog.Warn("smtp host not set, not sending email")
		return nil
	}
	return a.Mailer.DialAndSend(m)
}

func (a *AuthService) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.Config.TestMode {
			c.Next()
			return
		}

		cookie, err := c.Cookie(a.Config.CookieName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		user := &models.User{}
		if err := a.Config.DB.Where("session_token = ?", cookie).First(user).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		c.Set("user", user)
		c.Next()
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
			fmt.Println("User is not admin")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
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
	if err := user.Update(a.Config.DB); err != nil {
		return err
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     a.Config.CookieName,
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		Secure:   a.Config.CookieSecure,
		HttpOnly: a.Config.CookieHTTPOnly,
		SameSite: a.Config.CookieSameSite,
		Path:     "/",
		Domain:   a.Config.CookieDomain,
	})

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
