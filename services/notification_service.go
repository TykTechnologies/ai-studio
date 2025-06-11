package services

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/notifications"
	"gorm.io/gorm"
)

// NotificationService handles notification creation, storage, and delivery
type NotificationService struct {
	db          *gorm.DB
	fromEmail   string
	smtpHost    string
	smtpPort    int
	smtpUser    string
	smtpPass    string
	mailer      notifications.Mailer // Optional mailer for testing
	mailService *notifications.MailService
	// For testing purposes
	notifications []models.Notification
	mu            sync.RWMutex
}

// Notify creates and sends a notification using a template
// userFlags can be a specific user ID, models.NotifyAdmins, or a combination using bitwise OR (|)
func (s *NotificationService) Notify(notificationID string, title string, templatePath string, data interface{}, userFlags uint) error {
	// Render the template
	content, err := s.renderTemplate(templatePath, data)
	if err != nil {
		return fmt.Errorf("error rendering template: %v", err)
	}

	// Handle notifications based on flags
	if userFlags&models.NotifyAdmins != 0 {
		// Send to admin users
		var adminIDs []uint
		if err := s.db.Model(&models.User{}).
			Where("is_admin = ? AND notifications_enabled = ?", true, true).
			Pluck("id", &adminIDs).Error; err != nil {
			return fmt.Errorf("error finding admin users: %v", err)
		}

		// Send to each admin
		for _, adminID := range adminIDs {
			notification := &models.Notification{
				UserID:         adminID,
				Title:          title,
				Content:        content,
				NotificationID: fmt.Sprintf("%s_admin_%d", notificationID, adminID),
				SentAt:         time.Now(),
			}
			if err := s.Send(notification); err != nil {
				// Log error but continue with other admins
				fmt.Printf("Error sending notification to admin %d: %v\n", adminID, err)
			}
		}
	}

	// Send to specific user if a user ID is provided
	userID := userFlags &^ models.NotifyAdmins // Clear the admin flag to get the user ID
	if userID != 0 {
		notification := &models.Notification{
			UserID:         userID,
			Title:          title,
			Content:        content,
			NotificationID: fmt.Sprintf("%s_owner", notificationID),
			SentAt:         time.Now(),
		}
		if err := s.Send(notification); err != nil {
			return fmt.Errorf("failed to send user notification: %v", err)
		}
	}

	return nil
}

// NewNotificationService creates a new notification service
func NewNotificationService(db *gorm.DB, fromEmail, smtpHost string, smtpPort int, smtpUser, smtpPass string, mailer notifications.Mailer) *NotificationService {
	ns := &NotificationService{
		db:            db,
		fromEmail:     fromEmail,
		smtpHost:      smtpHost,
		smtpPort:      smtpPort,
		smtpUser:      smtpUser,
		smtpPass:      smtpPass,
		mailer:        mailer,
		notifications: make([]models.Notification, 0),
	}

	// Initialize mail service if mailer is provided
	if mailer != nil {
		ns.mailService = notifications.NewMailService(
			fromEmail,
			smtpHost,
			smtpPort,
			smtpUser,
			smtpPass,
			mailer,
			false,
		)
	}

	return ns
}

// GetNotifications returns all stored notifications (for testing)
func (s *NotificationService) GetNotifications() []models.Notification {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.notifications
}

// ClearNotifications clears all stored notifications (for testing)
func (s *NotificationService) ClearNotifications() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifications = make([]models.Notification, 0)
}

// send creates and sends a notification, preventing duplicates based on NotificationID
func (s *NotificationService) Send(notification *models.Notification) error {
	// For testing purposes
	s.mu.Lock()
	s.notifications = append(s.notifications, *notification)
	s.mu.Unlock()

	// Check for existing notification with same ID
	var existingNotification models.Notification
	result := s.db.Where("notification_id = ?", notification.NotificationID).First(&existingNotification)
	if result.Error == nil {
		// Notification already exists, skip
		return nil
	} else if result.Error != gorm.ErrRecordNotFound {
		// Unexpected error
		return fmt.Errorf("error checking for existing notification: %v", result.Error)
	}

	// Set sent time if not already set
	if notification.SentAt.IsZero() {
		notification.SentAt = time.Now()
	}

	// Store notification in database
	if err := s.db.Create(notification).Error; err != nil {
		return fmt.Errorf("error creating notification: %v", err)
	}

	// Send email if mail service is configured
	if s.mailService != nil {
		var email string
		if err := s.db.Model(&models.User{}).
			Where("id = ?", notification.UserID).
			Pluck("email", &email).Error; err != nil {
			return fmt.Errorf("error finding user email: %v", err)
		}

		if err := s.mailService.SendEmail(email, notification.Title, notification.Content); err != nil {
			// Log error but don't fail the notification creation
			fmt.Printf("Error sending email notification: %v\n", err)
		}
	}

	return nil
}

// MarkAsRead marks a notification as read
func (s *NotificationService) MarkAsRead(notificationID uint) error {
	result := s.db.Model(&models.Notification{}).
		Where("id = ?", notificationID).
		Update("read", true)

	if result.Error != nil {
		return fmt.Errorf("error marking notification as read: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("notification not found")
	}
	return nil
}

// GetUserNotifications retrieves notifications for a specific user
func (s *NotificationService) GetUserNotifications(userID uint, limit, offset int) ([]models.Notification, error) {
	var notifications []models.Notification
	result := s.db.Where("user_id = ?", userID).
		Order("sent_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&notifications)

	if result.Error != nil {
		return nil, fmt.Errorf("error retrieving notifications: %v", result.Error)
	}
	return notifications, nil
}

// GetUnreadCount returns the number of unread notifications for a user
func (s *NotificationService) GetUnreadCount(userID uint) (int64, error) {
	var count int64
	result := s.db.Model(&models.Notification{}).
		Where("user_id = ? AND read = ?", userID, false).
		Count(&count)

	if result.Error != nil {
		return 0, fmt.Errorf("error counting unread notifications: %v", result.Error)
	}
	return count, nil
}

// GetMailer returns the mailer for testing purposes
func (s *NotificationService) GetMailer() notifications.Mailer {
	return s.mailer
}

// renderTemplate renders a template with the given data
func (s *NotificationService) renderTemplate(templateName string, data interface{}) (string, error) {
	// Define the formatDate function
	formatDate := func(t time.Time) string {
		return t.Format("January 2, 2006")
	}

	funcMap := template.FuncMap{
		"formatDate": formatDate,
	}

	// Get the base name of the template for template.New()
	baseName := filepath.Base(templateName)

	// First try the full path if provided
	// Use New().Funcs().ParseFiles() to include the custom function
	tmpl, err := template.New(baseName).Funcs(funcMap).ParseFiles(templateName)
	if err != nil {
		// If that fails, try to find the templates directory by walking up
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("error getting working directory: %v", err)
		}

		// Walk up the directory tree until we find the templates directory
		templatePath := filepath.Join("templates", templateName)
		currentDir := wd
		for {
			tryPath := filepath.Join(currentDir, templatePath)
			if _, err := os.Stat(tryPath); err == nil {
				tmpl, err = template.New(baseName).Funcs(funcMap).ParseFiles(tryPath) // Use New().Funcs() here too
				if err != nil {
					return "", fmt.Errorf("error parsing template: %v", err)
				}
				break
			}
			parent := filepath.Dir(currentDir)
			if parent == currentDir {
				// We've reached the root directory
				return "", fmt.Errorf("could not find template %s in templates directory", templateName)
			}
			currentDir = parent
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing template: %v", err)
	}

	return buf.String(), nil
}
