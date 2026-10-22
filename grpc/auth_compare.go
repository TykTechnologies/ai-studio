package grpc

import (
	"crypto/sha256"
	"crypto/subtle"
)

// bearerMatches reports whether an authorization header value is
// "Bearer <token>", in constant time. Both sides are hashed first, so the
// comparison does not even leak the token's length.
func bearerMatches(header, token string) bool {
	got := sha256.Sum256([]byte(header))
	want := sha256.Sum256([]byte("Bearer " + token))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}
