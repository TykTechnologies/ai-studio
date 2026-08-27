package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Issue #395: malformed or truncated encrypted values must surface as errors,
// never as a panic that takes down the process.

func TestDecryptMalformedInputReturnsError(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "empty ciphertext", key: "my-key", value: ""},
		{name: "ciphertext shorter than block size", key: "my-key", value: "YWJj"}, // "abc" base64
		{name: "invalid base64", key: "my-key", value: "!!!not-base64!!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				_, err := decrypt(tt.key, tt.value)
				assert.Error(t, err, "malformed ciphertext should return an error")
			})
		})
	}
}

func TestDecryptValueMalformedReturnsOriginal(t *testing.T) {
	t.Setenv(midsommarSecret, "test-key")

	// A "$ENC/" prefixed value whose payload is garbage must not panic and
	// must fall back to returning the stored value untouched.
	malformed := "$ENC/YWJj"
	assert.NotPanics(t, func() {
		got := DecryptValue(malformed)
		assert.Equal(t, malformed, got)
	})
}

func TestDecryptValueRoundTrip(t *testing.T) {
	t.Setenv(midsommarSecret, "test-key")

	enc := EncryptValue("super-secret")
	assert.NotEqual(t, "super-secret", enc)
	assert.Equal(t, "super-secret", DecryptValue(enc))
}

func TestFilterSesitiveFieldsArrNonSliceDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		got := FilterSesitiveFieldsArr("not-a-slice")
		assert.Equal(t, "not-a-slice", got)
	})

	assert.NotPanics(t, func() {
		got := FilterSesitiveFieldsArr(42)
		assert.Equal(t, 42, got)
	})
}
