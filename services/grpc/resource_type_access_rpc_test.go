package grpc

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// TestRegisterResourceTypesRPC_AccessGrantedViaApp covers the tri-state wire
// field: unset falls back to the platform heuristic (the test plugin is a
// resource provider without endpoints, so false), an explicit value is
// stored as declared, and the portal path is validated.
func TestRegisterResourceTypesRPC_AccessGrantedViaApp(t *testing.T) {
	t.Run("unset resolves from hook types, explicit value wins", func(t *testing.T) {
		server, service, plugin, ctx := setupExtensionRPCTest(t, models.ServiceScopeResourceTypesManage)

		_, err := server.RegisterResourceTypes(ctx, &pb.RegisterResourceTypesRequest{
			Types: []*pb.ResourceTypeSpec{
				{Slug: "prompt", Name: "Prompt"},
				{Slug: "agent", Name: "Agent", AccessGrantedViaApp: proto.Bool(true), PortalDetailPath: "/portal/plugins/asset-catalog#/assets/{id}"},
			},
		})
		require.NoError(t, err)

		prompt, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "prompt")
		require.NoError(t, err)
		assert.False(t, prompt.AccessGrantedViaApp)
		assert.Nil(t, prompt.AccessGrantedViaAppDeclared)

		agent, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "agent")
		require.NoError(t, err)
		assert.True(t, agent.AccessGrantedViaApp)
		require.NotNil(t, agent.AccessGrantedViaAppDeclared)
		assert.True(t, *agent.AccessGrantedViaAppDeclared)
		assert.Equal(t, "/portal/plugins/asset-catalog#/assets/{id}", agent.PortalDetailPath)

		// Re-sync without the declaration goes back to the heuristic.
		_, err = server.RegisterResourceTypes(ctx, &pb.RegisterResourceTypesRequest{
			Types: []*pb.ResourceTypeSpec{{Slug: "agent", Name: "Agent"}},
		})
		require.NoError(t, err)
		agent, err = service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "agent")
		require.NoError(t, err)
		assert.False(t, agent.AccessGrantedViaApp)
		assert.Nil(t, agent.AccessGrantedViaAppDeclared)
		assert.Empty(t, agent.PortalDetailPath, "path cleared when omitted on re-sync")
	})

	t.Run("rejects off-site portal paths", func(t *testing.T) {
		server, _, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeResourceTypesManage)
		for _, bad := range []string{"https://evil.example/{id}", "//evil.example/{id}", "portal/relative", "javascript:alert(1)"} {
			_, err := server.RegisterResourceTypes(ctx, &pb.RegisterResourceTypesRequest{
				Types: []*pb.ResourceTypeSpec{{Slug: "agent", Name: "Agent", PortalDetailPath: bad}},
			})
			assert.Equal(t, codes.InvalidArgument, status.Code(err), bad)
		}
	})
}
