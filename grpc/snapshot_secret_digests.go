package grpc

import (
	"bytes"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// Secrets go to edges encrypted with AES-GCM under a random nonce, so the
// same secret encrypts differently every time a snapshot is built. The sync
// checksum (ComputeSnapshotChecksum) must still be stable while the
// configuration is: otherwise edges would never show as in sync. So the
// checksum is computed with every ciphertext the snapshot carries replaced by
// a keyed digest of its plaintext:
//
//	k_checksum = HKDF-SHA256(MICROGATEWAY_ENCRYPTION_KEY, info snapshotChecksumInfo)
//	digest     = hex(HMAC-SHA256(k_checksum, plaintext))
//
// Identical configuration gives an identical checksum on every hub replica,
// a changed secret changes it, and the digest never leaves the hub (only the
// SHA-256 over the whole snapshot does). Edges never recompute the checksum
// (they report the hub's), so this needs nothing from them.
//
// encryptForMicrogateway records each ciphertext it returns with its digest
// here; the checksum step swaps in the digests found in its snapshot and
// forgets them. Entries of a snapshot that was never checksummed (a build
// that failed) are dropped after snapshotDigestTTL.

const snapshotChecksumInfo = "midsommar:microgateway:snapshot-checksum:v1"

const snapshotDigestTTL = 15 * time.Minute

type snapshotDigest struct {
	digest  string
	created time.Time
}

// snapshotDigests maps a ciphertext to its plaintext's keyed digest.
type snapshotDigests struct {
	m sync.Map // ciphertext -> snapshotDigest
}

// digestFor returns the keyed digest of plaintext under encryptionKey.
func digestFor(encryptionKey, plaintext string) (string, error) {
	k, err := hkdf.Key(sha256.New, []byte(encryptionKey), nil, snapshotChecksumInfo, sha256.Size)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, k)
	mac.Write([]byte(plaintext))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (d *snapshotDigests) record(ciphertext, digest string) {
	d.m.Store(ciphertext, snapshotDigest{digest: digest, created: time.Now()})
}

// normalize replaces, in a snapshot's deterministic encoding, every recorded
// ciphertext it contains with that ciphertext's digest, forgets those
// entries, and drops expired ones. A ciphertext is random base64 of at least
// 40 characters, so it cannot occur by chance; it appears verbatim whether it
// is a field of its own or a value inside a JSON config string.
func (d *snapshotDigests) normalize(encoded []byte, now time.Time) []byte {
	d.m.Range(func(k, v any) bool {
		ct := k.(string)
		entry := v.(snapshotDigest)
		if bytes.Contains(encoded, []byte(ct)) {
			encoded = bytes.ReplaceAll(encoded, []byte(ct), []byte("hmac:"+entry.digest))
			d.m.Delete(ct)
		} else if now.Sub(entry.created) > snapshotDigestTTL {
			d.m.Delete(ct)
		}
		return true
	})
	return encoded
}
