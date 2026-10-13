package api

import (
	"crypto/sha256"
	"crypto/subtle"
)

// ssoSecretMatches reports whether presented, the Authorization header of a
// call to the SSO nonce endpoint, is the identity broker's shared secret
// (TYK_AI_SECRET_KEY). An unset secret matches nothing: the endpoint mints
// login nonces.
// Both sides are hashed first so the comparison takes the same time
// whatever the header's length or content.
func ssoSecretMatches(secret, presented string) bool {
	if secret == "" {
		return false
	}
	want := sha256.Sum256([]byte(secret))
	got := sha256.Sum256([]byte(presented))
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}
