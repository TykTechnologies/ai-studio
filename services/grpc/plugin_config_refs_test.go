package grpc

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Plugins cannot add secret references to plugin config (they would read
// any secret they name); writing back a resolved reference keeps it.
func TestUpdatePluginConfigSecretReferences(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key-for-secrets")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	service := services.NewService(db)
	require.NoError(t, db.AutoMigrate(&models.Plugin{}, &secrets.Secret{}))
	secrets.SetDBRef(db)
	t.Cleanup(func() { secrets.SetDBRef(nil) })
	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "VAULT_TOKEN", Value: "hvs.real-token"}))

	plugin := &models.Plugin{Name: "rag", Command: "/bin/rag", HookType: models.HookTypeStudioUI,
		Config: map[string]interface{}{"vault_token": "$SECRET/VAULT_TOKEN"}}
	require.NoError(t, db.Create(plugin).Error)
	server := NewPluginManagementServer(service.PluginService)
	ctx := context.Background()

	_, err = server.UpdatePluginConfig(ctx, &pb.UpdatePluginConfigRequest{
		PluginId: uint32(plugin.ID), ConfigJson: `{"vault_token":"$SECRET/VAULT_TOKEN","stolen":"$SECRET/OTHER"}`})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// The plugin saves its resolved config: the reference survives
	_, err = server.UpdatePluginConfig(ctx, &pb.UpdatePluginConfigRequest{
		PluginId: uint32(plugin.ID), ConfigJson: `{"vault_token":"hvs.real-token","rules":[]}`})
	require.NoError(t, err)
	stored, err := service.PluginService.GetPlugin(plugin.ID)
	require.NoError(t, err)
	assert.Equal(t, "$SECRET/VAULT_TOKEN", stored.Config["vault_token"])
}
