package grpc

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A plugin notification's link lands on the stored row so the bell can open
// it: a single `link` applies to every recipient, a `links` map picks
// "admin" for the admin fan-out and "portal" for the named user, and
// off-site or scheme-bearing links are dropped rather than stored.
func TestCreateNotificationRPC_StoresLink(t *testing.T) {

	t.Run("single link reaches admin and user rows", func(t *testing.T) {
		server, service, plugin, ctx := setupExtensionRPCTest(t, models.ServiceScopeNotificationsWrite)
		admin := &models.User{Email: "admin@test.com", Name: "Admin", IsAdmin: true, NotificationsEnabled: true, EmailVerified: true}
		require.NoError(t, admin.Create(service.DB))
		user := &models.User{Email: "user@test.com", Name: "User", EmailVerified: true}
		require.NoError(t, user.Create(service.DB))

		resp, err := server.CreateNotification(ctx, &pb.CreateNotificationRequest{
			NotificationId: "req_1",
			Title:          "Access request",
			Content:        "body",
			NotifyAdmins:   true,
			UserId:         uint32(user.ID),
			Link:           "/admin/enterprise/asset-catalog/requests#req_1",
		})
		require.NoError(t, err)
		require.True(t, resp.Success, resp.Message)

		var rows []models.Notification
		require.NoError(t, service.DB.Where("notification_id LIKE ?", "plugin_%").Order("user_id").Find(&rows).Error)
		require.Len(t, rows, 2)
		for _, row := range rows {
			assert.Equal(t, "/admin/enterprise/asset-catalog/requests#req_1", row.Link, "user %d", row.UserID)
		}
		_ = plugin
	})

	t.Run("links map picks admin for admins and portal for the user", func(t *testing.T) {
		server, service, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeNotificationsWrite)
		admin := &models.User{Email: "admin@test.com", Name: "Admin", IsAdmin: true, NotificationsEnabled: true, EmailVerified: true}
		require.NoError(t, admin.Create(service.DB))
		user := &models.User{Email: "user@test.com", Name: "User", EmailVerified: true}
		require.NoError(t, user.Create(service.DB))

		resp, err := server.CreateNotification(ctx, &pb.CreateNotificationRequest{
			NotificationId: "req_2",
			Title:          "Access request",
			NotifyAdmins:   true,
			UserId:         uint32(user.ID),
			Links: map[string]string{
				"admin":  "/admin/enterprise/asset-catalog/requests#req_2",
				"portal": "/portal/plugins/asset-catalog#/assets/ast_9",
			},
		})
		require.NoError(t, err)
		require.True(t, resp.Success, resp.Message)

		var adminRow, userRow models.Notification
		require.NoError(t, service.DB.Where("user_id = ?", admin.ID).First(&adminRow).Error)
		require.NoError(t, service.DB.Where("user_id = ?", user.ID).First(&userRow).Error)
		assert.Equal(t, "/admin/enterprise/asset-catalog/requests#req_2", adminRow.Link)
		assert.Equal(t, "/portal/plugins/asset-catalog#/assets/ast_9", userRow.Link)
	})

	t.Run("links map falls back to link, then to the other entry", func(t *testing.T) {
		server, service, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeNotificationsWrite)
		admin := &models.User{Email: "admin@test.com", Name: "Admin", IsAdmin: true, NotificationsEnabled: true, EmailVerified: true}
		require.NoError(t, admin.Create(service.DB))
		user := &models.User{Email: "user@test.com", Name: "User", EmailVerified: true}
		require.NoError(t, user.Create(service.DB))

		_, err := server.CreateNotification(ctx, &pb.CreateNotificationRequest{
			NotificationId: "req_3",
			Title:          "Decision",
			NotifyAdmins:   true,
			UserId:         uint32(user.ID),
			Link:           "/admin/fallback",
			Links:          map[string]string{"portal": "/portal/only"},
		})
		require.NoError(t, err)
		var adminRow, userRow models.Notification
		require.NoError(t, service.DB.Where("user_id = ?", admin.ID).First(&adminRow).Error)
		require.NoError(t, service.DB.Where("user_id = ?", user.ID).First(&userRow).Error)
		assert.Equal(t, "/admin/fallback", adminRow.Link, "no admin entry: the plain link applies")
		assert.Equal(t, "/portal/only", userRow.Link)
	})

	t.Run("unsafe links are dropped, not stored", func(t *testing.T) {
		server, service, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeNotificationsWrite)
		user := &models.User{Email: "user@test.com", Name: "User", EmailVerified: true}
		require.NoError(t, user.Create(service.DB))

		for _, bad := range []string{"javascript:alert(1)", "//evil.example/x", "ftp://x/y", "relative/path", "/ok but spaced"} {
			resp, err := server.CreateNotification(ctx, &pb.CreateNotificationRequest{
				Title:  "x",
				UserId: uint32(user.ID),
				Link:   bad,
			})
			require.NoError(t, err, bad)
			require.True(t, resp.Success, bad)
		}
		var rows []models.Notification
		require.NoError(t, service.DB.Where("user_id = ?", user.ID).Find(&rows).Error)
		require.Len(t, rows, 5)
		for _, row := range rows {
			assert.Empty(t, row.Link)
		}

		// http(s) links are allowed: the bell opens them in a new tab.
		resp, err := server.CreateNotification(ctx, &pb.CreateNotificationRequest{
			NotificationId: "ext", Title: "x", UserId: uint32(user.ID), Link: "https://example.com/docs",
		})
		require.NoError(t, err)
		require.True(t, resp.Success)
		var ext models.Notification
		require.NoError(t, service.DB.Where("notification_id = ?", "plugin_1_ext_owner").First(&ext).Error)
		assert.Equal(t, "https://example.com/docs", ext.Link)
	})
}
