package services

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	pb "github.com/TykTechnologies/midsommar/v2/proto/microgateway_management"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// On a plugin's brokered server (which records the plugin it serves under
// plugins.BoundPluginIDKey) a request's plugin context must name that plugin:
// a plugin cannot use another plugin's ID, and with it that plugin's scopes.
func TestValidatePluginScope_RequestMustNameTheBrokersPlugin(t *testing.T) {
	db, server := setupStoreAppTestDB(t) // plugin 1 holds apps.read
	require.NoError(t, db.Create(&database.Plugin{ID: 2, Name: "weak", Command: "/bin/weak", HookType: "auth", IsActive: true,
		ServiceScopes: database.JSON(`[]`)}).Error)

	// Plugin 2 (no scopes) claims to be plugin 1.
	_, err := server.ListApps(boundTo(2), &pb.ListAppsRequest{Context: &pb.PluginContext{PluginId: 1}})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// Plugin 1 naming itself passes.
	_, err = server.ListApps(boundTo(1), &pb.ListAppsRequest{Context: &pb.PluginContext{PluginId: 1}})
	require.NoError(t, err)

	// A call that did not come through a plugin's broker is refused.
	_, err = server.ListApps(context.Background(), &pb.ListAppsRequest{Context: &pb.PluginContext{PluginId: 1}})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// boundTo is the context a plugin's brokered server gives its calls.
func boundTo(id uint32) context.Context {
	return context.WithValue(context.Background(), plugins.BoundPluginIDKey, id)
}
