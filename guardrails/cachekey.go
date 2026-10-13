package guardrails

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"hash"
)

// cacheKeySecret keys the digests guardrail caches are keyed on. It is drawn
// at start-up and never leaves the process.
var cacheKeySecret = func() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("guardrails: no randomness for the cache key: " + err.Error())
	}
	return b
}()

// NewCacheMAC returns an HMAC-SHA256 under the process's cache key. Caches
// whose keys are derived from configuration holding resolved credentials,
// or from text the guardrail screened, key on it rather than on a bare hash:
// a key that leaked would allow no offline guessing of a low-entropy value,
// and no one outside the process can craft a collision. A key made with it
// must never be logged.
func NewCacheMAC() hash.Hash {
	return hmac.New(sha256.New, cacheKeySecret)
}

// CacheDigest is the NewCacheMAC digest of b, as a string usable as a map
// key.
func CacheDigest(b []byte) string {
	mac := NewCacheMAC()
	mac.Write(b)
	return string(mac.Sum(nil))
}
