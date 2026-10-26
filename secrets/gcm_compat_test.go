package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/fips140"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The v2 format moved from cipher.NewGCM with a caller-drawn nonce to
// cipher.NewGCMWithRandomNonce (approved in FIPS mode). Both use the
// salt || nonce || ciphertext layout, so values written before the change
// must still decrypt, and values written now must still decrypt with the
// old reader (an older build reading a database written by this one).

// v2Fixtures were produced by encrypt with key "fixture-key" before the
// switch to NewGCMWithRandomNonce.
var v2Fixtures = map[string]string{
	"fips-fixture-plaintext": "v2:ljnBC9UpkbVhch5jlj2OzhnrEU2g8ciqrvTOUHO1We1HGKBkgHPUOItEGggGf-yp64yfMMjem-oB5XO90LtVR4Ge",
	"":                       "v2:e93bAIFPHEYkgz0DeB_pn24EGyHekK_BOSrsHg2vRsyaTFskl6Z7-Sh8Njo=",
}

func TestV2FixturesFromCallerNonceGCMDecrypt(t *testing.T) {
	for plaintext, enc := range v2Fixtures {
		dec, err := decrypt("fixture-key", enc)
		require.NoError(t, err)
		assert.Equal(t, plaintext, dec)

		s := Secret{Value: enc}
		assert.Equal(t, plaintext != "", s.HasValue(), "HasValue for %q", plaintext)
	}
}

// oldDecryptGCM is the v2 reader as it was before the switch: split the
// nonce off by hand and open with cipher.NewGCM.
func oldDecryptGCM(t *testing.T, keyString, value string) string {
	t.Helper()
	payload, err := base64.URLEncoding.DecodeString(strings.TrimPrefix(value, encVersionPrefix))
	require.NoError(t, err)
	key, err := deriveKeyScrypt(keyString, payload[:saltLength])
	require.NoError(t, err)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	rest := payload[saltLength:]
	plaintext, err := gcm.Open(nil, rest[:gcm.NonceSize()], rest[gcm.NonceSize():], nil)
	require.NoError(t, err)
	return string(plaintext)
}

func TestNewV2CiphertextsDecryptWithOldReader(t *testing.T) {
	if fips140.Enforced() {
		t.Skip("cipher.NewGCM with a caller nonce is refused under fips140=only; the old reader runs in standard builds")
	}
	for _, val := range []string{"", "short", "a much longer value with unicode ✓"} {
		enc, err := encrypt("compat-key", val)
		require.NoError(t, err)
		assert.Equal(t, val, oldDecryptGCM(t, "compat-key", enc))
	}
}
