package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A brand-new instance bootstraps OPENAI_KEY and ANTHROPIC_KEY with empty
// values, then seeds providers pointing at them and renders them as healthy.
// These pin the three cases the UI needs to tell apart.
func TestCredentialStatus(t *testing.T) {
	t.Run("no key at all", func(t *testing.T) {
		status, ref := credentialStatus("")
		assert.Equal(t, CredentialUnset, status)
		assert.Empty(t, ref)
	})

	t.Run("inline literal key", func(t *testing.T) {
		status, ref := credentialStatus("sk-abc123")
		assert.Equal(t, CredentialInline, status)
		assert.Empty(t, ref, "an inline key has no secret name to report")
	})

	t.Run("secret reference with no backing value is unresolved", func(t *testing.T) {
		// No DB ref is configured in this test, so the reference cannot resolve.
		// This is exactly the fresh-instance case: the secret exists but is empty.
		status, ref := credentialStatus("$SECRET/OPENAI_KEY")
		assert.Equal(t, CredentialUnresolved, status)
		assert.Equal(t, "OPENAI_KEY", ref, "must name the secret so the UI can say which one to fill in")
	})

	t.Run("malformed reference is unresolved rather than treated as inline", func(t *testing.T) {
		status, _ := credentialStatus("$SECRET/")
		assert.Equal(t, CredentialUnresolved, status)
	})

	t.Run("env reference is classified as a reference, not an inline key", func(t *testing.T) {
		status, ref := credentialStatus("$ENV/DEFINITELY_NOT_SET_ANYWHERE")
		assert.Equal(t, CredentialUnresolved, status)
		assert.Equal(t, "DEFINITELY_NOT_SET_ANYWHERE", ref)
	})
}
