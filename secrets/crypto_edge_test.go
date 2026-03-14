package secrets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecryptWith_PlainPrefix_SpecialChars(t *testing.T) {
	t.Parallel()
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"embedded ENC marker", "$PLAIN/hello$ENC/world", "hello$ENC/world"},
		{"empty suffix", "$PLAIN/", ""},
		{"nested PLAIN prefix", "$PLAIN/$PLAIN/nested", "$PLAIN/nested"},
		{"just dollar sign", "$PLAIN/$", "$"},
		{"unicode", "$PLAIN/日本語テスト", "日本語テスト"},
		{"spaces and newlines", "$PLAIN/hello world\nline2", "hello world\nline2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := decryptWith(ctx, ciphers, "key", tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestDecryptWith_ENC_UnknownVersion(t *testing.T) {
	t.Parallel()
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	_, err := decryptWith(ctx, ciphers, "key", "$ENC/v99/somedata")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported cipher version: v99")
}

func TestDecryptWith_ENCV2_InvalidKeyID(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("kek")
	envelope := newTestEnvelopeCipher(kek)
	ciphers := legacyCipherInstances()
	ciphers["v2"] = envelope
	ctx := context.Background()

	tests := []struct {
		name  string
		value string
	}{
		{"non-numeric key ID", "$ENC/v2/abc/data"},
		{"negative key ID", "$ENC/v2/-1/data"},
		{"zero key ID", "$ENC/v2/0/data"},
		{"overflow uint", "$ENC/v2/99999999999999999999/data"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := decryptWith(ctx, ciphers, "key", tt.value)
			assert.Error(t, err)
		})
	}
}

func TestDecryptWith_ENCV2_CorruptedCiphertext(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("kek")
	envelope := newTestEnvelopeCipher(kek)
	ciphers := legacyCipherInstances()
	ciphers["v2"] = envelope
	ctx := context.Background()

	// Valid key ID but garbage ciphertext
	_, err := decryptWith(ctx, ciphers, "key", "$ENC/v2/1/AAAA")
	assert.Error(t, err)
}

func TestDecryptWith_ENCV1_WrongKey(t *testing.T) {
	t.Parallel()
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	// Encrypt with one key
	v1 := &cfbCipher{}
	encrypted, err := encryptWith(ctx, v1, "correct-key", "secret")
	require.NoError(t, err)

	// Decrypt with wrong key — CFB doesn't authenticate, so this "succeeds"
	// but produces garbage. We just verify it doesn't panic.
	result, err := decryptWith(ctx, ciphers, "wrong-key", encrypted)
	require.NoError(t, err)
	assert.NotEqual(t, "secret", result)
}

func TestDecryptWith_UnprefixedValidBase64ButInvalidCiphertext(t *testing.T) {
	t.Parallel()
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	// Valid base64 but too short for AES-CFB (< blocksize)
	_, err := decryptWith(ctx, ciphers, "key", "AAAA")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestDecryptWith_UnprefixedNotBase64(t *testing.T) {
	t.Parallel()
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	_, err := decryptWith(ctx, ciphers, "key", "not-valid-base64!!!")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode legacy base64")
}

func TestDecryptWith_NoRawKey_ENCPrefix(t *testing.T) {
	t.Parallel()
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	// $ENC/ prefix with empty rawKey — rawKey check is AFTER $PLAIN/ but BEFORE $ENC/
	// so empty rawKey passes through for non-prefixed values, but $ENC/ is still processed
	result, err := decryptWith(ctx, ciphers, "", "some-value")
	require.NoError(t, err)
	assert.Equal(t, "some-value", result, "empty rawKey should passthrough non-prefixed values")
}

func TestDecryptWith_ENCWithoutSlashSuffix(t *testing.T) {
	t.Parallel()
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	// "$ENCfoo" — does NOT match "$ENC/" prefix, treated as unprefixed legacy
	_, err := decryptWith(ctx, ciphers, "key", "$ENCfoo")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode legacy base64")
}

func TestDecryptWith_ENCSlashOnly(t *testing.T) {
	t.Parallel()
	ciphers := legacyCipherInstances()
	ctx := context.Background()

	// "$ENC/" — trimmed to empty string, detectVersion returns "v1" + ""
	_, err := decryptWith(ctx, ciphers, "key", "$ENC/")
	assert.Error(t, err)
}

func TestDecryptWith_ENCV2SlashOnly(t *testing.T) {
	t.Parallel()
	ciphers := legacyCipherInstances()
	kek := newTestLocalKEK("kek")
	ciphers["v2"] = newTestEnvelopeCipher(kek)
	ctx := context.Background()

	// "$ENC/v2/" — v2 payload is empty, which is an invalid format for envelope
	_, err := decryptWith(ctx, ciphers, "key", "$ENC/v2/")
	assert.Error(t, err)
}

func TestDecryptWith_NilCipherMap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// $ENC/ prefix with nil cipher map
	_, err := decryptWith(ctx, nil, "key", "$ENC/v1/data")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported cipher version")
}

func TestDecryptWith_EmptyCipherMap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Unprefixed value with empty cipher map
	_, err := decryptWith(ctx, map[string]Cipher{}, "key", "$ENC/v1/data")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported cipher version")
}

func TestDecryptWith_EmptyCipherMap_UnprefixedLegacy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	_, err := decryptWith(ctx, map[string]Cipher{}, "key", "AAAA")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported cipher version: v1")
}

func TestDetectVersion_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input       string
		wantVersion string
		wantPayload string
	}{
		{"v99/data", "v99", "data"},
		{"v0/data", "v0", "data"},
		{"v123456/big-version", "v123456", "big-version"},
		{"vX/not-a-version", "v1", "vX/not-a-version"},        // non-digit after v
		{"v/no-number", "v1", "v/no-number"},                   // just "v" with no digits
		{"/leading-slash", "v1", "/leading-slash"},              // slash at position 0
		{"noslash", "v1", "noslash"},                            // no slash at all
		{"", "v1", ""},                                          // empty
		{"v2/", "v2", ""},                                       // version with empty payload
		{"v2/nested/slashes/in/payload", "v2", "nested/slashes/in/payload"}, // only first slash matters
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			version, payload := detectVersion(tt.input)
			assert.Equal(t, tt.wantVersion, version)
			assert.Equal(t, tt.wantPayload, payload)
		})
	}
}
