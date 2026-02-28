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
		{"v2/someBase64Data", "v2", "someBase64Data"},
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

func TestGCMCipherRoundTrip(t *testing.T) {
	c := &gcmCipher{}
	ctx := context.Background()
	key := deriveKey("test-key")

	assert.Equal(t, "v2", c.Version())

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

func TestGCMCipherTamperDetection(t *testing.T) {
	c := &gcmCipher{}
	ctx := context.Background()
	key := deriveKey("test-key")

	encrypted, err := c.Encrypt(ctx, key, []byte("sensitive data"))
	require.NoError(t, err)

	encrypted[len(encrypted)-1] ^= 0xff

	_, err = c.Decrypt(ctx, key, encrypted)
	assert.Error(t, err, "GCM should detect tampered ciphertext")
}

func TestGCMCipherDecryptErrors(t *testing.T) {
	c := &gcmCipher{}
	ctx := context.Background()
	key := deriveKey("test-key")

	_, err := c.Decrypt(ctx, key, []byte("short"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestGCMCipherWrongKey(t *testing.T) {
	c := &gcmCipher{}
	ctx := context.Background()
	key1 := deriveKey("key-1")
	key2 := deriveKey("key-2")

	encrypted, err := c.Encrypt(ctx, key1, []byte("secret"))
	require.NoError(t, err)

	_, err = c.Decrypt(ctx, key2, encrypted)
	assert.Error(t, err, "GCM should reject wrong key")
}

func TestEncryptValuePassthrough(t *testing.T) {
	c := DefaultCipherInstance()
	ctx := context.Background()

	result, err := EncryptWith(ctx, c, "key", "")
	require.NoError(t, err)
	assert.Equal(t, "", result)

	result, err = EncryptWith(ctx, c, "key", "[redacted]")
	require.NoError(t, err)
	assert.Equal(t, "[redacted]", result)
}

func TestEncryptDecryptValueRoundTrip(t *testing.T) {
	ciphers := AllCipherInstances()
	ctx := context.Background()

	encrypted, err := EncryptWith(ctx, &gcmCipher{}, "my-key", "hello")
	require.NoError(t, err)
	assert.True(t, len(encrypted) > 0)
	assert.Contains(t, encrypted, "$ENC/v2/")

	decrypted, err := DecryptWith(ctx, ciphers, "my-key", encrypted)
	require.NoError(t, err)
	assert.Equal(t, "hello", decrypted)
}

func TestDecryptValueLegacyFormat(t *testing.T) {
	ciphers := AllCipherInstances()
	ctx := context.Background()

	encrypted, err := EncryptWith(ctx, &cfbCipher{}, "my-key", "legacy-secret")
	require.NoError(t, err)
	assert.True(t, len(encrypted) > 0)
	assert.NotContains(t, encrypted, "$ENC/v2/")

	decrypted, err := DecryptWith(ctx, ciphers, "my-key", encrypted)
	require.NoError(t, err)
	assert.Equal(t, "legacy-secret", decrypted)
}

func TestDecryptValueNonEncrypted(t *testing.T) {
	ciphers := AllCipherInstances()
	ctx := context.Background()

	result, err := DecryptWith(ctx, ciphers, "key", "plain-text")
	require.NoError(t, err)
	assert.Equal(t, "plain-text", result)
}

func TestDecryptValueUnsupportedVersion(t *testing.T) {
	ciphers := map[string]Cipher{"v1": &cfbCipher{}}
	ctx := context.Background()

	_, err := DecryptWith(ctx, ciphers, "key", "$ENC/v99/somedata")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported cipher version")
}

func TestAllCipherInstances(t *testing.T) {
	ciphers := AllCipherInstances()
	assert.Contains(t, ciphers, "v1")
	assert.Contains(t, ciphers, "v2")
	assert.Equal(t, "v1", ciphers["v1"].Version())
	assert.Equal(t, "v2", ciphers["v2"].Version())
}

func TestDefaultCipherInstance(t *testing.T) {
	c := DefaultCipherInstance()
	assert.Equal(t, "v2", c.Version())
}
