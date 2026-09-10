package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
)

// Brokered plugin sessions only put the plugin ID in the context. The governed
// metadata delegates must validate the scope and attach the plugin so that
// "plugin_resource:self:<slug>" resolves; without that every plugin call
// failed with InvalidArgument ("requires an authenticated plugin context").
func TestGovernedMetadataRPCScopesAndSelfResolution(t *testing.T) {
	t.Run("reads need metadata.read", func(t *testing.T) {
		server, _, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeKVReadWrite)
		_, err := server.GetObjectMetadata(ctx, &pb.GetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = server.GetResolvedMetadataSchema(ctx, &pb.GetResolvedMetadataSchemaRequest{ObjectType: "llm"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = server.ValidateObjectMetadata(ctx, &pb.ValidateObjectMetadataRequest{ObjectType: "llm", ValuesJson: "{}"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("writes need metadata.write", func(t *testing.T) {
		server, _, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeMetadataRead)
		_, err := server.SetObjectMetadata(ctx, &pb.SetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1", ValuesJson: "{}"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = server.DeleteObjectMetadata(ctx, &pb.DeleteObjectMetadataRequest{ObjectType: "llm", ObjectId: "1"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("the plugin's own resource type resolves once the scope check passed", func(t *testing.T) {
		server, service, plugin, ctx := setupExtensionRPCTest(t, models.ServiceScopeMetadataRead, models.ServiceScopeMetadataWrite)
		// Opt a resource type in so the resolved type is a known governed object type.
		require.NoError(t, service.GetDB().Create(&models.PluginResourceType{
			PluginID: plugin.ID, Slug: "agent", Name: "Agent", IsActive: true, SupportsMetadata: true,
		}).Error)

		// Before the fix every call was rejected with InvalidArgument
		// ("requires an authenticated plugin context") before reaching the
		// service. This test runs under both editions: Enterprise stores the
		// record, Community refuses with FailedPrecondition. Both prove the
		// self object type resolved.
		resolved, err := server.GetResolvedMetadataSchema(ctx, &pb.GetResolvedMetadataSchemaRequest{ObjectType: "plugin_resource:self:agent"})
		require.NoError(t, err)
		assert.NotNil(t, resolved)

		resp, err := server.SetObjectMetadata(ctx, &pb.SetObjectMetadataRequest{ObjectType: "plugin_resource:self:agent", ObjectId: "ast_1", ValuesJson: `{"risk_tier":"high"}`})
		if err != nil {
			assert.Equal(t, codes.FailedPrecondition, status.Code(err), err.Error())
			assert.NotContains(t, err.Error(), "authenticated plugin context")
			assert.NotContains(t, err.Error(), "unknown object type")
			return
		}
		require.True(t, resp.Success, resp.Message)

		got, err := server.GetObjectMetadata(ctx, &pb.GetObjectMetadataRequest{ObjectType: "plugin_resource:self:agent", ObjectId: "ast_1"})
		require.NoError(t, err)
		assert.True(t, got.Found)
		assert.Contains(t, got.ValuesJson, `"risk_tier":"high"`)

		var rec models.ObjectMetadata
		require.NoError(t, service.GetDB().Where("object_type = ? AND object_id = ?", models.PluginResourceObjectType(plugin.ID, "agent"), "ast_1").First(&rec).Error)
		assert.Equal(t, models.MetadataSourcePlugin(plugin.ID), rec.UpdatedBySource, "writes are attributed to the calling plugin")

		_, err = server.DeleteObjectMetadata(ctx, &pb.DeleteObjectMetadataRequest{ObjectType: "plugin_resource:self:agent", ObjectId: "ast_1"})
		require.NoError(t, err)
		got, err = server.GetObjectMetadata(ctx, &pb.GetObjectMetadataRequest{ObjectType: "plugin_resource:self:agent", ObjectId: "ast_1"})
		require.NoError(t, err)
		assert.False(t, got.Found)
	})

	t.Run("no plugin in context is unauthenticated", func(t *testing.T) {
		server, _, _, _ := setupExtensionRPCTest(t, models.ServiceScopeMetadataRead)
		_, err := server.GetObjectMetadata(t.Context(), &pb.GetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1"})
		require.Error(t, err)
		assert.NotEqual(t, codes.OK, status.Code(err))
	})
}
