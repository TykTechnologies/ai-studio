package secrets

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveKeyLength(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple string", "my-secret-key"},
		{"empty string", ""},
		{"long string", "this-is-a-very-long-string-that-is-longer-than-32-bytes-but-should-still-work"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := deriveKey(tt.input)
			assert.Equal(t, 32, len(key))

			key2 := deriveKey(tt.input)
			assert.Equal(t, key, key2)
		})
	}

	k1 := deriveKey("key-a")
	k2 := deriveKey("key-b")
	assert.NotEqual(t, k1, k2)
}

func TestDetectVersion(t *testing.T) {
	tests := []struct {
		input       string
		wantVersion string
		wantPayload string
	}{
		{"someBase64Data", "v1", "someBase64Data"},
		{"v2/1/someBase64Data", "v2", "1/someBase64Data"},
		{"v2/", "v2", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			version, payload := detectVersion(tt.input)
			assert.Equal(t, tt.wantVersion, version)
			assert.Equal(t, tt.wantPayload, payload)
		})
	}
}

func TestCFBCipherRoundTrip(t *testing.T) {
	c := &cfbCipher{}
	ctx := context.Background()
	key := deriveKey("test-key")

	assert.Equal(t, "v1", c.Version())

	tests := []string{
		"hello world",
		"",
		"this is a longer string with special chars: !@#$%^&*()",
	}

	for _, plaintext := range tests {
		t.Run(plaintext, func(t *testing.T) {
			encrypted, err := c.Encrypt(ctx, key, []byte(plaintext))
			require.NoError(t, err)

			decrypted, err := c.Decrypt(ctx, key, encrypted)
			require.NoError(t, err)
			assert.Equal(t, plaintext, string(decrypted))
		})
	}
}

func TestCFBCipherDecryptErrors(t *testing.T) {
	c := &cfbCipher{}
	ctx := context.Background()
	key := deriveKey("test-key")

	_, err := c.Decrypt(ctx, key, []byte("short"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestEncryptValuePassthrough(t *testing.T) {
	c := &cfbCipher{}
	ctx := context.Background()

	result, err := encryptWith(ctx, c, "key", "")
	require.NoError(t, err)
	assert.Equal(t, "", result)

	result, err = encryptWith(ctx, c, "key", "[redacted]")
	require.NoError(t, err)
	assert.Equal(t, "[redacted]", result)
}

func TestEncryptDecryptValueRoundTrip_V1(t *testing.T) {
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	encrypted, err := encryptWith(ctx, &cfbCipher{}, "my-key", "hello")
	require.NoError(t, err)
	assert.True(t, len(encrypted) > 0)
	assert.Contains(t, encrypted, "$ENC/")

	decrypted, err := decryptWith(ctx, ciphers, "my-key", encrypted)
	require.NoError(t, err)
	assert.Equal(t, "hello", decrypted)
}

func TestDecryptWith_PlainPrefix(t *testing.T) {
	rawKey := "test-key-1234567890123456"
	value := "$PLAIN/this-is-not-encrypted"

	// $PLAIN/ prefix is not supported - only main branch format (unprefixed) and v2 envelope
	result, err := DecryptWith(rawKey, value, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported encryption format")
	assert.Empty(t, result)
}

func TestDecryptWith_PlainPrefix_EmptyValue(t *testing.T) {
	value := "$PLAIN/"
	// $PLAIN/ prefix is not supported
	result, err := DecryptWith("key", value, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported encryption format")
	assert.Empty(t, result)
}

func TestDecryptWith_UnprefixedLegacy(t *testing.T) {
	ciphers := legacyCipherInstances()
	ctx := context.Background()
	rawKey := "legacy-key"

	// Encrypt with v1 cipher, get raw base64 (no $ENC/ prefix)
	v1 := &cfbCipher{}
	key := deriveKey(rawKey)
	ct, err := v1.Encrypt(ctx, key, []byte("legacy-secret"))
	require.NoError(t, err)
	raw := base64.URLEncoding.EncodeToString(ct)

	// decryptWith should handle unprefixed as legacy v1
	result, err := decryptWith(ctx, ciphers, rawKey, raw)
	require.NoError(t, err)
	assert.Equal(t, "legacy-secret", result)
}

func TestDecryptWith_EmptyString(t *testing.T) {
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	result, err := decryptWith(ctx, ciphers, "key", "")
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestDecryptWith_UnprefixedNonBase64_ReturnsError(t *testing.T) {
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	// Unprefixed non-base64 string is treated as legacy v1 — base64 decode fails
	_, err := decryptWith(ctx, ciphers, "key", "not-valid-base64!!!")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode legacy base64")
}

func TestDecryptWith_NoRawKey_Passthrough(t *testing.T) {
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	// No rawKey: unprefixed values pass through as-is
	result, err := decryptWith(ctx, ciphers, "", "some-value")
	require.NoError(t, err)
	assert.Equal(t, "some-value", result)
}

func TestDecryptValueUnsupportedVersion(t *testing.T) {
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	_, err := decryptWith(ctx, ciphers, "key", "$ENC/v99/somedata")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported cipher version")
}

func TestLegacyCipherInstances(t *testing.T) {
	ciphers := legacyCipherInstances()
	assert.Contains(t, ciphers, "v1")
	assert.NotContains(t, ciphers, "v2")
	assert.Equal(t, "v1", ciphers["v1"].Version())
}
