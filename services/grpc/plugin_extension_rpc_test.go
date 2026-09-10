package grpc

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupExtensionRPCTest(t *testing.T, scopes ...string) (*AIStudioManagementServer, *services.Service, *models.Plugin, context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	service := services.NewService(db)
	service.NotificationService = services.NewTestNotificationService(db)
	server := NewAIStudioManagementServer(service)

	plugin := &models.Plugin{
		Name:                    "asset-catalog",
		Command:                 "/usr/bin/asset-catalog",
		HookType:                models.HookTypeResourceProvider,
		HookTypes:               []string{models.HookTypeResourceProvider},
		IsActive:                true,
		ServiceAccessAuthorized: true,
		ServiceScopes:           scopes,
	}
	require.NoError(t, db.Create(plugin).Error)

	ctx := SetPluginIDInContext(context.Background(), plugin.ID)
	return server, service, plugin, ctx
}

func TestCreateNotificationRPC(t *testing.T) {
	t.Run("denied without notifications.write scope", func(t *testing.T) {
		server, _, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeKVReadWrite)
		_, err := server.CreateNotification(ctx, &pb.CreateNotificationRequest{Title: "x", NotifyAdmins: true})
		require.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("validates arguments", func(t *testing.T) {
		server, _, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeNotificationsWrite)

		_, err := server.CreateNotification(ctx, &pb.CreateNotificationRequest{NotifyAdmins: true})
		assert.Equal(t, codes.InvalidArgument, status.Code(err), "missing title")

		_, err = server.CreateNotification(ctx, &pb.CreateNotificationRequest{Title: "x"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err), "no recipient")

		_, err = server.CreateNotification(ctx, &pb.CreateNotificationRequest{Title: "x", UserId: uint32(models.NotifyAdmins)})
		assert.Equal(t, codes.InvalidArgument, status.Code(err), "user id collides with admin flag")
	})

	t.Run("creates plugin-scoped notifications for admins and a user", func(t *testing.T) {
		server, service, plugin, ctx := setupExtensionRPCTest(t, models.ServiceScopeNotificationsWrite)

		admin := &models.User{Email: "admin@test.com", Name: "Admin", IsAdmin: true, NotificationsEnabled: true, EmailVerified: true}
		require.NoError(t, admin.Create(service.DB))
		user := &models.User{Email: "user@test.com", Name: "User", EmailVerified: true}
		require.NoError(t, user.Create(service.DB))

		resp, err := server.CreateNotification(ctx, &pb.CreateNotificationRequest{
			NotificationId: "req_42",
			Type:           "access_request",
			Title:          "Access request: Triage Agent",
			Content:        "**alice** asked for access",
			NotifyAdmins:   true,
			UserId:         uint32(user.ID),
		})
		require.NoError(t, err)
		assert.True(t, resp.Success)

		var stored []models.Notification
		require.NoError(t, service.DB.Find(&stored).Error)
		require.Len(t, stored, 2)
		for _, n := range stored {
			assert.Contains(t, n.NotificationID, "plugin_")
			assert.Contains(t, n.NotificationID, "req_42")
			assert.Equal(t, "access_request", n.Type)
		}

		// Same dedupe key from the same plugin is a no-op
		_, err = server.CreateNotification(ctx, &pb.CreateNotificationRequest{
			NotificationId: "req_42", Title: "dup", NotifyAdmins: true, UserId: uint32(user.ID),
		})
		require.NoError(t, err)
		var count int64
		require.NoError(t, service.DB.Model(&models.Notification{}).Count(&count).Error)
		assert.Equal(t, int64(2), count)

		// Default type labels the plugin when none is given
		_, err = server.CreateNotification(ctx, &pb.CreateNotificationRequest{Title: "untyped", UserId: uint32(user.ID)})
		require.NoError(t, err)
		var untyped models.Notification
		require.NoError(t, service.DB.Where("title = ?", "untyped").First(&untyped).Error)
		assert.Equal(t, "plugin:"+plugin.Name, untyped.Type)
	})

	t.Run("strips HTML and script links from plugin content before storing", func(t *testing.T) {
		server, service, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeNotificationsWrite)
		user := &models.User{Email: "user2@test.com", Name: "User", EmailVerified: true}
		require.NoError(t, user.Create(service.DB))

		resp, err := server.CreateNotification(ctx, &pb.CreateNotificationRequest{
			Title:   "Alert <script>alert(1)</script>",
			Content: "**bold** <img src=x onerror=alert(1)> [go](javascript:alert(2)) [ok](https://example.com)",
			UserId:  uint32(user.ID),
		})
		require.NoError(t, err)
		assert.True(t, resp.Success)
		var stored models.Notification
		require.NoError(t, service.DB.Where("user_id = ?", user.ID).First(&stored).Error)
		assert.Equal(t, "Alert alert(1)", stored.Title)
		assert.Equal(t, "**bold**  [go](#alert(2)) [ok](https://example.com)", stored.Content)

		// A title that is nothing but markup is rejected rather than stored empty.
		_, err = server.CreateNotification(ctx, &pb.CreateNotificationRequest{Title: "<b></b>", UserId: uint32(user.ID)})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

func TestRegisterResourceTypesRPC(t *testing.T) {
	t.Run("denied without resource-types.manage scope", func(t *testing.T) {
		server, _, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeNotificationsWrite)
		_, err := server.RegisterResourceTypes(ctx, &pb.RegisterResourceTypesRequest{
			Types: []*pb.ResourceTypeSpec{{Slug: "agent", Name: "Agent"}},
		})
		require.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("registers, updates and deactivates missing types", func(t *testing.T) {
		server, service, plugin, ctx := setupExtensionRPCTest(t, models.ServiceScopeResourceTypesManage)

		schema := `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`
		resp, err := server.RegisterResourceTypes(ctx, &pb.RegisterResourceTypesRequest{
			Types: []*pb.ResourceTypeSpec{
				{Slug: "agent", Name: "Agent", SupportsSubmissions: true, SubmissionSchema: schema},
				{Slug: "prompt", Name: "Prompt"},
			},
			DeactivateMissing: true,
		})
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, uint32(2), resp.Registered)
		assert.Equal(t, uint32(0), resp.Deactivated)

		agent, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "agent")
		require.NoError(t, err)
		assert.Equal(t, schema, agent.SubmissionSchema)
		assert.True(t, agent.SupportsSubmissions)
		assert.True(t, agent.IsActive)

		// Second sync drops "prompt" and renames "agent"
		resp, err = server.RegisterResourceTypes(ctx, &pb.RegisterResourceTypesRequest{
			Types:             []*pb.ResourceTypeSpec{{Slug: "agent", Name: "AI Agent"}},
			DeactivateMissing: true,
		})
		require.NoError(t, err)
		assert.Equal(t, uint32(1), resp.Deactivated)

		agent, err = service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "agent")
		require.NoError(t, err)
		assert.Equal(t, "AI Agent", agent.Name)
		assert.Empty(t, agent.SubmissionSchema, "schema cleared when omitted on re-sync")

		prompt, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "prompt")
		require.NoError(t, err)
		assert.False(t, prompt.IsActive)
	})

	t.Run("rejects invalid schema and empty slug", func(t *testing.T) {
		server, _, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeResourceTypesManage)

		_, err := server.RegisterResourceTypes(ctx, &pb.RegisterResourceTypesRequest{
			Types: []*pb.ResourceTypeSpec{{Slug: "agent", Name: "Agent", SubmissionSchema: `{"type":"array"}`}},
		})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = server.RegisterResourceTypes(ctx, &pb.RegisterResourceTypesRequest{
			Types: []*pb.ResourceTypeSpec{{Slug: "", Name: "Agent"}},
		})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("cannot touch another plugin's types", func(t *testing.T) {
		server, service, plugin, ctx := setupExtensionRPCTest(t, models.ServiceScopeResourceTypesManage)

		other := &models.Plugin{Name: "other", Command: "/usr/bin/other", HookType: models.HookTypeResourceProvider, IsActive: true}
		require.NoError(t, service.DB.Create(other).Error)
		require.NoError(t, service.RegisterPluginResourceTypes(other.ID, []models.PluginResourceType{{Slug: "agent", Name: "Other Agent"}}))

		_, err := server.RegisterResourceTypes(ctx, &pb.RegisterResourceTypesRequest{
			Types:             []*pb.ResourceTypeSpec{{Slug: "skill", Name: "Skill"}},
			DeactivateMissing: true,
		})
		require.NoError(t, err)

		otherAgent, err := service.GetPluginResourceTypeByPluginAndSlug(other.ID, "agent")
		require.NoError(t, err)
		assert.True(t, otherAgent.IsActive, "other plugin's type untouched")

		mine, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "skill")
		require.NoError(t, err)
		assert.Equal(t, plugin.ID, mine.PluginID)
	})
}
