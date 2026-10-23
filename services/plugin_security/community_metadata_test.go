//go:build !enterprise
// +build !enterprise

package plugin_security

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
)

// With signatures required, the plugin metadata CE writes to the cache says
// that nothing checked the signature, so an Enterprise build sharing the
// cache verifies the plugin again (midsommar-clean-c1g.16).
func TestCommunityService_CachedMetadataIsNotSignatureChecked(t *testing.T) {
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
	for _, publicKey := range []string{"", "1"} { // default key and a numbered key
		ref := &ociplugins.OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Tag: "1.0.0"}
		local, err := client.FetchPlugin(context.Background(), ref,
			&ociplugins.OCIPluginParams{Architecture: "linux/amd64", PublicKey: publicKey})
		require.NoError(t, err, "CE loads plugins when signatures are required")
		assert.False(t, local.Verified, "key %q", publicKey)
	}

	var found int
	err = filepath.WalkDir(filepath.Join(cfg.CacheDir, "metadata"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var meta ociplugins.PluginMetadata
		require.NoError(t, json.Unmarshal(data, &meta), path)
		found++
		assert.False(t, meta.Verified, path)
		assert.False(t, meta.SignatureChecked, "%s: CE must not record a signature check", path)
		assert.Empty(t, meta.SignedDigest, path)
		return nil
	})
	require.NoError(t, err)
	require.NotZero(t, found, "no cached plugin metadata found; the cache layout changed")
}
