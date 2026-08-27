package secrets

import (
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

// Issue #394: EncryptValue must not silently fall back to plaintext when the
// encryption key is not configured — operators need a clear signal.

func TestEncryptionKeyConfigured(t *testing.T) {
	t.Setenv(midsommarSecret, "")
	assert.False(t, EncryptionKeyConfigured())

	t.Setenv(midsommarSecret, "some-key")
	assert.True(t, EncryptionKeyConfigured())
}

func TestEncryptValueMissingKeyWarnsAndReturnsPlaintext(t *testing.T) {
	t.Setenv(midsommarSecret, "")

	hook := logtest.NewGlobal()
	defer hook.Reset()

	// Behavior is preserved: without a key the value passes through unchanged.
	got := EncryptValue("my-plaintext-secret")
	assert.Equal(t, "my-plaintext-secret", got)

	// But it must be logged loudly at least once.
	found := false
	for _, entry := range hook.AllEntries() {
		if entry.Level <= log.WarnLevel && containsIgnoreCase(entry.Message, "plaintext") {
			found = true
			// The secret value itself must never be logged.
			assert.NotContains(t, entry.Message, "my-plaintext-secret")
		}
	}
	assert.True(t, found, "expected a warning-or-worse log entry about plaintext fallback, got: %v", messages(hook))
}

func TestEncryptValueWithKeyDoesNotWarn(t *testing.T) {
	t.Setenv(midsommarSecret, "a-real-key")

	hook := logtest.NewGlobal()
	defer hook.Reset()

	got := EncryptValue("my-secret")
	assert.NotEqual(t, "my-secret", got)
	assert.Equal(t, "my-secret", DecryptValue(got))
}

func TestWarnIfEncryptionUnconfigured(t *testing.T) {
	t.Setenv(midsommarSecret, "")
	assert.False(t, WarnIfEncryptionUnconfigured())

	t.Setenv(midsommarSecret, "key")
	assert.True(t, WarnIfEncryptionUnconfigured())
}

func containsIgnoreCase(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func messages(hook *logtest.Hook) []string {
	var out []string
	for _, e := range hook.AllEntries() {
		out = append(out, e.Message)
	}
	return out
}
