package services_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
)

func TestNotify(t *testing.T) {
	// Create a temporary template file for testing
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "admin-notify.tmpl")
	err := os.WriteFile(templatePath, []byte("Name: {{.Name}}\nEmail: {{.Email}}"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test template: %v", err)
	}

	t.Run("flag-based notification test", func(t *testing.T) {
		// Setup test DB using existing helper
		db := apitest.SetupTestDB(t)
		notificationService := services.NewTestNotificationService(db)
		service := &services.Service{
			DB:                  db,
			NotificationService: notificationService,
			Budget:              budget.NewService(db, notificationService),
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

		// Test sending to both user and admins using flag
		err = service.NotificationService.Notify(
			fmt.Sprintf("test_notify_%d_%d", regularUser.ID, time.Now().UnixNano()),
			"Test Notification",
			templatePath,
			map[string]interface{}{
				"Name":  "Regular User",
				"Email": regularUser.Email,
			},
			regularUser.ID|models.NotifyAdmins,
		)
		assert.NoError(t, err)

		// Get notifications from the test service
		notifications := service.NotificationService.GetNotifications()

		// Should have 2 notifications: 1 for regular user, 1 for admin1 (admin2 has notifications disabled)
		assert.Len(t, notifications, 2, "Expected two notifications")

		// Verify both regular user and admin received notifications
		var foundRegularUser bool
		var foundAdmin bool
		for _, n := range notifications {
			if n.UserID == regularUser.ID {
				foundRegularUser = true
				assert.Contains(t, n.Content, "Name: Regular User")
				assert.Contains(t, n.Content, "Email: user@test.com")
			}
			if n.UserID == admin1.ID {
				foundAdmin = true
				assert.Contains(t, n.Content, "Name: Regular User")
				assert.Contains(t, n.Content, "Email: user@test.com")
			}
		}
		assert.True(t, foundRegularUser, "Regular user should receive notification")
		assert.True(t, foundAdmin, "Admin1 should receive notification")

		// Verify admin2 (notifications disabled) did not receive notification
		for _, notification := range notifications {
			assert.NotEqual(t, admin2.ID, notification.UserID, "Admin2 should not receive notification")
		}

		// Get the test mailer and verify emails
		if testMailer, ok := notificationService.GetMailer().(interface {
			GetEmails() []struct {
				To string
			}
		}); ok {
			sentEmails := testMailer.GetEmails()
			// Verify emails were sent to the correct recipients
			if assert.Len(t, sentEmails, 2, "Expected two emails") {
				emailRecipients := []string{sentEmails[0].To, sentEmails[1].To}
				assert.Contains(t, emailRecipients, "admin1@test.com")
				assert.Contains(t, emailRecipients, "user@test.com")
			}
		}
	})

	t.Run("basic notification test", func(t *testing.T) {
		// Setup test DB using existing helper
		db := apitest.SetupTestDB(t)
		notificationService := services.NewTestNotificationService(db)
		service := &services.Service{
			DB:                  db,
			NotificationService: notificationService,
			Budget:              budget.NewService(db, notificationService),
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

		// Test sending to specific user
		err = service.NotificationService.Notify(
			fmt.Sprintf("test_notify_%d_%d", regularUser.ID, time.Now().UnixNano()),
			"Test User Notification",
			templatePath,
			map[string]interface{}{
				"Name":  "Regular User",
				"Email": regularUser.Email,
			},
			regularUser.ID,
		)
		assert.NoError(t, err)

		// Test sending to admins
		err = service.NotificationService.Notify(
			fmt.Sprintf("test_notify_admin_%d", time.Now().UnixNano()),
			"Test Admin Notification",
			templatePath,
			map[string]interface{}{
				"Name":  "Admin User",
				"Email": "admin@test.com",
			},
			models.NotifyAdmins,
		)
		assert.NoError(t, err)

		// Get notifications from the test service
		notifications := service.NotificationService.GetNotifications()

		// Should have 2 notifications: 1 for regular user, 1 for admin1
		assert.Len(t, notifications, 2, "Expected two notifications")

		// Verify regular user notification
		var foundRegularUser bool
		var foundAdmin bool
		for _, n := range notifications {
			if n.UserID == regularUser.ID {
				foundRegularUser = true
				assert.Contains(t, n.Content, "Name: Regular User")
				assert.Contains(t, n.Content, "Email: user@test.com")
			}
			if n.UserID == admin1.ID {
				foundAdmin = true
				assert.Contains(t, n.Content, "Name: Admin User")
				assert.Contains(t, n.Content, "Email: admin@test.com")
			}
		}
		assert.True(t, foundRegularUser, "Regular user should receive notification")
		assert.True(t, foundAdmin, "Admin1 should receive notification")

		// Verify admin2 (notifications disabled) did not receive notification
		for _, notification := range notifications {
			assert.NotEqual(t, admin2.ID, notification.UserID, "Admin2 should not receive notification")
		}

		// Get the test mailer and verify emails
		if testMailer, ok := notificationService.GetMailer().(interface {
			GetEmails() []struct {
				To string
			}
		}); ok {
			sentEmails := testMailer.GetEmails()
			// Verify emails were sent to the correct recipients
			if assert.Len(t, sentEmails, 2, "Expected two emails") {
				emailRecipients := []string{sentEmails[0].To, sentEmails[1].To}
				assert.Contains(t, emailRecipients, "admin1@test.com")
				assert.Contains(t, emailRecipients, "user@test.com")
			}
		}
	})
}
