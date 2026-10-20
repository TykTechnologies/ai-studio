package agenttrace

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sync/atomic"
)

// Fingerprint is a key id byte followed by HMAC-SHA-256 of canonical bytes
// (D4). The key id lets a rotated key coexist with the previous one. A nil
// Fingerprint means "not available".
type Fingerprint []byte

// KeyID returns the key id byte, or 0 for an empty fingerprint.
func (f Fingerprint) KeyID() byte {
	if len(f) == 0 {
		return 0
	}
	return f[0]
}

// String renders the fingerprint as hex, key id first.
func (f Fingerprint) String() string { return hex.EncodeToString(f) }

// Fingerprinter computes fingerprints with the install-wide HMAC key (D4).
// The Enterprise key provider implements it; the edge receives only the
// derived HMAC key through config sync.
//
// Callers canonicalise first: JSON is RFC 8785 (JCS) bytes, text is
// trimmed, whitespace-collapsed NFC (§4.4). Implementations must be safe for
// concurrent use and use stdlib crypto only (FIPS rule).
type Fingerprinter interface {
	// KeyID is the id byte of the current key.
	KeyID() byte
	// Fingerprint returns KeyID() followed by HMAC-SHA-256(key, canonical).
	Fingerprint(canonical []byte) Fingerprint
}

type fingerprinterHolder struct{ f Fingerprinter }

var currentFingerprinter atomic.Pointer[fingerprinterHolder]

// SetFingerprinter installs the process-wide Fingerprinter (the key
// provider calls it at start-up and on rotation). Nil removes it.
func SetFingerprinter(f Fingerprinter) {
	if f == nil {
		currentFingerprinter.Store(nil)
		return
	}
	currentFingerprinter.Store(&fingerprinterHolder{f: f})
}

// CurrentFingerprinter returns the installed Fingerprinter, or nil. Gateway
// code computes no fingerprints without one.
func CurrentFingerprinter() Fingerprinter {
	if h := currentFingerprinter.Load(); h != nil {
		return h.f
	}
	return nil
}

// StaticFingerprinter is a deterministic Fingerprinter over a fixed key. It
// is for tests and fixtures (goldens need stable fingerprints); production
// keys come from the key provider.
type StaticFingerprinter struct {
	ID  byte
	Key []byte
}

// KeyID implements Fingerprinter.
func (s StaticFingerprinter) KeyID() byte { return s.ID }

// Fingerprint implements Fingerprinter.
func (s StaticFingerprinter) Fingerprint(canonical []byte) Fingerprint {
	m := hmac.New(sha256.New, s.Key)
	m.Write(canonical)
	out := make([]byte, 1, 1+sha256.Size)
	out[0] = s.ID
	return m.Sum(out)
}
