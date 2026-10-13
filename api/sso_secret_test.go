package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The nonce endpoint's shared secret: an unset secret matches nothing, and
// only the exact secret matches.
func TestSSOSecretMatches(t *testing.T) {
	for _, presented := range []string{"", " ", "anything"} {
		assert.False(t, ssoSecretMatches("", presented), "unset secret, header %q", presented)
	}
	const secret = "s3cret-value"
	assert.True(t, ssoSecretMatches(secret, secret))
	for _, presented := range []string{"", "s3cret", "s3cret-value ", "S3CRET-VALUE", "s3cret-valuex", "Bearer s3cret-value"} {
		assert.False(t, ssoSecretMatches(secret, presented), "header %q", presented)
	}
}
