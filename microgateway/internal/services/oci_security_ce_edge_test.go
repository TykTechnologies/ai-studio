//go:build !enterprise

package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
)

// A Community edge gateway with signatures required (the default) still
// loads OCI plugins, recorded as unverified, rather than refusing them
// (midsommar-clean-c1g.22, c1g.16).
func TestHubSpokeContainer_CEFetchesOCIPluginUnverified(t *testing.T) {
	cfg := newOCIContainerConfig(t)
	c, err := NewHubSpokeServiceContainer(setupEdgeSyncTestDB(t), cfg)
	require.NoError(t, err)
	t.Cleanup(c.Cleanup)
	client := c.PluginManager.GetOCIClient()
	require.NotNil(t, client)

	reg := ocitest.NewRegistry(t)
	reg.PushPlugin("plugins/demo", "1.0.0", "linux/amd64", []byte("unsigned binary"))
	ref := &ociplugins.OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Tag: "1.0.0"}
	local, err := client.FetchPlugin(context.Background(), ref, &ociplugins.OCIPluginParams{Architecture: "linux/amd64"})
	require.NoError(t, err, "CE loads OCI plugins when signatures are required")
	assert.False(t, local.Verified)
}
