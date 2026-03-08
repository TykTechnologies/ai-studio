package secrets

import (
	"context"
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

func TestDecryptValueNonEncrypted(t *testing.T) {
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	result, err := decryptWith(ctx, ciphers, "key", "plain-text")
	require.NoError(t, err)
	assert.Equal(t, "plain-text", result)
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
