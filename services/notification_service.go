package services

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// NotifyOptions refines how a notification is recorded and delivered.
type NotifyOptions struct {
	// Type is stored on the record (e.g. "app", "user", "submission") and
	// may be empty.
	Type string
	// Link is the in-app path the notification points at ("/admin/apps/3").
	// Empty when there is nothing to open.
	Link string
	// ActorID is the user who performed the action. They are never a
	// recipient: an administrator does not need to be told about the app they
	// just created, and a user does not need to be told they registered.
	ActorID uint
	// Summary is the plain-text in-app content. When empty, the content
	// passed to NotifyWithOptions is used after stripEmailFraming.
	Summary string
	// SkipEmail records the notification without sending an email, for
	// callers that have already mailed the recipient through another path.
	SkipEmail bool
}

// Notify creates and sends a notification using a template
// userFlags can be a specific user ID, models.NotifyAdmins, or a combination using bitwise OR (|)
func (s *NotificationService) Notify(notificationID string, title string, templatePath string, data interface{}, userFlags uint) error {
	return s.NotifyTemplate(notificationID, title, templatePath, data, userFlags, NotifyOptions{})
}

// NotifyTemplate renders an email template and delivers it. The rendered
// template is the email body; the in-app record gets opts.Summary, or the
// rendered body with its email framing stripped when no summary is given.
func (s *NotificationService) NotifyTemplate(notificationID string, title string, templatePath string, data interface{}, userFlags uint, opts NotifyOptions) error {
	// Render the template
	content, err := s.renderTemplate(templatePath, data)
	if err != nil {
		return fmt.Errorf("error rendering template: %v", err)
	}

	return s.NotifyWithOptions(notificationID, title, content, userFlags, opts)
}

// NotifyDirect creates and sends a notification with pre-rendered content
// (plain text or markdown; the in-app list renders markdown). Use this when no
// email template applies, e.g. for notifications raised by plugins or by
// workflows whose content is composed in code.
//
// notifType is stored on the record (e.g. "submission", "plugin:asset-catalog")
// and may be empty. userFlags follows the same convention as Notify.
func (s *NotificationService) NotifyDirect(notificationID string, notifType string, title string, content string, userFlags uint) error {
	return s.NotifyWithOptions(notificationID, title, content, userFlags, NotifyOptions{Type: notifType})
}

// NotifyWithOptions is the delivery core behind Notify, NotifyTemplate and
// NotifyDirect. content is the email body; the in-app record stores
// opts.Summary when given, otherwise content with its email framing removed.
// The actor named in opts never receives a copy.
func (s *NotificationService) NotifyWithOptions(notificationID string, title string, content string, userFlags uint, opts NotifyOptions) error {
	if notificationID == "" {
		return fmt.Errorf("notification ID is required")
	}
	if title == "" {
		return fmt.Errorf("notification title is required")
	}

	inApp := opts.Summary
	if inApp == "" {
		inApp = stripEmailFraming(content)
	}
	// In-app records are plain text. Object names (apps, users, submissions)
	// are user-supplied and end up interpolated here, so any markup is
	// dropped before storage as defence in depth; the UI renders these as
	// text as well.
	inApp = stripMarkup(inApp)
	title = stripMarkup(title)

	// Handle notifications based on flags
	if userFlags&models.NotifyAdmins != 0 {
		// Send to admin users
		var adminIDs []uint
		if err := s.db.Model(&models.User{}).
			Where("is_admin = ? AND notifications_enabled = ? AND disabled = ?", true, true, false).
			Pluck("id", &adminIDs).Error; err != nil {
			return fmt.Errorf("error finding admin users: %v", err)
		}

		// Send to each admin
		for _, adminID := range adminIDs {
			if adminID == opts.ActorID {
				// The admin did this themselves; there is nothing to tell them.
				continue
			}
			notification := &models.Notification{
				UserID:         adminID,
				Type:           opts.Type,
				Title:          title,
				Content:        inApp,
				Link:           opts.Link,
				NotificationID: fmt.Sprintf("%s_admin_%d", notificationID, adminID),
				SentAt:         time.Now(),
			}
			if err := s.deliver(notification, content, opts.SkipEmail); err != nil {
				// Log error but continue with other admins
				fmt.Printf("Error sending notification to admin %d: %v\n", adminID, err)
			}
		}
	}

	// Send to specific user if a user ID is provided
	userID := userFlags &^ models.NotifyAdmins // Clear the admin flag to get the user ID
	if userID != 0 && userID != opts.ActorID {
		notification := &models.Notification{
			UserID:         userID,
			Type:           opts.Type,
			Title:          title,
			Content:        inApp,
			Link:           opts.Link,
			NotificationID: fmt.Sprintf("%s_owner", notificationID),
			SentAt:         time.Now(),
		}
		if err := s.deliver(notification, content, opts.SkipEmail); err != nil {
			return fmt.Errorf("failed to send user notification: %v", err)
		}
	}

	return nil
}

// Email framing that has no place in an in-app notification. The templates
// are written as emails ("Subject: ...", "Dear Administrator,", a sign-off);
// the bell shows the same record, where that reads as noise.
var (
	emailSubjectLine  = regexp.MustCompile(`^\s*Subject:`)
	emailGreetingLine = regexp.MustCompile(`^\s*(Dear|Hi|Hello)\b[^\n]{0,60}[,:]?\s*$`)
	emailSignOffLine  = regexp.MustCompile(`^\s*(Best regards|Kind regards|Warm regards|Regards|Sincerely|Thanks|Thank you|Cheers)\s*,?\s*$`)
	emailBoilerplate  = regexp.MustCompile(`^\s*(This is an automated (notification|message|email)|Please do not reply)`)
	blankRuns         = regexp.MustCompile(`\n{3,}`)
)

// htmlTag matches anything that looks like an HTML/XML tag, including
// unterminated ones at the end of the text.
var htmlTag = regexp.MustCompile(`<[^>]*>?`)

// stripMarkup removes HTML tags from text destined for an in-app
// notification. It is deliberately blunt: notifications never carry
// legitimate markup, and a user-chosen object name has no business
// contributing any.
func stripMarkup(text string) string {
	if !strings.Contains(text, "<") {
		return text
	}
	return strings.TrimSpace(htmlTag.ReplaceAllString(text, ""))
}

// stripEmailFraming turns an email body into in-app content: the subject
// line, the greeting, the sign-off and everything after it, and "do not
// reply" boilerplate are dropped. Content that carries none of these (the
// markdown composed for NotifyDirect) passes through unchanged apart from
// trimming.
func stripEmailFraming(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	atTop := true
	for _, line := range lines {
		if atTop {
			if strings.TrimSpace(line) == "" || emailSubjectLine.MatchString(line) || emailGreetingLine.MatchString(line) {
				continue
			}
			atTop = false
		}
		if emailSignOffLine.MatchString(line) {
			break
		}
		if emailBoilerplate.MatchString(line) {
			continue
		}
		kept = append(kept, line)
	}
	out := strings.Join(kept, "\n")
	out = blankRuns.ReplaceAllString(out, "\n\n")
	return strings.TrimSpace(out)
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

// Send creates and sends a notification, preventing duplicates based on
// NotificationID. The email body is the notification content.
func (s *NotificationService) Send(notification *models.Notification) error {
	return s.deliver(notification, notification.Content, false)
}

// deliver stores the notification and, unless skipEmail is set, emails
// emailBody to the recipient. emailBody may differ from the stored content:
// the in-app record carries a plain summary while the email keeps the full
// template. Duplicates by NotificationID are skipped.
func (s *NotificationService) deliver(notification *models.Notification, emailBody string, skipEmail bool) error {
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
	if s.mailService != nil && !skipEmail {
		var email string
		if err := s.db.Model(&models.User{}).
			Where("id = ?", notification.UserID).
			Pluck("email", &email).Error; err != nil {
			return fmt.Errorf("error finding user email: %v", err)
		}

		if err := s.mailService.SendEmail(email, notification.Title, emailBody); err != nil {
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

// MarkAllAsRead marks all unread notifications as read for a user
func (s *NotificationService) MarkAllAsRead(userID uint) error {
	result := s.db.Model(&models.Notification{}).
		Where("user_id = ? AND read = ?", userID, false).
		Update("read", true)

	if result.Error != nil {
		return fmt.Errorf("error marking all notifications as read: %v", result.Error)
	}
	return nil
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
