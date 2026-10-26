package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/fips140"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The crypto service moved from cipher.NewGCM with a caller-drawn nonce to
// cipher.NewGCMWithRandomNonce (approved in FIPS mode). Both use the
// nonce || ciphertext layout, so values written before the change must still
// decrypt, and values written now must still decrypt with the old reader.

const compatKey = "12345678901234567890123456789012"

// callerNonceFixture was produced by Encrypt("fips-fixture-plaintext") with
// compatKey before the switch to NewGCMWithRandomNonce.
const callerNonceFixture = "ijxy9MPBGBTFII18s/at6Za3tQQes351M3lmIlicbXYRdZl+U4pbhqplrmTRc1IPBwM="

func TestCryptoService_DecryptsCallerNonceFixture(t *testing.T) {
	dec, err := NewCryptoService(compatKey).Decrypt(callerNonceFixture)
	require.NoError(t, err)
	assert.Equal(t, "fips-fixture-plaintext", dec)
}

func TestCryptoService_NewCiphertextsDecryptWithOldReader(t *testing.T) {
	if fips140.Enforced() {
		t.Skip("cipher.NewGCM with a caller nonce is refused under fips140=only; the old reader runs in standard builds")
	}
	enc, err := NewCryptoService(compatKey).Encrypt("fresh-value")
	require.NoError(t, err)

	// The old Decrypt: split the nonce off by hand and open with cipher.NewGCM.
	data, err := base64.StdEncoding.DecodeString(enc)
	require.NoError(t, err)
	block, err := aes.NewCipher([]byte(compatKey))
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	plaintext, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	require.NoError(t, err)
	assert.Equal(t, "fresh-value", string(plaintext))
}
