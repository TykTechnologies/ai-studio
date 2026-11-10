// Package credhash computes the keyed hash under which a credential (an App
// key, a standalone microgateway API token) is cached, stored and looked up
// in place of the credential itself:
//
//	k    = HKDF-SHA256(encryption key, info "credential-hash-v1")
//	hash = "v1:" + hex(HMAC-SHA256(k, credential))
//
// The encryption key is the hub's MICROGATEWAY_ENCRYPTION_KEY, which is each
// edge's ENCRYPTION_KEY, so the hub and its edges compute the same hash for
// the same credential (a standalone microgateway uses its own
// ENCRYPTION_KEY). This is the construction grpc/snapshot_secret_digests.go
// uses for snapshot checksums, under a label of its own.
//
// Credentials are 256-bit random values, so a fast keyed hash is enough: it
// allows an indexed lookup, a database holding only hashes yields nothing
// usable without the key, and the hash cannot be reversed with it. It is not
// a password hash: do not use it for secrets a person chose.
//
// Only FIPS 140-3 approved primitives from the standard library are used;
// the package runs under GODEBUG=fips140=only. See features/ActiveActive.md
// (Phase 4).
package credhash

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
	"sync"
)

// label is the HKDF info string. It is part of the derivation, so a hash
// made for another purpose from the same encryption key never equals a
// credential hash. A change of construction takes a new label and a new
// Prefix.
const label = "credential-hash-v1"

// Prefix starts every hash and names the construction that made it.
const Prefix = "v1:"

// Len is the length of every hash: Prefix and 64 hexadecimal characters.
const Len = len(Prefix) + 2*sha256.Size

// minKeyLen is the shortest encryption key accepted: 112 bits, the least the
// Go Cryptographic Module allows for an HMAC or HKDF key in FIPS mode.
const minKeyLen = 14

// ErrKeyTooShort is returned by New for an encryption key under 14 bytes.
var ErrKeyTooShort = errors.New("credhash: encryption key must be at least 14 bytes")

// Hasher computes credential hashes under one encryption key. It is safe for
// concurrent use.
type Hasher struct {
	// macs holds HMACs keyed with the derived key (never the encryption key
	// itself). Hash runs on every gateway request, and keying an HMAC costs
	// more than the hash of a credential.
	macs sync.Pool
}

// New returns the Hasher for encryptionKey (MICROGATEWAY_ENCRYPTION_KEY on
// the hub, ENCRYPTION_KEY on a microgateway).
func New(encryptionKey string) (*Hasher, error) {
	if len(encryptionKey) < minKeyLen {
		return nil, ErrKeyTooShort
	}
	k, err := hkdf.Key(sha256.New, []byte(encryptionKey), nil, label, sha256.Size)
	if err != nil {
		return nil, fmt.Errorf("credhash: derive key: %w", err)
	}
	h := &Hasher{}
	h.macs.New = func() any { return hmac.New(sha256.New, k) }
	return h, nil
}

// Hash returns the hash of credential: Prefix followed by 64 lower-case
// hexadecimal characters, Len in all.
func (h *Hasher) Hash(credential string) string {
	mac := h.macs.Get().(hash.Hash)
	mac.Reset()
	io.WriteString(mac, credential)
	var sum [sha256.Size]byte
	var out [Len]byte
	copy(out[:], Prefix)
	hex.Encode(out[len(Prefix):], mac.Sum(sum[:0]))
	h.macs.Put(mac)
	return string(out[:])
}

// IsHash reports whether s has the form of a hash made by this package's
// current construction. It says nothing about which key made it.
func IsHash(s string) bool {
	if len(s) != Len || !strings.HasPrefix(s, Prefix) {
		return false
	}
	for _, c := range []byte(s[len(Prefix):]) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
