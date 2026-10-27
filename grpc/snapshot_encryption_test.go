package grpc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/fips140"
	"encoding/base64"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decryptLikeMicrogateway mirrors how edges released before the FIPS work
// decrypt (microgateway/internal/services/crypto_service.go then):
// base64 → [12-byte nonce | GCM ciphertext] → plaintext, with cipher.NewGCM.
// Edges now read the same layout with cipher.NewGCMWithRandomNonce.
func decryptLikeMicrogateway(t *testing.T, key, encoded string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	block, err := aes.NewCipher([]byte(key))
	require.NoError(t, err)
	var plain []byte
	if fips140.Enforced() {
		// cipher.NewGCM is refused under fips140=only; read as current edges do.
		gcm, err := cipher.NewGCMWithRandomNonce(block)
		require.NoError(t, err)
		plain, err = gcm.Open(nil, nil, raw, nil)
		require.NoError(t, err)
		return string(plain)
	}
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	require.Greater(t, len(raw), gcm.NonceSize())
	plain, err = gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	require.NoError(t, err)
	return string(plain)
}

func TestEncryptForMicrogateway_RandomNonceAndDecryptable(t *testing.T) {
	server, _ := setupTestServer(t, nil)
	key := os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")
	require.Len(t, key, 32)

	a1, err := server.encryptForMicrogateway("sk-secret-one")
	require.NoError(t, err)
	a2, err := server.encryptForMicrogateway("sk-secret-one")
	require.NoError(t, err)

	assert.NotEqual(t, a1, a2, "each encryption draws a fresh nonce")
	assert.Equal(t, "sk-secret-one", decryptLikeMicrogateway(t, key, a1))
	assert.Equal(t, "sk-secret-one", decryptLikeMicrogateway(t, key, a2))
}

// Regenerating a snapshot of unchanged configuration must yield the same
// checksum, otherwise edges can never report themselves in sync, though the
// ciphertexts differ.
func TestSnapshotChecksum_StableAcrossGenerations(t *testing.T) {
	server, db := setupTestServer(t, nil)
	createTestLLMs(db, "")

	first, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	second, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)

	require.NotEmpty(t, first.Llms, "test LLMs with API keys must be in the snapshot")
	require.NotEmpty(t, first.Llms[0].ApiKeyEncrypted)
	assert.NotEqual(t, first.Llms[0].ApiKeyEncrypted, second.Llms[0].ApiKeyEncrypted, "random nonces")
	assert.NotEmpty(t, first.Checksum)
	assert.Equal(t, first.Checksum, second.Checksum)
}

// A changed secret changes the checksum, so edges show as pending until the
// change is pushed.
func TestSnapshotChecksum_ChangesWithASecret(t *testing.T) {
	server, db := setupTestServer(t, nil)
	createTestLLMs(db, "")

	before, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE llms SET api_key = ? WHERE id = (SELECT MIN(id) FROM llms)", "sk-rotated-key").Error)
	after, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	assert.NotEqual(t, before.Checksum, after.Checksum)
}

// The digests are keyed: another encryption key gives other digests.
func TestSnapshotDigest_Keyed(t *testing.T) {
	d1, err := digestFor("12345678901234567890123456789012", "sk")
	require.NoError(t, err)
	d2, err := digestFor("another-32-char-key-for-testing!", "sk")
	require.NoError(t, err)
	d3, err := digestFor("12345678901234567890123456789012", "sk2")
	require.NoError(t, err)
	assert.Len(t, d1, 64)
	assert.NotEqual(t, d1, d2)
	assert.NotEqual(t, d1, d3)
}

// normalize swaps only the ciphertexts present, forgets them, and drops
// stale entries of snapshots never checksummed.
func TestSnapshotDigests_Normalize(t *testing.T) {
	var d snapshotDigests
	now := time.Now()
	d.record("CIPHERTEXT-A-0123456789012345678901234567890123", "da")
	d.record("CIPHERTEXT-B-0123456789012345678901234567890123", "db")
	d.m.Store("CIPHERTEXT-OLD-012345678901234567890123456789", snapshotDigest{digest: "old", created: now.Add(-time.Hour)})

	out := d.normalize([]byte(`x{"k":"CIPHERTEXT-A-0123456789012345678901234567890123"}y`), now)
	assert.Equal(t, `x{"k":"hmac:da"}y`, string(out))

	_, okA := d.m.Load("CIPHERTEXT-A-0123456789012345678901234567890123")
	_, okB := d.m.Load("CIPHERTEXT-B-0123456789012345678901234567890123")
	_, okOld := d.m.Load("CIPHERTEXT-OLD-012345678901234567890123456789")
	assert.False(t, okA, "used entries are forgotten")
	assert.True(t, okB, "another snapshot's entry stays")
	assert.False(t, okOld, "stale entries are dropped")
}
