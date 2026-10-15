package ociplugins

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cacheTestArch = "linux/amd64"

// cachedPluginFixture stores binary in a fresh client's cache, with metadata
// written by meta (nil writes none), and returns the client and the
// reference to load it by.
func cachedPluginFixture(t *testing.T, requireSignature bool, binary []byte, meta func(*PluginMetadata)) (*OCIPluginClient, *OCIReference, *OCIPluginParams) {
	t.Helper()
	config := DefaultOCIConfig()
	config.CacheDir = t.TempDir()
	config.RequireSignature = requireSignature
	client, err := NewOCIPluginClient(config)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	ref := &OCIReference{Registry: "registry.example.com", Repository: "plugins/demo", Digest: digestOf([]byte("manifest"))}
	params := &OCIPluginParams{Architecture: cacheTestArch}
	_, err = client.storage.StoreExecutable(ref.Digest, cacheTestArch, binary)
	require.NoError(t, err)

	if meta != nil {
		sum := sha256.Sum256(binary)
		m := &PluginMetadata{
			Reference:        ref,
			Params:           params,
			FetchTime:        time.Now().Add(-time.Hour),
			Verified:         requireSignature,
			Size:             int64(len(binary)),
			BinarySHA256:     hex.EncodeToString(sum[:]),
			SignatureChecked: requireSignature,
		}
		meta(m)
		require.NoError(t, client.storage.StoreMetadata(ref.Digest, cacheTestArch, m))
	}
	return client, ref, params
}

func requireCacheCorrupted(t *testing.T, err error) {
	t.Helper()
	var corrupted *ErrCacheCorrupted
	require.Error(t, err)
	require.True(t, errors.As(err, &corrupted), "error %v is not ErrCacheCorrupted", err)
}

func TestGetPlugin_IntactCacheEntryLoads(t *testing.T) {
	for _, requireSignature := range []bool{false, true} {
		client, ref, params := cachedPluginFixture(t, requireSignature, []byte("plugin binary"), func(*PluginMetadata) {})

		local, err := client.GetPlugin(ref, params)
		require.NoError(t, err)
		assert.Equal(t, requireSignature, local.Verified)
		assert.Equal(t, client.storage.GetExecutablePath(ref.Digest, cacheTestArch), local.ExecutablePath)
	}
}

func TestGetPlugin_TamperedBinaryIsRefused(t *testing.T) {
	for _, requireSignature := range []bool{false, true} {
		client, ref, params := cachedPluginFixture(t, requireSignature, []byte("plugin binary"), func(*PluginMetadata) {})
		path := client.storage.GetExecutablePath(ref.Digest, cacheTestArch)
		require.NoError(t, os.WriteFile(path, []byte("tampered binary"), 0o700))

		_, err := client.GetPlugin(ref, params)
		requireCacheCorrupted(t, err)
		assert.Contains(t, err.Error(), "does not match")
	}
}

func TestGetPlugin_EntryWithoutHash(t *testing.T) {
	cases := map[string]func(*PluginMetadata){
		"no metadata":                  nil,
		"metadata from an old release": func(m *PluginMetadata) { m.BinarySHA256 = "" },
	}
	for name, meta := range cases {
		t.Run(name+"/signatures optional", func(t *testing.T) {
			// Callers fetch it again on any GetPlugin error, which records
			// its hash (see TestFetchPlugin_UnhashedEntryIsFetchedOnceToRecordItsHash).
			client, ref, params := cachedPluginFixture(t, false, []byte("plugin binary"), meta)
			_, err := client.GetPlugin(ref, params)
			var unchecked *ErrCacheUnchecked
			require.True(t, errors.As(err, &unchecked), "error %v is not ErrCacheUnchecked", err)
		})
		t.Run(name+"/signatures required", func(t *testing.T) {
			client, ref, params := cachedPluginFixture(t, true, []byte("plugin binary"), meta)
			_, err := client.GetPlugin(ref, params)
			requireCacheCorrupted(t, err)
		})
	}
}

// An entry cached while signatures were optional is not trusted once they
// are required.
func TestGetPlugin_UnverifiedEntryRefusedWhenSignaturesRequired(t *testing.T) {
	client, ref, params := cachedPluginFixture(t, true, []byte("plugin binary"), func(m *PluginMetadata) { m.SignatureChecked = false })
	_, err := client.GetPlugin(ref, params)
	requireCacheCorrupted(t, err)
}

// An entry verified with one public key is not reused for a load that asks
// for another.
func TestGetPlugin_EntryVerifiedWithOtherKeyRefused(t *testing.T) {
	client, ref, params := cachedPluginFixture(t, true, []byte("plugin binary"), func(m *PluginMetadata) { m.SignatureKey = "key-1" })

	params.PublicKey = "key-2"
	_, err := client.GetPlugin(ref, params)
	requireCacheCorrupted(t, err)
	assert.Contains(t, err.Error(), "different public key")

	params.PublicKey = "key-1"
	_, err = client.GetPlugin(ref, params)
	require.NoError(t, err)
}

// A refetch after a failed check must replace the cached file, so storing
// over an existing executable overwrites it.
func TestStoreExecutable_ReplacesExistingFile(t *testing.T) {
	s, err := NewContentStorage(t.TempDir(), 0, 0)
	require.NoError(t, err)
	digest := digestOf([]byte("manifest"))

	path, err := s.StoreExecutable(digest, cacheTestArch, []byte("tampered"))
	require.NoError(t, err)
	_, err = s.StoreExecutable(digest, cacheTestArch, []byte("fetched again"))
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "fetched again", string(got))
}
