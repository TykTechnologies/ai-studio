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
		server, _, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeMetadataRead, models.ServiceScopeMetadataWrite)
		// Community Edition resolves to an empty schema and refuses writes with
		// FailedPrecondition. Before the fix the self object type was rejected
		// earlier with InvalidArgument ("requires an authenticated plugin context").
		resolved, err := server.GetResolvedMetadataSchema(ctx, &pb.GetResolvedMetadataSchemaRequest{ObjectType: "plugin_resource:self:agent"})
		require.NoError(t, err)
		assert.NotNil(t, resolved)

		_, err = server.SetObjectMetadata(ctx, &pb.SetObjectMetadataRequest{ObjectType: "plugin_resource:self:agent", ObjectId: "ast_1", ValuesJson: `{"risk_tier":"high"}`})
		require.Error(t, err)
		assert.Equal(t, codes.FailedPrecondition, status.Code(err), err.Error())

		// Community Edition treats deletes as no-ops; either way the self type must resolve.
		_, err = server.DeleteObjectMetadata(ctx, &pb.DeleteObjectMetadataRequest{ObjectType: "plugin_resource:self:agent", ObjectId: "ast_1"})
		if err != nil {
			assert.Equal(t, codes.FailedPrecondition, status.Code(err), err.Error())
		}
	})

	t.Run("no plugin in context is unauthenticated", func(t *testing.T) {
		server, _, _, _ := setupExtensionRPCTest(t, models.ServiceScopeMetadataRead)
		_, err := server.GetObjectMetadata(t.Context(), &pb.GetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1"})
		require.Error(t, err)
		assert.NotEqual(t, codes.OK, status.Code(err))
	})
}
