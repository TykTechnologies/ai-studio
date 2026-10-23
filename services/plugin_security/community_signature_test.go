//go:build !enterprise
// +build !enterprise

package plugin_security

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
)

// With signatures required, CE still loads OCI plugins (it cannot verify
// them) but never reports them as verified.
func TestCommunityService_OCIPluginsAreNotReportedVerified(t *testing.T) {
	cfg := ociplugins.DefaultOCIConfig()
	cfg.CacheDir = t.TempDir()
	cfg.GCInterval = 0
	cfg.RequireSignature = true
	client, err := ociplugins.NewOCIPluginClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	client.SetSecurityService(NewService(&Config{OCIConfig: cfg}))

	reg := ocitest.NewRegistry(t)
	reg.PushPlugin("plugins/demo", "1.0.0", "linux/amd64", []byte("unsigned binary"))
	ref := &ociplugins.OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Tag: "1.0.0"}
	params := &ociplugins.OCIPluginParams{Architecture: "linux/amd64", PublicKey: "1"}

	local, err := client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err, "CE loads plugins when signatures are required")
	assert.False(t, local.Verified)

	cached, err := client.ListCached()
	require.NoError(t, err)
	require.Len(t, cached, 1)
	assert.False(t, cached[0].Verified, "the cache listing must not claim a CE plugin was verified")
}
