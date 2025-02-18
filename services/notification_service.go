package services

import (
	"fmt"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/notifications"
	"gorm.io/gorm"
)

// NotificationService handles notification creation, storage, and delivery
type NotificationService struct {
	db          *gorm.DB
	mailService *notifications.MailService
	// For testing purposes
	notifications []models.Notification
	mu            sync.RWMutex
}

// NewNotificationService creates a new notification service
func NewNotificationService(db *gorm.DB, mailService *notifications.MailService) *NotificationService {
	return &NotificationService{
		db:            db,
		mailService:   mailService,
		notifications: make([]models.Notification, 0),
	}
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

// Send creates and sends a notification, preventing duplicates based on NotificationID
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

	// Get user's email
	var user models.User
	if err := s.db.First(&user, notification.UserID).Error; err != nil {
		return fmt.Errorf("error finding user: %v", err)
	}

	// Store notification in database
	if err := s.db.Create(notification).Error; err != nil {
		return fmt.Errorf("error creating notification: %v", err)
	}

	// Send email if mail service is configured
	if s.mailService != nil {
		if err := s.mailService.SendEmail(user.Email, notification.Title, notification.Content); err != nil {
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

// SendAdminAppNotification sends a notification to all admin users who have notifications enabled
// and also sends an email to the admin email from config for backward compatibility
func (s *NotificationService) SendAdminAppNotification(title, content string) error {
	// Find all admin users with notifications enabled
	var adminUsers []models.User
	if err := s.db.Where("is_admin = ? AND notifications_enabled = ?", true, true).Find(&adminUsers).Error; err != nil {
		return fmt.Errorf("error finding admin users: %v", err)
	}

	// Send notifications to admin users
	for _, admin := range adminUsers {
		notification := &models.Notification{
			UserID:         admin.ID,
			Title:          title,
			Content:        content,
			NotificationID: fmt.Sprintf("admin_app_%d_%d", admin.ID, time.Now().UnixNano()),
			SentAt:         time.Now(),
		}
		if err := s.Send(notification); err != nil {
			// Log error but continue with other admins
			fmt.Printf("Error sending notification to admin %s: %v\n", admin.Email, err)
		}
	}

	// For backward compatibility, also send email to admin email from config if set and not already included
	if s.mailService != nil {
		adminEmail := config.Get().AdminEmail
		if adminEmail != "" {
			// Check if adminEmail is not already in the list of admin users
			alreadyIncluded := false
			for _, admin := range adminUsers {
				if admin.Email == adminEmail {
					alreadyIncluded = true
					break
				}
			}

			if !alreadyIncluded {
				if err := s.mailService.SendEmail(adminEmail, title, content); err != nil {
					// Log error but don't fail the entire operation
					fmt.Printf("Error sending email to admin email %s: %v\n", adminEmail, err)
				}
			}
		}
	}

	return nil
}
