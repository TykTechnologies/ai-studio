package services

import (
	"context"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/studiodb"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSecurityWiringTest(t *testing.T) (*gorm.DB, *ociplugins.OCIConfig) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, studiodb.Migrate(db))

	cfg := ociplugins.DefaultOCIConfig()
	cfg.CacheDir = t.TempDir()
	cfg.GCInterval = 0
	cfg.RequireSignature = true
	return db, cfg
}

// recordingSecurityService is a plugin security service whose
// VerifySignature records each call and accepts the artifact.
type recordingSecurityService struct {
	plugin_security.Service

	mu      sync.Mutex
	digests []string
}

func (s *recordingSecurityService) VerifySignature(_ context.Context, ref *ociplugins.OCIReference, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.digests = append(s.digests, ref.Digest)
	return nil
}

func (s *recordingSecurityService) verified() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.digests...)
}

// registerRecordingSecurityService makes plugin_security.NewService return a
// recording service (as the enterprise factory does in ENT builds) for the
// rest of the test.
func registerRecordingSecurityService(t *testing.T, cfg *ociplugins.OCIConfig) *recordingSecurityService {
	t.Helper()
	fake := &recordingSecurityService{
		Service: plugin_security.NewService(&plugin_security.Config{OCIConfig: cfg}),
	}
	previous := plugin_security.RegisterEnterpriseFactory(func(*plugin_security.Config) plugin_security.Service {
		return fake
	})
	t.Cleanup(func() { plugin_security.RegisterEnterpriseFactory(previous) })
	return fake
}

func TestNewServiceWithOCI_EveryOCIClientHasTheSecurityService(t *testing.T) {
	db, cfg := setupSecurityWiringTest(t)
	svc := NewServiceWithOCI(db, cfg)

	require.NotNil(t, svc.PluginSecurityService)
	clients := map[string]*ociplugins.OCIPluginClient{
		"PluginService":         svc.PluginService.ociClient,
		"PluginManifestService": svc.PluginManifestService.ociClient,
		"AIStudioPluginManager": svc.AIStudioPluginManager.ociClient,
	}
	marketplaceClient, err := svc.NewOCIPluginClient(cfg)
	require.NoError(t, err)
	clients["marketplace (Service.NewOCIPluginClient)"] = marketplaceClient

	for name, client := range clients {
		require.NotNil(t, client, "%s has no OCI client", name)
		assert.Same(t, svc.PluginSecurityService, client.SecurityService(),
			"%s's OCI client must verify through the shared plugin security service", name)
	}
}

func TestNewServiceWithOCI_NoOCIConfigHasNoSecurityService(t *testing.T) {
	db, _ := setupSecurityWiringTest(t)
	svc := NewServiceWithOCI(db, nil)
	assert.Nil(t, svc.PluginSecurityService)
}

// A manifest fetch through PluginManifestService verifies the signature with
// the plugin security service (the enterprise verifier in ENT builds) rather
// than a verifier of its own.
func TestPluginManifestFetch_VerifiesThroughSecurityService(t *testing.T) {
	db, cfg := setupSecurityWiringTest(t)
	fake := registerRecordingSecurityService(t, cfg)
	svc := NewServiceWithOCI(db, cfg)
	require.Same(t, fake, svc.PluginSecurityService)

	reg := ocitest.NewRegistry(t)
	manifest := reg.PushPlugin("plugins/demo", "1.0.0", "linux/amd64", []byte("plugin binary"))
	plugin := &models.Plugin{
		Name:         "demo",
		Command:      "oci://" + reg.Host() + "/plugins/demo@" + manifest.Digest.String() + "?arch=linux/amd64",
		OCIReference: "oci://" + reg.Host() + "/plugins/demo@" + manifest.Digest.String() + "?arch=linux/amd64",
		HookType:     models.HookTypeStudioUI,
		IsActive:     true,
		Manifest: map[string]interface{}{
			"id": "com.example.demo", "version": "1.0.0", "name": "demo",
			"capabilities": map[string]interface{}{"hooks": []string{models.HookTypeStudioUI}},
		},
	}

	_, err := svc.PluginManifestService.ParsePluginManifest(plugin)
	require.NoError(t, err)
	assert.Equal(t, []string{manifest.Digest.String()}, fake.verified(),
		"the manifest fetch must verify the pulled digest through the security service")
}
