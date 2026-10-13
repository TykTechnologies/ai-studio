package nativeapi

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
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

	// hkdfInfoSecret is the info for keys derived from a hub-generated
	// signing secret.
	hkdfInfoSecret = "tyk-native-resource/secret/v1"
)

var (
	// ErrEmptyKey: the LLM has no signing secret or vendor credential to
	// derive a key from.
	// The proxy answers 501, never an unsigned ID.
	ErrEmptyKey = errors.New("nativeapi: empty signing key")
	// ErrMalformedID: the ID is not in the wrapped format or the vendor ID
	// fails the resource's charset.
	ErrMalformedID = errors.New("nativeapi: malformed resource id")
	// ErrKeyMismatch: well-formed, but signed with a key the codec does not
	// hold (a rotated key past its grace period, or another installation).
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

// Key is one signing key and its identifier.
type Key struct {
	key []byte
	kid string
}

// KID is the key's identifier, embedded in the IDs it signs.
func (k Key) KID() string { return k.kid }

func keyOf(key []byte) Key {
	sum := sha256.Sum256(key)
	return Key{key: key, kid: hex.EncodeToString(sum[:kidHexLen/2])}
}

func deriveKey(material, info string) (Key, error) {
	if material == "" {
		return Key{}, ErrEmptyKey
	}
	key, err := hkdf.Key(sha256.New, []byte(material), nil, info, sha256.Size)
	if err != nil {
		return Key{}, err
	}
	return keyOf(key), nil
}

// SecretKey derives a signing key from a hub-generated signing secret.
func SecretKey(secret string) (Key, error) { return deriveKey(secret, hkdfInfoSecret) }

// CredentialKey derives a signing key from an LLM's plaintext vendor
// credential: the key of LLMs without a signing secret, and of every ID
// minted before signing secrets existed.
func CredentialKey(credential string) (Key, error) {
	if credential == "" {
		return Key{}, ErrEmptyKey
	}
	key, err := models.DeriveCredentialSigningKey(credential)
	if err != nil {
		return Key{}, err
	}
	return keyOf(key), nil
}

// PreviousKeyForLLM returns llm's previous signing key, whether or not its
// grace period is over: a rotated secret, the vendor-credential key the
// backfill stored (ResourceSigningDerivedKeyPrefix), or, for the legacy
// sentinel, the key of the LLM's current credential. ok is false when the
// LLM has none.
func PreviousKeyForLLM(llm *models.LLM) (k Key, ok bool, err error) {
	prev := llm.ResourceSigningKeys().Previous
	switch {
	case prev == "":
		return Key{}, false, nil
	case prev == models.ResourceSigningLegacyKey:
		k, err = CredentialKey(llm.APIKey)
	case strings.HasPrefix(prev, models.ResourceSigningDerivedKeyPrefix):
		raw, derr := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(prev, models.ResourceSigningDerivedKeyPrefix))
		if derr != nil || len(raw) != sha256.Size {
			return Key{}, false, ErrEmptyKey
		}
		k = keyOf(raw)
	default:
		k, err = SecretKey(prev)
	}
	if err != nil {
		return Key{}, false, err
	}
	return k, true, nil
}

// Codec wraps resource IDs with one key and verifies them against that key
// and any older keys still in their grace period.
type Codec struct {
	key    []byte
	kid    string
	verify []Key
}

// NewCodec derives the signing key from the LLM's plaintext vendor
// credential.
func NewCodec(credential string) (*Codec, error) {
	k, err := CredentialKey(credential)
	if err != nil {
		return nil, err
	}
	return NewKeyringCodec(k), nil
}

// NewKeyringCodec signs with sign and also accepts IDs signed with any of
// accept (previous keys in their grace period).
func NewKeyringCodec(sign Key, accept ...Key) *Codec {
	c := &Codec{key: sign.key, kid: sign.kid, verify: []Key{sign}}
	for _, k := range accept {
		if len(k.key) > 0 {
			c.verify = append(c.verify, k)
		}
	}
	return c
}

// CodecForLLM builds the codec for llm's resource IDs at now. An LLM with a
// signing secret signs with it and still accepts its previous key (a rotated
// secret, or the stored vendor-credential key) until the grace period ends
// (PreviousKeyForLLM). An LLM without one, a standalone edge's, signs with
// the vendor-credential key.
func CodecForLLM(llm *models.LLM, now time.Time) (*Codec, error) {
	keys := llm.ResourceSigningKeys()
	if keys.Secret == "" {
		return NewCodec(llm.APIKey)
	}
	sign, err := SecretKey(keys.Secret)
	if err != nil {
		return nil, err
	}
	var accept []Key
	if keys.PreviousActive(now) {
		if prev, ok, perr := PreviousKeyForLLM(llm); perr == nil && ok {
			accept = append(accept, prev)
		}
	}
	return NewKeyringCodec(sign, accept...), nil
}

// KID is the key identifier embedded in IDs this codec mints.
func (c *Codec) KID() string { return c.kid }

func (c *Codec) mac(rt ResourceType, vendorID string, s Scope) []byte {
	return macWith(c.key, rt, vendorID, s)
}

func macWith(key []byte, rt ResourceType, vendorID string, s Scope) []byte {
	h := hmac.New(sha256.New, key)
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
	// MAC; it picks the key and lets logs tell a rotated key from a forgery.
	// An ID whose kid matches none of the codec's keys is never accepted.
	got, _ := hex.DecodeString(p.MAC) // validated by ParseID
	known := false
	for _, k := range c.verify {
		if k.kid != p.KID {
			continue
		}
		known = true
		if hmac.Equal(got, macWith(k.key, rt, p.VendorID, s)) {
			return p.VendorID, nil
		}
	}
	if !known {
		return "", ErrKeyMismatch
	}
	return "", ErrBadSignature
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
