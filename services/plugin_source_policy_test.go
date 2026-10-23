package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/marketplace"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
)

func mustSourcePolicy(t *testing.T, csv string) plugin_security.SourcePolicy {
	t.Helper()
	policy, err := plugin_security.ParseSourcePolicy(csv)
	require.NoError(t, err)
	return policy.WithSetting("AI_STUDIO_PLUGIN_ALLOWED_SOURCES")
}

func requireSourceNotAllowed(t *testing.T, err error) {
	t.Helper()
	var notAllowed *plugin_security.ErrPluginSourceNotAllowed
	require.True(t, errors.As(err, &notAllowed), "want ErrPluginSourceNotAllowed, got %v", err)
}

func TestPluginService_SourcePolicy(t *testing.T) {
	db, _ := setupSecurityWiringTest(t)
	svc := NewPluginService(db)
	svc.SetSourcePolicy(mustSourcePolicy(t, "grpc"))

	_, err := svc.CreatePlugin(&CreatePluginRequest{Name: "file", Command: "/usr/local/bin/demo", HookType: "post_auth"})
	requireSourceNotAllowed(t, err)

	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	plugin, err := svc.CreatePlugin(&CreatePluginRequest{Name: "grpc", Command: "grpc://plugin.example.com:50051", HookType: "post_auth"})
	require.NoError(t, err)

	// Editing keeps working; saving a disallowed command does not.
	name := "renamed"
	_, err = svc.UpdatePlugin(plugin.ID, &UpdatePluginRequest{Name: &name})
	require.NoError(t, err)
	command := "/usr/local/bin/demo"
	_, err = svc.UpdatePlugin(plugin.ID, &UpdatePluginRequest{Command: &command})
	requireSourceNotAllowed(t, err)
	ociRef := "oci://registry.example.com/plugins/demo:1"
	_, err = svc.UpdatePlugin(plugin.ID, &UpdatePluginRequest{OCIReference: &ociRef})
	requireSourceNotAllowed(t, err)
}

// With oci disallowed, no OCI path reaches the registry.
func TestOCIPaths_NoRegistryRequestWhenOCIDisallowed(t *testing.T) {
	db, cfg := setupSecurityWiringTest(t)
	svc := NewServiceWithOCI(db, cfg)
	noOCI := mustSourcePolicy(t, "file,grpc")
	svc.PluginService.SetSourcePolicy(noOCI)
	svc.PluginManifestService.SetSourcePolicy(noOCI)
	svc.AIStudioPluginManager.SetSourcePolicy(noOCI)

	reg := ocitest.NewRegistry(t)
	manifest := reg.PushPlugin("plugins/demo", "1.0.0", "linux/amd64", []byte("binary"))
	command := "oci://" + reg.Host() + "/plugins/demo@" + manifest.Digest.String() + "?arch=linux/amd64"

	_, err := svc.PluginService.CreateOCIPluginFromReference(&CreateOCIPluginRequest{Name: "demo", OCIReference: command})
	requireSourceNotAllowed(t, err)

	plugin := &models.Plugin{Name: "stored", Command: command, OCIReference: command, HookType: models.HookTypeStudioUI, IsActive: true}
	require.NoError(t, db.Create(plugin).Error)

	_, err = svc.PluginService.RefreshOCIPlugin(plugin.ID)
	requireSourceNotAllowed(t, err)
	_, err = svc.PluginManifestService.ParsePluginManifest(plugin)
	requireSourceNotAllowed(t, err)
	_, err = svc.AIStudioPluginManager.LoadPlugin(plugin.ID)
	requireSourceNotAllowed(t, err)

	ms := NewMarketplaceService(db, nil, svc.PluginService, svc.AIStudioPluginManager, t.TempDir(), "https://marketplace.example.com/index.yaml", 0)
	ms.SetSourcePolicy(noOCI)
	require.NoError(t, db.Create(&models.MarketplacePlugin{
		PluginID: "com.example.demo", Version: "1.0.0", Name: "Demo",
		OCIRegistry: reg.Host(), OCIRepository: "plugins/demo", OCIDigest: manifest.Digest.String(), PrimaryHook: "post_auth",
	}).Error)
	_, err = ms.InstallFromMarketplace(context.Background(), &marketplace.InstallRequest{PluginID: "com.example.demo", Version: "1.0.0"})
	requireSourceNotAllowed(t, err)
	var installed int64
	require.NoError(t, db.Model(&models.Plugin{}).Where("name = ?", "Demo").Count(&installed).Error)
	assert.Zero(t, installed, "a refused install creates no plugin")

	for _, method := range []string{"GET", "HEAD"} {
		for _, kind := range []string{"manifests", "blobs", "referrers"} {
			assert.Zero(t, reg.Requests(method, kind), "%s %s", method, kind)
		}
	}
}

// A plugin row written straight to the database with a disallowed source is
// not spawned, even by a manager that has no security service: the policy
// comes from configuration.
func TestAIStudioPluginManager_RefusesDisallowedSourceFromConfig(t *testing.T) {
	db, _ := setupSecurityWiringTest(t)
	saved := config.Get("")
	conf := *saved
	conf.PluginSourcePolicy = mustSourcePolicy(t, "oci")
	config.Set(&conf)
	t.Cleanup(func() { config.Set(saved) })

	manager := NewAIStudioPluginManager(db, nil)
	require.Nil(t, manager.getSecurityService())
	plugin := &models.Plugin{Name: "direct", Command: "/bin/true", HookType: "post_auth", IsActive: true}
	require.NoError(t, db.Create(plugin).Error)

	_, err := manager.LoadPlugin(plugin.ID)
	requireSourceNotAllowed(t, err)
	assert.Contains(t, err.Error(), "AI_STUDIO_PLUGIN_ALLOWED_SOURCES")
	assert.False(t, manager.IsPluginLoaded(plugin.ID))
}
