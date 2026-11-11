package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
)

// A credential that is encrypted at rest cannot be found by its value:
// EncryptValue's output is different every time. It is found by a hash of
// the value, stored next to it.
//
// LookupHash is that hash. With an encryption key (TYK_AI_SECRET_KEY) it is
// keyed: pkg/credhash's construction (HKDF-SHA-256, then HMAC-SHA-256) under
// the encryption key, so a copy of the database yields nothing that can be
// tested against a guessed credential without the key. Without a key, or
// with one shorter than the 14 bytes that construction needs, it is a plain
// SHA-256, as the value itself is then stored in plaintext beside it.
//
// The two forms carry different prefixes, so a hash says which it is, and
// rows hashed before a key was set are found and rehashed
// (LookupHashIsCurrent). The hash depends on the key: with another key
// nothing is found, as nothing decrypts.
const (
	// LookupHashKeyedPrefix starts a hash keyed with the encryption key;
	// what follows is a pkg/credhash hash.
	LookupHashKeyedPrefix = "k:"
	// LookupHashPlainPrefix starts an unkeyed SHA-256, in hexadecimal.
	LookupHashPlainPrefix = "u:"
	// LookupHashMaxLen bounds every form (a column size).
	LookupHashMaxLen = 80
)

var (
	lookupHashersMu sync.Mutex
	// lookupHashers caches the hasher of each key seen (one, outside tests).
	lookupHashers = map[string]*credhash.Hasher{}
	shortKeyOnce  sync.Once
)

// lookupHasher returns the keyed hasher for the configured encryption key,
// or nil when there is none or it is too short.
func lookupHasher() *credhash.Hasher {
	key := encryptionKey()
	if key == "" {
		return nil
	}
	lookupHashersMu.Lock()
	defer lookupHashersMu.Unlock()
	if h, ok := lookupHashers[key]; ok {
		return h
	}
	h, err := credhash.New(key)
	if err != nil {
		shortKeyOnce.Do(func() {
			log.Errorf("SECURITY WARNING: %s is shorter than 14 bytes: App credentials are found by an unkeyed hash. Use a longer key.", midsommarSecret)
		})
		h = nil
	}
	lookupHashers[key] = h
	return h
}

// LookupHash returns the hash value is found by.
func LookupHash(value string) string {
	if h := lookupHasher(); h != nil {
		return LookupHashKeyedPrefix + h.Hash(value)
	}
	sum := sha256.Sum256([]byte(value))
	return LookupHashPlainPrefix + hex.EncodeToString(sum[:])
}

// LookupHashPrefix is the prefix of the hashes this process writes.
func LookupHashPrefix() string {
	if lookupHasher() != nil {
		return LookupHashKeyedPrefix
	}
	return LookupHashPlainPrefix
}
