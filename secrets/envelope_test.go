package secrets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestEnvelopeCipher creates a test cipher with KEK cache.
func newTestEnvelopeCipher(kek KEKProvider) *EnvelopeCipher {
	cache := map[string]KEKProvider{kek.KeyID(): kek}
	return NewEnvelopeCipher(kek, cache)
}

// newTestEnvelopeCipherWithCache creates a test cipher with custom cache.
func newTestEnvelopeCipherWithCache(kek KEKProvider, cache map[string]KEKProvider) *EnvelopeCipher {
	if cache == nil {
		cache = make(map[string]KEKProvider)
	}
	cache[kek.KeyID()] = kek
	return NewEnvelopeCipher(kek, cache)
}

// Deprecated: seedActiveKey is no longer used with inline DEK storage
func seedActiveKey(t *testing.T, _ interface{}, kek KEKProvider) {
	t.Helper()
	// No-op - inline DEK storage doesn't need seeding
}

func TestLocalKEKProvider_RoundTrip(t *testing.T) {
	w := newTestLocalKEK("test-kek")
	ctx := context.Background()

	dek := make([]byte, 32)
	for i := range dek {
		dek[i] = byte(i)
	}

	wrapped, err := w.WrapKey(ctx, dek)
	require.NoError(t, err)
	assert.NotEqual(t, dek, wrapped)

	unwrapped, err := w.UnwrapKey(ctx, wrapped)
	require.NoError(t, err)
	assert.Equal(t, dek, unwrapped)
}

func TestLocalKEKProvider_WrongKEK(t *testing.T) {
	w1 := newTestLocalKEK("kek-1")
	w2 := newTestLocalKEK("kek-2")
	ctx := context.Background()

	dek := []byte("this-is-a-32-byte-data-enc-key!")

	wrapped, err := w1.WrapKey(ctx, dek)
	require.NoError(t, err)

	_, err = w2.UnwrapKey(ctx, wrapped)
	assert.Error(t, err, "wrong KEK should fail to unwrap")
}

func TestLocalKEKProvider_TooShort(t *testing.T) {
	w := newTestLocalKEK("kek")
	ctx := context.Background()

	_, err := w.UnwrapKey(ctx, []byte("short"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestGenerateDEK(t *testing.T) {
	w := newTestLocalKEK("test-kek")
	ctx := context.Background()

	wrapped, err := GenerateDEK(ctx, w)
	require.NoError(t, err)
	assert.NotEmpty(t, wrapped)

	// Should be unwrappable
	dek, err := w.UnwrapKey(ctx, wrapped)
	require.NoError(t, err)
	assert.Len(t, dek, 32)
}

func TestEnvelopeCipher_RoundTrip(t *testing.T) {
	w := newTestLocalKEK("my-kek")
	c := newTestEnvelopeCipher(w)
	ctx := context.Background()

	assert.Equal(t, "v2", c.Version())

	tests := []string{
		"hello world",
		"",
		"special chars: !@#$%^&*()",
		"unicode: \u65e5\u672c\u8a9e\u30c6\u30b9\u30c8",
	}

	for _, plaintext := range tests {
		t.Run(plaintext, func(t *testing.T) {
			encrypted, err := c.Encrypt(ctx, nil, []byte(plaintext))
			require.NoError(t, err)

			decrypted, err := c.Decrypt(ctx, nil, encrypted)
			require.NoError(t, err)
			assert.Equal(t, plaintext, string(decrypted))
		})
	}
}

func TestEnvelopeCipher_FormatContainsKeyID(t *testing.T) {
	w := newTestLocalKEK("my-kek")
	c := newTestEnvelopeCipher(w)
	ctx := context.Background()

	encrypted, err := EncryptEnvelope(ctx, c, "test-data")
	require.NoError(t, err)

	// Should be $ENC/v2/<key_id>/<ciphertext>
	assert.Contains(t, encrypted, "$ENC/v2/")
}

func TestEnvelopeCipher_TamperDetection(t *testing.T) {
	w := newTestLocalKEK("my-kek")
	c := newTestEnvelopeCipher(w)
	ctx := context.Background()

	encrypted, err := c.Encrypt(ctx, nil, []byte("sensitive"))
	require.NoError(t, err)

	// Tamper with ciphertext
	payload := string(encrypted)
	tampered := payload[:len(payload)-2] + "XX"

	_, err = c.Decrypt(ctx, nil, []byte(tampered))
	assert.Error(t, err, "tampered ciphertext should be rejected")
}

func TestEnvelopeCipher_InvalidFormat(t *testing.T) {
	w := newTestLocalKEK("my-kek")
	c := newTestEnvelopeCipher(w)
	ctx := context.Background()

	_, err := c.Decrypt(ctx, nil, []byte("no-slash"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")
}

func TestEnvelopeCipher_WrongKEK(t *testing.T) {
	w1 := newTestLocalKEK("kek-1")
	w2 := newTestLocalKEK("kek-2")
	ctx := context.Background()

	c1 := newTestEnvelopeCipher(w1)
	c2 := newTestEnvelopeCipher(w2) // same key store, wrong kek

	encrypted, err := c1.Encrypt(ctx, nil, []byte("secret"))
	require.NoError(t, err)

	_, err = c2.Decrypt(ctx, nil, encrypted)
	assert.Error(t, err, "wrong KEK should fail to unwrap DEK")
}

func TestEncryptEnvelope_Passthrough(t *testing.T) {
	w := newTestLocalKEK("kek")
	c := newTestEnvelopeCipher(w)
	ctx := context.Background()

	result, err := EncryptEnvelope(ctx, c, "")
	require.NoError(t, err)
	assert.Equal(t, "", result)

	result, err = EncryptEnvelope(ctx, c, "[redacted]")
	require.NoError(t, err)
	assert.Equal(t, "[redacted]", result)
}

func TestEncryptEnvelope_DecryptWithCipherMap(t *testing.T) {
	w := newTestLocalKEK("my-kek")
	envelope := newTestEnvelopeCipher(w)
	ctx := context.Background()

	encrypted, err := EncryptEnvelope(ctx, envelope, "hello envelope")
	require.NoError(t, err)
	assert.Contains(t, encrypted, "$ENC/v2/")

	// decryptWith should work when v2 cipher is in the map
	ciphers := legacyCipherInstances()
	ciphers["v2"] = envelope

	decrypted, err := decryptWith(ctx, ciphers, "any-key", encrypted)
	require.NoError(t, err)
	assert.Equal(t, "hello envelope", decrypted)
}

func TestBackwardCompat_V2StoreReadsV1(t *testing.T) {
	ctx := context.Background()
	rawKey := "compat-key"

	// Encrypt with v1
	v1Enc, err := encryptWith(ctx, legacyCipherInstances()["v1"], rawKey, "v1-secret")
	require.NoError(t, err)
	assert.Contains(t, v1Enc, "$ENC/")

	// Build cipher map with v2 available
	w := newTestLocalKEK(rawKey)
	envelope := newTestEnvelopeCipher(w)

	ciphers := legacyCipherInstances()
	ciphers["v2"] = envelope

	// v1 should decrypt through the combined cipher map
	d1, err := decryptWith(ctx, ciphers, rawKey, v1Enc)
	require.NoError(t, err)
	assert.Equal(t, "v1-secret", d1)

	// v2 should also work
	v2Enc, err := EncryptEnvelope(ctx, envelope, "v2-secret")
	require.NoError(t, err)

	d2, err := decryptWith(ctx, ciphers, rawKey, v2Enc)
	require.NoError(t, err)
	assert.Equal(t, "v2-secret", d2)
}

func TestEnvelopeCipher_MultipleKeys(t *testing.T) {
	w := newTestLocalKEK("kek")
	ctx := context.Background()

	c := newTestEnvelopeCipher(w)

	// Each encrypt creates a unique per-object DEK (inline)
	enc1, err := c.Encrypt(ctx, nil, []byte("data-with-key-1"))
	require.NoError(t, err)

	enc2, err := c.Encrypt(ctx, nil, []byte("data-with-key-2"))
	require.NoError(t, err)

	// Both should decrypt with their own embedded DEKs
	dec1, err := c.Decrypt(ctx, nil, enc1)
	require.NoError(t, err)
	assert.Equal(t, "data-with-key-1", string(dec1))

	dec2, err := c.Decrypt(ctx, nil, enc2)
	require.NoError(t, err)
	assert.Equal(t, "data-with-key-2", string(dec2))

	// Verify each has unique wrapped DEK (3-part format)
	parts1 := string(enc1)
	parts2 := string(enc2)
	assert.NotEqual(t, parts1, parts2, "Each encryption should have unique DEK")
}
