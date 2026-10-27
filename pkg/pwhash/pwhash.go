// Package pwhash hashes and verifies passwords and long-lived secrets (user
// passwords, OAuth client secrets, microgateway credential secrets).
//
// Two formats are read by every build: bcrypt ("$2a$...", "$2b$..."), written
// by standard builds, and PBKDF2-HMAC-SHA256, written when
// crypto/fips140.Enabled() (bcrypt is not a FIPS 140-3 approved algorithm):
//
//	$pbkdf2-sha256$<iterations>$<base64 salt>$<base64 key>
//
// with a 16-byte random salt, a 32-byte key and unpadded standard base64.
//
// A database therefore moves between standard and FIPS builds in both
// directions. A FIPS process that verifies a bcrypt hash reports it through
// NeedsUpgrade, so the caller can store a PBKDF2 hash in its place.
//
// This is the only package that imports golang.org/x/crypto/bcrypt.
package pwhash

import (
	"crypto/fips140"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

const (
	// PBKDF2Prefix starts every PBKDF2 hash.
	PBKDF2Prefix = "$pbkdf2-sha256$"

	// Iterations is the PBKDF2 work factor for new hashes (OWASP 2023 for
	// PBKDF2-HMAC-SHA256). Verification uses the count stored in the hash.
	Iterations = 600_000

	// testIterations is used instead inside test binaries, which hash many
	// passwords; the count is stored, so such hashes verify anywhere.
	testIterations = 1_000

	// maxIterations bounds the count read from a stored hash, so a tampered
	// row cannot make one verification run for minutes.
	maxIterations = 10_000_000

	saltBytes = 16
	keyBytes  = 32
)

var b64 = base64.RawStdEncoding

// fipsEnabled is crypto/fips140.Enabled, replaceable by ForceFIPSModeForTest.
var fipsEnabled = fips140.Enabled

// ForceFIPSModeForTest makes Hash and NeedsUpgrade behave as in FIPS 140-3
// mode until the test ends, so standard test runs cover the FIPS writer and
// the upgrade path. Not safe for parallel tests.
func ForceFIPSModeForTest(tb testing.TB) {
	tb.Helper()
	prev := fipsEnabled
	fipsEnabled = func() bool { return true }
	tb.Cleanup(func() { fipsEnabled = prev })
}

// bcryptCost is the bcrypt cost of new standard-build hashes:
// bcrypt.DefaultCost, lowered only in test binaries (UseMinBcryptCostForTests).
var bcryptCost atomic.Int32

func init() { bcryptCost.Store(int32(bcrypt.DefaultCost)) }

// BcryptCost returns the bcrypt cost Hash uses outside FIPS mode.
func BcryptCost() int { return int(bcryptCost.Load()) }

// UseMinBcryptCostForTests lowers the bcrypt cost to bcrypt.MinCost. It
// panics outside a test binary, so production can never lower the cost.
// Verification reads the cost from the hash, so stored hashes still verify.
func UseMinBcryptCostForTests() {
	if !testing.Testing() {
		panic("pwhash.UseMinBcryptCostForTests called outside a test binary")
	}
	bcryptCost.Store(int32(bcrypt.MinCost))
}

// Hash hashes secret with PBKDF2-HMAC-SHA256 when FIPS 140-3 mode is enabled,
// and with bcrypt (BcryptCost) otherwise.
func Hash(secret string) (string, error) {
	if fipsEnabled() {
		return HashPBKDF2(secret)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(secret), BcryptCost())
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// HashPBKDF2 hashes secret with PBKDF2-HMAC-SHA256 regardless of mode.
func HashPBKDF2(secret string) (string, error) {
	iter := currentIterations()
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, secret, salt, iter, keyBytes)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d$%s$%s", PBKDF2Prefix, iter, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// currentIterations is the count new hashes use: Iterations, or
// testIterations inside a test binary.
func currentIterations() int {
	if testing.Testing() {
		return testIterations
	}
	return Iterations
}

// IsPBKDF2 reports whether hash is in the PBKDF2 format.
func IsPBKDF2(hash string) bool { return strings.HasPrefix(hash, PBKDF2Prefix) }

// Verify reports whether secret matches hash, which may be in either format.
// A malformed hash never matches.
func Verify(secret, hash string) bool {
	if IsPBKDF2(hash) {
		ok, err := verifyPBKDF2(secret, hash)
		return err == nil && ok
	}
	if !strings.HasPrefix(hash, "$2") {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}

// NeedsUpgrade reports whether a hash that just verified should be replaced
// with a new one from Hash: true in FIPS mode for anything but PBKDF2, and
// for a PBKDF2 hash with fewer iterations than new hashes get (one written by
// a test binary, say).
func NeedsUpgrade(hash string) bool {
	if !fipsEnabled() {
		return false
	}
	if !IsPBKDF2(hash) {
		return true
	}
	iter, _, _, err := parsePBKDF2(hash)
	return err == nil && iter < currentIterations()
}

var errMalformed = errors.New("pwhash: malformed PBKDF2 hash")

// parsePBKDF2 splits a PBKDF2 hash, refusing parameters this package never
// writes: a key other than 32 bytes, a salt under 16 bytes, or an iteration
// count out of range. A short key would let a corrupted or crafted row match
// random passwords.
func parsePBKDF2(hash string) (iter int, salt, key []byte, err error) {
	parts := strings.Split(strings.TrimPrefix(hash, PBKDF2Prefix), "$")
	if len(parts) != 3 {
		return 0, nil, nil, errMalformed
	}
	iter, err = strconv.Atoi(parts[0])
	if err != nil || iter < 1 || iter > maxIterations {
		return 0, nil, nil, errMalformed
	}
	salt, err = b64.DecodeString(parts[1])
	if err != nil || len(salt) < saltBytes {
		return 0, nil, nil, errMalformed
	}
	key, err = b64.DecodeString(parts[2])
	if err != nil || len(key) != keyBytes {
		return 0, nil, nil, errMalformed
	}
	return iter, salt, key, nil
}

func verifyPBKDF2(secret, hash string) (bool, error) {
	iter, salt, want, err := parsePBKDF2(hash)
	if err != nil {
		return false, err
	}
	got, err := pbkdf2.Key(sha256.New, secret, salt, iter, keyBytes)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
