package grpc

import (
	"context"
	"net"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// brokerFixture serves the management service the way a plugin's brokered
// server does in production (services.PluginBrokerServerOptions) and returns a
// client for the plugin it is bound to.
type brokerFixture struct {
	db     *gorm.DB
	caller *models.Plugin
	other  *models.Plugin
	client pb.AIStudioManagementServiceClient
}

func newBrokerFixture(t *testing.T, serviceAccess bool, scopes ...string) *brokerFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	service := services.NewService(db)

	caller := &models.Plugin{Name: "caller", Command: "/usr/bin/caller", HookType: models.HookTypePostAuth,
		IsActive: true, ServiceAccessAuthorized: serviceAccess, ServiceScopes: scopes,
		Config: map[string]interface{}{"mine": "yes"}}
	other := &models.Plugin{Name: "other", Command: "/usr/bin/other", HookType: models.HookTypePostAuth,
		IsActive: true, Config: map[string]interface{}{"api_key": "other-secret"}}
	require.NoError(t, db.Create(caller).Error)
	require.NoError(t, db.Create(other).Error)

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(services.PluginBrokerServerOptions(nil, caller.ID, service)...)
	pb.RegisterAIStudioManagementServiceServer(srv, NewAIStudioManagementServer(service))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	return &brokerFixture{db: db, caller: caller, other: other, client: pb.NewAIStudioManagementServiceClient(conn)}
}

func requireCode(t *testing.T, want codes.Code, err error, what string) {
	t.Helper()
	require.Error(t, err, what)
	assert.Equal(t, want, status.Code(err), "%s: %v", what, err)
}

// A plugin with no approved scopes gets nothing from the management service
// beyond its own configuration and the license status.
func TestBroker_PluginWithoutScopesIsRefused(t *testing.T) {
	f := newBrokerFixture(t, true)
	ctx := context.Background()

	_, err := f.client.CreateLLM(ctx, &pb.CreateLLMRequest{Name: "rogue"})
	requireCode(t, codes.PermissionDenied, err, "CreateLLM")
	_, err = f.client.ListPlugins(ctx, &pb.ListPluginsRequest{})
	requireCode(t, codes.PermissionDenied, err, "ListPlugins")
	_, err = f.client.UpdatePluginConfig(ctx, &pb.UpdatePluginConfigRequest{PluginId: uint32(f.other.ID), ConfigJson: `{"api_key":"stolen"}`})
	requireCode(t, codes.PermissionDenied, err, "UpdatePluginConfig on another plugin")

	var other models.Plugin
	require.NoError(t, f.db.First(&other, f.other.ID).Error)
	assert.Equal(t, "other-secret", other.Config["api_key"], "another plugin's config was changed")

	// Another plugin's details need plugins.read, even without its config.
	_, err = f.client.GetPlugin(ctx, &pb.GetPluginRequest{PluginId: uint32(f.other.ID)})
	requireCode(t, codes.PermissionDenied, err, "GetPlugin on another plugin")

	// Its own configuration and the license status need no scope.
	_, err = f.client.UpdatePluginConfig(ctx, &pb.UpdatePluginConfigRequest{PluginId: uint32(f.caller.ID), ConfigJson: `{"mine":"updated"}`})
	require.NoError(t, err, "UpdatePluginConfig on itself")
	got, err := f.client.GetPlugin(ctx, &pb.GetPluginRequest{PluginId: uint32(f.caller.ID)})
	require.NoError(t, err, "GetPlugin on itself")
	assert.Contains(t, got.Plugin.ConfigJson, "updated")
	_, err = f.client.GetLicenseInfo(ctx, &pb.GetLicenseInfoRequest{})
	require.NoError(t, err, "GetLicenseInfo")
}

// Without approved service access, scoped calls are refused even when the
// plugin declares the scope.
func TestBroker_UnauthorizedServiceAccessIsRefused(t *testing.T) {
	f := newBrokerFixture(t, false, models.ServiceScopeLLMsWrite)
	_, err := f.client.CreateLLM(context.Background(), &pb.CreateLLMRequest{Name: "rogue"})
	requireCode(t, codes.PermissionDenied, err, "CreateLLM without approved service access")
}

// plugins.config and plugins.read do not reach another plugin's
// configuration: updates are refused and reads leave it out.
func TestBroker_PluginScopesStayOnTheCallersOwnConfig(t *testing.T) {
	f := newBrokerFixture(t, true, models.ServiceScopePluginsConfig, models.ServiceScopePluginsRead)
	ctx := context.Background()

	_, err := f.client.UpdatePluginConfig(ctx, &pb.UpdatePluginConfigRequest{PluginId: uint32(f.other.ID), ConfigJson: `{"api_key":"stolen"}`})
	requireCode(t, codes.PermissionDenied, err, "UpdatePluginConfig on another plugin with plugins.config")

	other, err := f.client.GetPlugin(ctx, &pb.GetPluginRequest{PluginId: uint32(f.other.ID)})
	require.NoError(t, err)
	assert.Empty(t, other.Plugin.ConfigJson, "GetPlugin returned another plugin's config")

	list, err := f.client.ListPlugins(ctx, &pb.ListPluginsRequest{})
	require.NoError(t, err)
	for _, p := range list.Plugins {
		if p.Id == uint32(f.caller.ID) {
			assert.Contains(t, p.ConfigJson, "mine")
		} else {
			assert.Empty(t, p.ConfigJson, "ListPlugins returned plugin %d's config", p.Id)
		}
	}
}

// The streaming CallLLM gets the broker's plugin identity too, and a request
// cannot name another plugin to borrow its scopes or agent configuration.
func TestBroker_CallLLMCannotNameAnotherPlugin(t *testing.T) {
	f := newBrokerFixture(t, true, models.ServiceScopeLLMsProxy)
	stream, err := f.client.CallLLM(context.Background(), &pb.CallLLMRequest{
		Context: &pb.PluginContext{PluginId: uint32(f.other.ID)},
		LlmId:   1,
	})
	require.NoError(t, err)
	_, err = stream.Recv()
	requireCode(t, codes.PermissionDenied, err, "CallLLM naming another plugin")

	noScope := newBrokerFixture(t, true)
	stream, err = noScope.client.CallLLM(context.Background(), &pb.CallLLMRequest{LlmId: 1})
	require.NoError(t, err)
	_, err = stream.Recv()
	requireCode(t, codes.PermissionDenied, err, "CallLLM without llms.proxy")
}

// Calls to other services on the broker (the plugin event service) are not
// management calls and are not scope-checked here; an unmapped management
// method would be refused (TestEveryManagementRPCHasAScope keeps the map
// complete).
func TestManagementScopeInterceptor_PassesOtherServices(t *testing.T) {
	unary, _ := ManagementScopeInterceptors(nil)
	called := false
	_, err := unary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/plugin_events.PluginEventService/Publish"},
		func(ctx context.Context, req interface{}) (interface{}, error) { called = true; return nil, nil })
	require.NoError(t, err)
	assert.True(t, called)

	_, err = unary(SetPluginIDInContext(context.Background(), 1), nil, &grpc.UnaryServerInfo{FullMethod: managementServicePrefix + "NoSuchMethod"},
		func(ctx context.Context, req interface{}) (interface{}, error) { return nil, nil })
	assert.Equal(t, codes.Internal, status.Code(err), "an unmapped management method must be refused")
}
