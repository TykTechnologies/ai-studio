package ociplugins

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const vendorKey = "env:OCI_PLUGINS_PUBKEY_VENDOR"

// keyedVerifier accepts a digest only with the key it was signed with, and
// records every (digest, key) it was asked about.
type keyedVerifier struct {
	stubVerifier
	signedWith map[string]string // digest -> key ID
	asked      []string          // "digest key"
}

func (v *keyedVerifier) VerifySignature(_ context.Context, ref *OCIReference, keyID string) error {
	v.asked = append(v.asked, ref.Digest+" "+keyID)
	if key, ok := v.signedWith[ref.Digest]; ok && key == keyID {
		return nil
	}
	return fmt.Errorf("no signature by %s", keyID)
}

// keyedFixture is fetchFixture with the embedded Tyk key and a vendor key
// configured, and the artifact signed with signer.
func keyedFixture(t *testing.T, signer string) (*OCIPluginClient, *countingPuller, *keyedVerifier, *OCIReference, *OCIPluginParams) {
	t.Helper()
	client, puller, _, ref, params := fetchFixture(t)
	client.config.DefaultPublicKeys = []string{"embedded:default", vendorKey}
	verifier := &keyedVerifier{signedWith: map[string]string{ref.Digest: signer}}
	client.SetSecurityService(verifier)
	return client, puller, verifier, ref, params
}

func signatureKeyOf(t *testing.T, client *OCIPluginClient, ref *OCIReference) string {
	t.Helper()
	meta, err := client.storage.LoadMetadata(ref.Digest, cacheTestArch)
	require.NoError(t, err)
	return meta.SignatureKey
}

// A plugin signed with an operator-added key installs from a command that
// names no key (as marketplace installs do): every configured key is tried,
// the Tyk key first, and the key that verified is recorded.
func TestUnpinned_SignedWithAddedKeyInstalls(t *testing.T) {
	client, puller, verifier, ref, params := keyedFixture(t, vendorKey)

	local, err := client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	assert.True(t, local.Verified)
	assert.Equal(t, []string{ref.Digest + " embedded:default", ref.Digest + " " + vendorKey}, verifier.asked)
	assert.Equal(t, vendorKey, signatureKeyOf(t, client, ref))

	_, err = client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	assert.Equal(t, 1, puller.pulls, "the cached copy is served while its key is configured")
}

// A Tyk-signed plugin verifies on the first key and tries no other.
func TestUnpinned_TykSignedTriesOneKey(t *testing.T) {
	client, _, verifier, ref, params := keyedFixture(t, "embedded:default")

	_, err := client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	assert.Equal(t, []string{ref.Digest + " embedded:default"}, verifier.asked)
	assert.Equal(t, "embedded:default", signatureKeyOf(t, client, ref))
}

// A command that names a key is checked with that key only.
func TestPinned_OtherKeyIsNotTried(t *testing.T) {
	client, _, verifier, ref, params := keyedFixture(t, vendorKey)
	params.PublicKey = "embedded:default"

	_, err := client.FetchPlugin(context.Background(), ref, params)
	var verr *ErrSignatureVerificationFailed
	require.True(t, errors.As(err, &verr), "want ErrSignatureVerificationFailed, got %v", err)
	assert.Equal(t, []string{ref.Digest + " embedded:default"}, verifier.asked)
	assert.False(t, client.HasPlugin(ref.Digest, cacheTestArch))
}

// When no configured key verifies, the error names every key tried.
func TestUnpinned_NoKeyVerifiesNamesEveryKey(t *testing.T) {
	client, _, _, ref, params := keyedFixture(t, "env:OCI_PLUGINS_PUBKEY_UNKNOWN")

	_, err := client.FetchPlugin(context.Background(), ref, params)
	var verr *ErrSignatureVerificationFailed
	require.True(t, errors.As(err, &verr), "want ErrSignatureVerificationFailed, got %v", err)
	assert.Contains(t, verr.Reason, `key "embedded:default"`)
	assert.Contains(t, verr.Reason, `key "`+vendorKey+`"`)
	assert.False(t, client.HasPlugin(ref.Digest, cacheTestArch))
}

// Removing a key from the configuration revokes the cached plugins it
// verified: the cache entry is refused and the refetch fails verification.
func TestUnpinned_RemovedKeyRevokesCachedPlugin(t *testing.T) {
	client, puller, _, ref, params := keyedFixture(t, vendorKey)
	_, err := client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)

	client.config.DefaultPublicKeys = []string{"embedded:default"}
	_, err = client.GetPlugin(ref, params)
	requireCacheCorrupted(t, err)

	_, err = client.FetchPlugin(context.Background(), ref, params)
	var verr *ErrSignatureVerificationFailed
	require.True(t, errors.As(err, &verr), "want ErrSignatureVerificationFailed, got %v", err)
	assert.Equal(t, 2, puller.pulls)
}

// An entry verified for a command naming a key is not served to one naming
// another, nor the reverse; an unpinned load verifies such an entry again.
func TestPinnedAndUnpinnedCacheEntriesAreKeptApart(t *testing.T) {
	client, _, _, ref, params := keyedFixture(t, "VENDOR")
	params.PublicKey = "VENDOR"
	_, err := client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	assert.Equal(t, "VENDOR", signatureKeyOf(t, client, ref))

	_, err = client.GetPlugin(ref, &OCIPluginParams{Architecture: cacheTestArch, PublicKey: "embedded:default"})
	requireCacheCorrupted(t, err)
	_, err = client.GetPlugin(ref, &OCIPluginParams{Architecture: cacheTestArch})
	requireCacheCorrupted(t, err) // "VENDOR" is not a configured default key ID
}

// An entry cached by an earlier release for an unpinned command recorded no
// key; it was checked with the default key and is still served unpinned.
func TestUnpinned_LegacyEntryWithoutKeyIsServed(t *testing.T) {
	client, ref, params := cachedPluginFixture(t, true, []byte("plugin binary"), func(m *PluginMetadata) { m.SignatureKey = "" })
	_, err := client.GetPlugin(ref, params)
	require.NoError(t, err)

	params.PublicKey = "VENDOR"
	_, err = client.GetPlugin(ref, params)
	requireCacheCorrupted(t, err)
}

// A multi-arch plugin whose platform manifest alone is signed, with an added
// key, falls back to the platform manifest and records that key.
func TestUnpinned_PlatformFallbackWithAddedKey(t *testing.T) {
	client, _, reg, index, amd64, _ := fallbackFixture(t)
	client.config.DefaultPublicKeys = []string{"embedded:default", vendorKey}
	verifier := &keyedVerifier{signedWith: map[string]string{amd64.Digest.String(): vendorKey}}
	client.SetSecurityService(verifier)

	_, err := client.FetchPlugin(context.Background(), tagRef(reg), &OCIPluginParams{Architecture: "linux/amd64"})
	require.NoError(t, err)
	assert.Equal(t, []string{
		index.Digest.String() + " embedded:default",
		index.Digest.String() + " " + vendorKey,
		amd64.Digest.String() + " embedded:default",
		amd64.Digest.String() + " " + vendorKey,
	}, verifier.asked)
	meta, err := client.storage.LoadMetadata(amd64.Digest.String(), "linux/amd64")
	require.NoError(t, err)
	assert.Equal(t, amd64.Digest.String(), meta.SignedDigest)
	assert.Equal(t, vendorKey, meta.SignatureKey)
}

func TestSignatureKeys(t *testing.T) {
	client := &OCIPluginClient{config: &OCIConfig{DefaultPublicKeys: []string{"embedded:default", vendorKey, "", vendorKey}}}
	assert.Equal(t, []string{"VENDOR"}, client.signatureKeys(&OCIPluginParams{PublicKey: "VENDOR"}))
	assert.Equal(t, []string{"embedded:default", vendorKey}, client.signatureKeys(&OCIPluginParams{}))

	client.config.DefaultPublicKeys = nil
	assert.Equal(t, []string{""}, client.signatureKeys(&OCIPluginParams{}), "no keys leaves the verifier to refuse")
}

// Every key ID LoadPublicKeysFromEnv lists resolves, so an unpinned check
// can try each of them by ID.
func TestDefaultKeyIDsResolve(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "vendor.pub")
	require.NoError(t, os.WriteFile(keyFile, []byte(DefaultTykPublicKey), 0o600))
	t.Setenv("OCI_PLUGINS_PUBKEY_1", DefaultTykPublicKey)
	t.Setenv("OCI_PLUGINS_PUBKEY_VENDOR", DefaultTykPublicKey)
	t.Setenv("OCI_PLUGINS_PUBKEY_FILE_OTHER", keyFile)

	cfg := &OCIConfig{DefaultPublicKeys: LoadPublicKeysFromEnv()}
	require.Len(t, cfg.DefaultPublicKeys, 4)
	for _, keyID := range cfg.DefaultPublicKeys {
		_, err := ResolvePublicKey(cfg, keyID)
		assert.NoError(t, err, keyID)
	}
}
