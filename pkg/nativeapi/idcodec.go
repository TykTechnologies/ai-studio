package nativeapi

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
)

// ID format: <vendor_id>.t1k<kid>.<mac>
//
//	t1   format version 1
//	kid  8 lowercase hex chars: the first 4 bytes of SHA-256(key)
//	mac  32 lowercase hex chars: HMAC-SHA256(key, type|vendor_id|llm_id|app_id)
//	     truncated to 16 bytes, each field length-prefixed
//
// The suffix is all lowercase hex after ".t1k", so it never contains ".t1k"
// itself and splitting on the last ".t1k" is unambiguous whatever the vendor
// ID contains.
const (
	idMarker  = ".t1k"
	kidHexLen = 8
	macHexLen = 32
	macBytes  = macHexLen / 2

	// hkdfInfo domain-separates the derived key from every other use of the
	// vendor credential.
	hkdfInfo = "tyk-native-resource/v1"
)

var (
	// ErrEmptyKey: the LLM has no vendor credential to derive a key from.
	// The proxy answers 501, never an unsigned ID.
	ErrEmptyKey = errors.New("nativeapi: empty signing key")
	// ErrMalformedID: the ID is not in the wrapped format or the vendor ID
	// fails the resource's charset.
	ErrMalformedID = errors.New("nativeapi: malformed resource id")
	// ErrKeyMismatch: well-formed, but signed with a different key (rotated
	// vendor credential, or another installation).
	ErrKeyMismatch = errors.New("nativeapi: resource id signed with another key")
	// ErrBadSignature: the MAC does not match this type, LLM and App.
	ErrBadSignature = errors.New("nativeapi: resource id signature mismatch")
)

// Scope is what an ID is bound to besides its type. IDs are the hub's IDs;
// edges see the same values.
type Scope struct {
	LLMID uint
	AppID uint
}

// Codec wraps and verifies resource IDs for one LLM's vendor credential.
type Codec struct {
	key []byte
	kid string
}

// NewCodec derives the signing key from the LLM's plaintext vendor
// credential.
func NewCodec(credential string) (*Codec, error) {
	if credential == "" {
		return nil, ErrEmptyKey
	}
	key, err := hkdf.Key(sha256.New, []byte(credential), nil, hkdfInfo, sha256.Size)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(key)
	return &Codec{key: key, kid: hex.EncodeToString(sum[:kidHexLen/2])}, nil
}

// KID is the key identifier embedded in IDs this codec mints.
func (c *Codec) KID() string { return c.kid }

func (c *Codec) mac(rt ResourceType, vendorID string, s Scope) []byte {
	h := hmac.New(sha256.New, c.key)
	var n [4]byte
	for _, f := range []string{
		rt.Name,
		vendorID,
		strconv.FormatUint(uint64(s.LLMID), 10),
		strconv.FormatUint(uint64(s.AppID), 10),
	} {
		binary.BigEndian.PutUint32(n[:], uint32(len(f)))
		h.Write(n[:])
		h.Write([]byte(f))
	}
	return h.Sum(nil)[:macBytes]
}

// Wrap signs vendorID for rt and scope.
func (c *Codec) Wrap(rt ResourceType, vendorID string, s Scope) (string, error) {
	if c == nil || len(c.key) == 0 {
		return "", ErrEmptyKey
	}
	if rt.Name == "" || !rt.ValidVendorID(vendorID) {
		return "", ErrMalformedID
	}
	return vendorID + idMarker + c.kid + "." + hex.EncodeToString(c.mac(rt, vendorID, s)), nil
}

// ParsedID is the structure of a wrapped ID. Nothing in it is verified
// except its format.
type ParsedID struct {
	VendorID string
	KID      string
	MAC      string
}

// ParseID splits a wrapped ID without verifying it. Its only legitimate use
// is the creator node's key-rotation fallback (a ledger row matching
// llm, type, vendor ID and app authorises a MAC failure); otherwise use
// Unwrap.
func ParseID(rt ResourceType, wrapped string) (ParsedID, error) {
	i := strings.LastIndex(wrapped, idMarker)
	if i <= 0 {
		return ParsedID{}, ErrMalformedID
	}
	vendorID, suffix := wrapped[:i], wrapped[i+len(idMarker):]
	if len(suffix) != kidHexLen+1+macHexLen || suffix[kidHexLen] != '.' {
		return ParsedID{}, ErrMalformedID
	}
	kid, mac := suffix[:kidHexLen], suffix[kidHexLen+1:]
	if !isLowerHex(kid) || !isLowerHex(mac) {
		return ParsedID{}, ErrMalformedID
	}
	if !rt.ValidVendorID(vendorID) {
		return ParsedID{}, ErrMalformedID
	}
	return ParsedID{VendorID: vendorID, KID: kid, MAC: mac}, nil
}

// Unwrap verifies wrapped for rt and scope and returns the raw vendor ID.
// Callers must map every error to the same not-found response.
func (c *Codec) Unwrap(rt ResourceType, wrapped string, s Scope) (string, error) {
	if c == nil || len(c.key) == 0 {
		return "", ErrEmptyKey
	}
	p, err := ParseID(rt, wrapped)
	if err != nil {
		return "", err
	}
	// The kid is public (derived from the key hash) and not covered by the
	// MAC; it only lets logs tell a rotated key from a forgery, and an ID
	// whose kid is not ours is never accepted.
	if p.KID != c.kid {
		return "", ErrKeyMismatch
	}
	got, _ := hex.DecodeString(p.MAC) // validated by ParseID
	if !hmac.Equal(got, c.mac(rt, p.VendorID, s)) {
		return "", ErrBadSignature
	}
	return p.VendorID, nil
}

func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
