package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
)

// Both service containers give their plugin manager the gateway's
// PLUGINS_ALLOWED_SOURCES policy, with and without OCI support.
func TestServiceContainers_PluginManagerHasSourcePolicy(t *testing.T) {
	newConfig := func(t *testing.T, oci bool) *config.Config {
		cfg := &config.Config{Security: config.SecurityConfig{EncryptionKey: agentTracingTestEncKey}}
		cfg.HubSpoke.Mode = "standalone"
		if oci {
			cfg.OCIPlugins.CacheDir = t.TempDir()
		}
		policy, err := plugin_security.ParseSourcePolicy("oci")
		require.NoError(t, err)
		cfg.Plugins.SourcePolicy = policy
		return cfg
	}
	assertPolicy := func(t *testing.T, pm *plugins.PluginManager) {
		t.Helper()
		assert.Equal(t, []plugin_security.PluginSource{plugin_security.SourceOCI}, pm.SourcePolicy().Allowed())
	}

	for _, oci := range []bool{false, true} {
		t.Run("standalone container", func(t *testing.T) {
			c, err := NewServiceContainer(setupEdgeSyncTestDB(t), newConfig(t, oci))
			require.NoError(t, err)
			t.Cleanup(c.Cleanup)
			assertPolicy(t, c.PluginManager)
		})
		t.Run("hub-spoke container", func(t *testing.T) {
			c, err := NewHubSpokeServiceContainer(setupEdgeSyncTestDB(t), newConfig(t, oci))
			require.NoError(t, err)
			t.Cleanup(c.Cleanup)
			assertPolicy(t, c.PluginManager)
		})
	}
}
