package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
)

// Both service containers give the OCI plugin client the plugin security
// service. Without it a gateway that requires signatures (the default)
// refuses every OCI plugin, and an Enterprise gateway never runs its
// verifier. The hub-spoke container is the one control and edge gateways run.
func TestServiceContainers_OCIClientHasSecurityService(t *testing.T) {
	newConfig := func(t *testing.T) *config.Config {
		cfg := &config.Config{Security: config.SecurityConfig{EncryptionKey: agentTracingTestEncKey}}
		cfg.HubSpoke.Mode = "standalone"
		cfg.OCIPlugins.CacheDir = t.TempDir()
		cfg.OCIPlugins.RequireSignature = true
		return cfg
	}

	t.Run("standalone container", func(t *testing.T) {
		c, err := NewServiceContainer(setupEdgeSyncTestDB(t), newConfig(t))
		require.NoError(t, err)
		t.Cleanup(c.Cleanup)
		client := c.PluginManager.GetOCIClient()
		require.NotNil(t, client, "OCI support must be enabled")
		assert.NotNil(t, client.SecurityService())
	})

	t.Run("hub-spoke container", func(t *testing.T) {
		t.Skip("fails until midsommar-clean-c1g.22 is fixed; remove this skip with the fix")
		c, err := NewHubSpokeServiceContainer(setupEdgeSyncTestDB(t), newConfig(t))
		require.NoError(t, err)
		t.Cleanup(c.Cleanup)
		client := c.PluginManager.GetOCIClient()
		require.NotNil(t, client, "OCI support must be enabled")
		assert.NotNil(t, client.SecurityService(),
			"edge and control gateways must verify OCI plugins through the plugin security service")
	})
}
