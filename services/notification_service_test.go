package services_test

import (
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/notifications"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
)

func TestSendAdminAppNotification(t *testing.T) {
	t.Run("basic notification test", func(t *testing.T) {
		// Setup test DB using existing helper
		db := apitest.SetupTestDB(t)
		testMailer := notifications.NewTestMailer()
		mailService := notifications.NewMailService("test@example.com", "localhost", 25, "testuser", "testpass", testMailer)
		notificationService := services.NewNotificationService(db, mailService)
		service := &services.Service{
			DB:                  db,
			NotificationService: notificationService,
			Budget:              services.NewBudgetService(db, notificationService),
		}

		// Create admin users with different notification settings
		admin1 := &models.User{
			Email:                "admin1@test.com",
			Name:                 "Admin 1",
			IsAdmin:              true,
			NotificationsEnabled: true,
			EmailVerified:        true,
		}
		err := admin1.Create(db)
		assert.NoError(t, err)

		admin2 := &models.User{
			Email:                "admin2@test.com",
			Name:                 "Admin 2",
			IsAdmin:              true,
			NotificationsEnabled: false,
			EmailVerified:        true,
		}
		err = admin2.Create(db)
		assert.NoError(t, err)

		regularUser := &models.User{
			Email:                "user@test.com",
			Name:                 "Regular User",
			IsAdmin:              false,
			NotificationsEnabled: true,
			EmailVerified:        true,
		}
		err = regularUser.Create(db)
		assert.NoError(t, err)

		// Send admin notification
		title := "Test Notification"
		content := "Test Content"
		err = service.NotificationService.SendAdminAppNotification(title, content)
		assert.NoError(t, err)

		// Get notifications from the test service
		notifications := service.NotificationService.GetNotifications()

		// Verify only admin1 (with notifications enabled) received the notification
		assert.Len(t, notifications, 1, "Expected exactly one notification")
		if len(notifications) > 0 {
			assert.Equal(t, admin1.ID, notifications[0].UserID, "Notification should be sent to admin1")
			assert.Equal(t, title, notifications[0].Title, "Notification title should match")
			assert.Equal(t, content, notifications[0].Content, "Notification content should match")
		}

		// Verify admin2 (notifications disabled) and regular user did not receive notifications
		for _, notification := range notifications {
			assert.NotEqual(t, admin2.ID, notification.UserID, "Admin2 should not receive notification")
			assert.NotEqual(t, regularUser.ID, notification.UserID, "Regular user should not receive notification")
		}

		// Verify emails sent through the mailer
		sentEmails := testMailer.GetEmails()
		assert.Len(t, sentEmails, 1, "Expected exactly one email")
		if len(sentEmails) > 0 {
			assert.Equal(t, "admin1@test.com", sentEmails[0].To)
		}
	})

	t.Run("config admin email matches admin user", func(t *testing.T) {
		db := apitest.SetupTestDB(t)
		testMailer := notifications.NewTestMailer()
		mailService := notifications.NewMailService("test@example.com", "localhost", 25, "testuser", "testpass", testMailer)
		notificationService := services.NewNotificationService(db, mailService)
		service := &services.Service{
			DB:                  db,
			NotificationService: notificationService,
			Budget:              services.NewBudgetService(db, notificationService),
		}

		// Create admin user with email matching config
		adminUser := &models.User{
			Email:                "admin@test.com",
			Name:                 "Admin",
			IsAdmin:              true,
			NotificationsEnabled: true,
			EmailVerified:        true,
		}
		err := adminUser.Create(db)
		assert.NoError(t, err)

		// Set config admin email to match admin user and reset config
		t.Setenv("ADMIN_EMAIL", "admin@test.com")
		config.Get().AdminEmail = "admin@test.com"

		// Send notification
		err = service.NotificationService.SendAdminAppNotification("Test", "Content")
		assert.NoError(t, err)

		// Verify only one notification/email was sent (to admin user)
		// and not duplicated to config admin email
		sentEmails := testMailer.GetEmails()
		assert.Len(t, sentEmails, 1, "Expected exactly one email")
		if len(sentEmails) > 0 {
			assert.Equal(t, "admin@test.com", sentEmails[0].To)
		}
	})

	t.Run("config admin email differs from admin users", func(t *testing.T) {
		db := apitest.SetupTestDB(t)
		testMailer := notifications.NewTestMailer()
		mailService := notifications.NewMailService("test@example.com", "localhost", 25, "testuser", "testpass", testMailer)
		notificationService := services.NewNotificationService(db, mailService)
		service := &services.Service{
			DB:                  db,
			NotificationService: notificationService,
			Budget:              services.NewBudgetService(db, notificationService),
		}

		// Create admin user with different email
		adminUser := &models.User{
			Email:                "admin@test.com",
			Name:                 "Admin",
			IsAdmin:              true,
			NotificationsEnabled: true,
			EmailVerified:        true,
		}
		err := adminUser.Create(db)
		assert.NoError(t, err)

		// Set different config admin email and reset config
		t.Setenv("ADMIN_EMAIL", "different@test.com")
		config.Get().AdminEmail = "different@test.com"

		// Send notification
		err = service.NotificationService.SendAdminAppNotification("Test", "Content")
		assert.NoError(t, err)

		// Verify emails were sent to both admin user and config admin email
		sentEmails := testMailer.GetEmails()
		assert.Len(t, sentEmails, 2, "Expected two emails")

		// Verify recipients
		emailRecipients := []string{sentEmails[0].To, sentEmails[1].To}
		assert.Contains(t, emailRecipients, "admin@test.com")
		assert.Contains(t, emailRecipients, "different@test.com")
	})
}
