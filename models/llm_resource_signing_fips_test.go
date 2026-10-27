package models

import (
	"crypto/fips140"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Signatures of standard builds, pinned: resource IDs already handed out are
// signed with these keys (and the backfill stores them).
var credentialKeyFixtures = map[string]string{
	"abcd":                                  "cac626d4fa7f3200e579e809ac0707e703149cab4892689ef9f190cad456bdca",
	"vendor-key":                            "ab3e20cddd85e9da4553cc68a2befc876c44aee03ecd8a569622b5ab38479f0b",
	"sk-ant-api03-a-long-vendor-credential": "2aed1c4d5ba98d00b06e9f51b2c12fe0fd863f1863643b2aa2a3af11726f857c",
}

func forceFIPS(t *testing.T) {
	t.Helper()
	prev := fipsEnabled
	fipsEnabled = func() bool { return true }
	t.Cleanup(func() { fipsEnabled = prev })
}

func TestDeriveCredentialSigningKey_StandardBuildUnchanged(t *testing.T) {
	if fips140.Enabled() {
		t.Skip("standard-build derivation; this process runs in FIPS mode")
	}
	for cred, want := range credentialKeyFixtures {
		k, err := DeriveCredentialSigningKey(cred)
		require.NoError(t, err)
		assert.Equal(t, want, hex.EncodeToString(k), cred)
	}
}

func TestDeriveCredentialSigningKey_FIPSMode(t *testing.T) {
	forceFIPS(t)

	// A 4-byte credential is hashed before HKDF, so it is a 256-bit key.
	k, err := DeriveCredentialSigningKey("abcd")
	require.NoError(t, err)
	sum := sha256.Sum256([]byte("abcd"))
	want, err := hkdf.Key(sha256.New, sum[:], nil, credentialKeyInfo, sha256.Size)
	require.NoError(t, err)
	assert.Equal(t, want, k)
	assert.NotEqual(t, credentialKeyFixtures["abcd"], hex.EncodeToString(k))

	// Credentials of 112 bits or more derive the standard-build key.
	long := "sk-ant-api03-a-long-vendor-credential"
	k, err = DeriveCredentialSigningKey(long)
	require.NoError(t, err)
	assert.Equal(t, credentialKeyFixtures[long], hex.EncodeToString(k))
	exactly14 := "14-byte-secret"
	require.Len(t, exactly14, minHKDFKeyBytes)
	assert.Equal(t, []byte(exactly14), HKDFKeyMaterial(exactly14))
}

// Under GODEBUG=fips140=only (make test-fips) this runs with the real module:
// a short credential must still derive a key.
func TestDeriveCredentialSigningKey_ShortCredentialInEveryMode(t *testing.T) {
	k, err := DeriveCredentialSigningKey("abcd")
	require.NoError(t, err)
	assert.Len(t, k, sha256.Size)
}
