package services_test

import (
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNotifyDirect covers notifications composed in code (no template), which
// is the path used by submission notifications and by plugins.
func TestNotifyDirect(t *testing.T) {
	db := apitest.SetupTestDB(t)
	ns := services.NewTestNotificationService(db)

	admin := &models.User{Email: "admin@test.com", Name: "Admin", IsAdmin: true, NotificationsEnabled: true, EmailVerified: true}
	require.NoError(t, admin.Create(db))
	mutedAdmin := &models.User{Email: "muted@test.com", Name: "Muted", IsAdmin: true, NotificationsEnabled: false, EmailVerified: true}
	require.NoError(t, mutedAdmin.Create(db))
	user := &models.User{Email: "user@test.com", Name: "User", IsAdmin: false, NotificationsEnabled: true, EmailVerified: true}
	require.NoError(t, user.Create(db))

	t.Run("delivers to admins and a specific user with type set", func(t *testing.T) {
		ns.ClearNotifications()
		err := ns.NotifyDirect("direct_1", "access_request", "Access requested", "**bold** body", user.ID|models.NotifyAdmins)
		require.NoError(t, err)

		var stored []models.Notification
		require.NoError(t, db.Where("notification_id LIKE ?", "direct_1%").Find(&stored).Error)
		require.Len(t, stored, 2)

		recipients := map[uint]models.Notification{}
		for _, n := range stored {
			recipients[n.UserID] = n
		}
		assert.Contains(t, recipients, admin.ID)
		assert.Contains(t, recipients, user.ID)
		assert.NotContains(t, recipients, mutedAdmin.ID)
		assert.Equal(t, "access_request", recipients[user.ID].Type)
		assert.Equal(t, "**bold** body", recipients[user.ID].Content)
		assert.Equal(t, "Access requested", recipients[admin.ID].Title)
	})

	t.Run("deduplicates on notification ID", func(t *testing.T) {
		require.NoError(t, ns.NotifyDirect("direct_2", "", "Once", "body", user.ID))
		require.NoError(t, ns.NotifyDirect("direct_2", "", "Once again", "body", user.ID))

		var count int64
		require.NoError(t, db.Model(&models.Notification{}).Where("notification_id = ?", "direct_2_owner").Count(&count).Error)
		assert.Equal(t, int64(1), count)
	})

	t.Run("rejects missing id or title", func(t *testing.T) {
		assert.Error(t, ns.NotifyDirect("", "", "Title", "body", user.ID))
		assert.Error(t, ns.NotifyDirect("direct_3", "", "", "body", user.ID))
	})
}

// TestNotifyWithoutTemplateFails documents why NotifyDirect exists: Notify
// requires a template path and cannot be used for code-composed content.
func TestNotifyWithoutTemplateFails(t *testing.T) {
	db := apitest.SetupTestDB(t)
	ns := services.NewTestNotificationService(db)

	err := ns.Notify("tmpl_less", "Title", "", map[string]interface{}{}, models.NotifyAdmins)
	assert.Error(t, err)
}
