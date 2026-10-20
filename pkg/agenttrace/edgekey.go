package agenttrace

import (
	"crypto/hmac"
	"crypto/sha256"
)

// EdgeKeyExporter is implemented by the hub's Fingerprinter (the Enterprise
// key provider). The config snapshot builder sends this key to edges (D4):
// the derived HMAC key and its id, never the root key.
type EdgeKeyExporter interface {
	EdgeKey() (id byte, key []byte)
}

// EdgeKey returns the key the installed Fingerprinter hands to edges. ok is
// false when tracing is not running on this node or the Fingerprinter does
// not export one.
func EdgeKey() (id byte, key []byte, ok bool) {
	e, isExporter := CurrentFingerprinter().(EdgeKeyExporter)
	if !isExporter {
		return 0, nil, false
	}
	id, key = e.EdgeKey()
	if id == 0 || len(key) == 0 {
		return 0, nil, false
	}
	return id, key, true
}

// InstallEdgeKey installs, on an edge, the fingerprint key the hub sent in
// the config snapshot. The key is held in memory only. Id 0 or an empty key
// removes the Fingerprinter, so the edge computes no fingerprints.
func InstallEdgeKey(id byte, key []byte) {
	if id == 0 || len(key) == 0 {
		SetFingerprinter(nil)
		return
	}
	SetFingerprinter(hmacFingerprinter{id: id, key: append([]byte(nil), key...)})
}

// hmacFingerprinter is an edge's Fingerprinter: HMAC-SHA-256 under the key
// the hub derived. It prints no key material.
type hmacFingerprinter struct {
	id  byte
	key []byte
}

func (h hmacFingerprinter) KeyID() byte { return h.id }

func (h hmacFingerprinter) Fingerprint(canonical []byte) Fingerprint {
	m := hmac.New(sha256.New, h.key)
	m.Write(canonical)
	out := make([]byte, 1, 1+sha256.Size)
	out[0] = h.id
	return m.Sum(out)
}

// String keeps the key out of logs and %v formatting.
func (h hmacFingerprinter) String() string { return "agenttrace.Fingerprinter(edge key)" }

// GoString keeps the key out of %#v formatting.
func (h hmacFingerprinter) GoString() string { return h.String() }
