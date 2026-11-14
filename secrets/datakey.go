package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
)

// Data-key encryption, for values there are many of (App credentials).
//
// EncryptValue derives a fresh key for every value: the operator's
// TYK_AI_SECRET_KEY through scrypt (PBKDF2 in FIPS mode) with a random
// per-value salt, about 100 ms each, and decrypting pays the same unless
// that value's key is in a small cache. That is right for a handful of
// values (a secret, an LLM's API key) and wrong for tens of thousands: a
// migration, a list, or a hub validating keys would spend its time deriving
// keys.
//
// Here the key derivation runs once per process: TYK_AI_SECRET_KEY and a
// salt that belongs to the database (random, created once, stored in it,
// not secret) give one 256-bit data key, and each value is sealed under it
// with AES-256-GCM, a random 96-bit nonce, and the identity of the row it
// belongs to as additional authenticated data, so a ciphertext copied to
// another row does not open. Sealing or opening a value then costs
// microseconds, at any number of values.
//
// The stored form is versioned and distinct from EncryptValue's "$ENC/":
//
//	"$DK1/s:"        + base64url(nonce || ciphertext)   data key derived with scrypt
//	"$DK1/p<iter>:"  + base64url(nonce || ciphertext)   data key derived with PBKDF2-HMAC-SHA256
//
// The tag after the prefix names the key derivation, as v2 and v3 do for
// "$ENC/" values: FIPS-mode processes write the PBKDF2 form, every build
// reads both. Only approved primitives are used in FIPS mode.
const (
	// DataKeyPrefix starts every value sealed under a data key.
	DataKeyPrefix = "$DK1/"

	dataKeyScryptTag = "s"
	dataKeyPBKDF2Tag = "p"

	// DataKeySaltLength is the size of a database's salt, in bytes.
	DataKeySaltLength = 32
)

// ErrDataKeyUnavailable: there is no encryption key, or no salt, to derive
// the data key from.
var ErrDataKeyUnavailable = errors.New("no data key: TYK_AI_SECRET_KEY or the database's salt is missing")

// keyDerivations counts the key derivations this process has computed
// (cache hits are not derivations). Tests assert that work on N values
// derives a constant number of keys, not N.
var keyDerivations atomic.Uint64

// KeyDerivations returns how many key derivations (scrypt or PBKDF2, each
// about 100 ms at production cost) this process has computed.
func KeyDerivations() uint64 { return keyDerivations.Load() }

// dataKeyTag is the derivation tag this process writes.
func dataKeyTag() string {
	if fipsEnabled() {
		return dataKeyPBKDF2Tag + strconv.Itoa(v3Iterations())
	}
	return dataKeyScryptTag
}

// DataKeyCurrentPrefix is what every value this process seals starts with.
// A stored value that does not start with it is in an older or another
// form and is due to be sealed again.
func DataKeyCurrentPrefix() string { return DataKeyPrefix + dataKeyTag() + ":" }

// IsDataKeyValue reports whether stored is a value sealed under a data key.
func IsDataKeyValue(stored string) bool { return strings.HasPrefix(stored, DataKeyPrefix) }

// dataKey derives (once; the derivation cache keeps it) the data key for
// salt under the derivation tag names.
func dataKey(tag string, salt []byte) ([]byte, error) {
	key := encryptionKey()
	if key == "" || len(salt) == 0 {
		return nil, ErrDataKeyUnavailable
	}
	// The salt is bound to its purpose, so the data key is never a key
	// EncryptValue could derive for one of its own values.
	bound := append([]byte("tyk-ai-studio/data-key/v1\x00"), salt...)
	switch {
	case tag == dataKeyScryptTag:
		return deriveKeyScrypt(key, bound)
	case strings.HasPrefix(tag, dataKeyPBKDF2Tag):
		iter, err := strconv.Atoi(strings.TrimPrefix(tag, dataKeyPBKDF2Tag))
		if err != nil || iter < 1 || iter > pbkdf2MaxIterations {
			return nil, fmt.Errorf("malformed data key value header %q", tag)
		}
		return deriveKeyPBKDF2(key, bound, iter)
	}
	return nil, fmt.Errorf("unknown data key derivation %q", tag)
}

func dataKeyGCM(tag string, salt []byte) (cipher.AEAD, error) {
	key, err := dataKey(tag, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	// A random 96-bit nonce per value, prepended to the ciphertext.
	return cipher.NewGCMWithRandomNonce(block)
}

// SealWithDataKey encrypts plaintext under the data key of salt, bound to
// aad (the identity of the row the value belongs to). Unlike EncryptValue it
// does not fall back to plaintext: without an encryption key or a salt it
// returns ErrDataKeyUnavailable, and the caller decides.
func SealWithDataKey(salt []byte, plaintext, aad string) (string, error) {
	tag := dataKeyTag()
	gcm, err := dataKeyGCM(tag, salt)
	if err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nil, []byte(plaintext), []byte(aad))
	return DataKeyPrefix + tag + ":" + base64.URLEncoding.EncodeToString(sealed), nil
}

// OpenWithDataKey decrypts a value SealWithDataKey wrote for the same salt
// and aad. It fails for another key, another salt, another row's aad, or a
// value that was altered.
func OpenWithDataKey(salt []byte, stored, aad string) (string, error) {
	if !IsDataKeyValue(stored) {
		return "", errors.New("not a data key value")
	}
	tag, encoded, ok := strings.Cut(strings.TrimPrefix(stored, DataKeyPrefix), ":")
	if !ok {
		return "", errors.New("malformed data key value")
	}
	gcm, err := dataKeyGCM(tag, salt)
	if err != nil {
		return "", err
	}
	sealed, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to base64-decode ciphertext: %w", err)
	}
	if len(sealed) < gcmNonceSize+gcmTagSize {
		return "", fmt.Errorf("ciphertext too short: %d bytes", len(sealed))
	}
	plaintext, err := gcm.Open(nil, nil, sealed, []byte(aad))
	if err != nil {
		return "", fmt.Errorf("failed to decrypt value (wrong key, salt or row, or tampered ciphertext): %w", err)
	}
	return string(plaintext), nil
}
