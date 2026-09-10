package grpc

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decryptLikeMicrogateway mirrors microgateway/internal/services/crypto_service.go:
// base64 → [12-byte nonce | GCM ciphertext] → plaintext.
func decryptLikeMicrogateway(t *testing.T, key, encoded string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	block, err := aes.NewCipher([]byte(key))
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	require.Greater(t, len(raw), gcm.NonceSize())
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	require.NoError(t, err)
	return string(plain)
}

func TestEncryptForMicrogateway_DeterministicAndDecryptable(t *testing.T) {
	server, _ := setupTestServer(t, nil)
	key := os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")
	require.Len(t, key, 32)

	a1, err := server.encryptForMicrogateway("sk-secret-one")
	require.NoError(t, err)
	a2, err := server.encryptForMicrogateway("sk-secret-one")
	require.NoError(t, err)
	b, err := server.encryptForMicrogateway("sk-secret-two")
	require.NoError(t, err)

	assert.Equal(t, a1, a2, "same plaintext must produce the same ciphertext so snapshot checksums are stable")
	assert.NotEqual(t, a1, b, "different plaintexts must not share a nonce or ciphertext")
	assert.Equal(t, "sk-secret-one", decryptLikeMicrogateway(t, key, a1))
	assert.Equal(t, "sk-secret-two", decryptLikeMicrogateway(t, key, b))

	// Nonce derivation is keyed: a different encryption key yields a different nonce.
	n1 := deriveSnapshotNonce(key, "sk-secret-one", 12)
	n2 := deriveSnapshotNonce("another-32-char-key-for-testing!", "sk-secret-one", 12)
	assert.Len(t, n1, 12)
	assert.NotEqual(t, n1, n2)
}

// Regenerating a snapshot of unchanged configuration must yield the same
// checksum, otherwise edges can never report themselves in sync.
func TestSnapshotChecksum_StableAcrossGenerations(t *testing.T) {
	server, db := setupTestServer(t, nil)
	createTestLLMs(db, "")

	first, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	second, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)

	require.NotEmpty(t, first.Llms, "test LLMs with API keys must be in the snapshot")
	assert.Equal(t, first.Llms[0].ApiKeyEncrypted, second.Llms[0].ApiKeyEncrypted)
	assert.Equal(t, first.Checksum, second.Checksum)
}
