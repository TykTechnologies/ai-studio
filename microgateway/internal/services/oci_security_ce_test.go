//go:build !enterprise

package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
)

// A Community edge gateway requiring signatures loads OCI plugins (it cannot
// verify them) and records them as unverified, rather than refusing them.
func TestHubSpokeContainer_CEEdgeLoadsOCIPluginUnverified(t *testing.T) {
	cfg := &config.Config{Security: config.SecurityConfig{EncryptionKey: agentTracingTestEncKey}}
	cfg.HubSpoke.Mode = "standalone"
	cfg.OCIPlugins.CacheDir = t.TempDir()
	cfg.OCIPlugins.RequireSignature = true
	c, err := NewHubSpokeServiceContainer(setupEdgeSyncTestDB(t), cfg)
	require.NoError(t, err)
	t.Cleanup(c.Cleanup)
	client := c.PluginManager.GetOCIClient()
	require.NotNil(t, client)

	reg := ocitest.NewRegistry(t)
	reg.PushPlugin("plugins/demo", "1.0.0", "linux/amd64", []byte("binary"))
	ref := &ociplugins.OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Tag: "1.0.0"}
	local, err := client.FetchPlugin(context.Background(), ref, &ociplugins.OCIPluginParams{Architecture: "linux/amd64"})
	require.NoError(t, err)
	assert.False(t, local.Verified)
}
