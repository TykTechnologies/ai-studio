package auth

import "crypto/subtle"

// SecureTokenEquals compares two tokens in constant time to avoid timing
// side channels. Empty tokens never match — an unset credential must not
// authenticate anything.
func SecureTokenEquals(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
