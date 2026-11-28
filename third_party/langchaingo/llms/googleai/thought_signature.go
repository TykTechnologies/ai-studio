package googleai

// Tyk: this file is not in upstream langchaingo.
//
// Gemini 3 models attach a thoughtSignature to each functionCall part they
// return and refuse a later turn whose functionCall parts come back without
// it. The generative-ai-go client has no field for the signature (its Part and
// FunctionCall types drop it when decoding), so the proxy carries it past the
// client on the loopback hop: the HTTP transport moves a part's signature into
// the call's args under ThoughtSignatureArg on the way in, and back out on the
// way to the API. In between, this package moves it from the args into the
// tool call's ID, which OpenAI-shaped clients echo back unchanged, and from the
// ID into the args when the call is sent again.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// ThoughtSignatureArg is the args key that holds a functionCall part's
// thoughtSignature between the HTTP transport and this package. It never
// reaches a caller or the API.
const ThoughtSignatureArg = "__tyk_thought_signature"

// signatureIDPrefix marks a tool call ID that carries a thought signature:
// call_gs1_{8 hex}_{base64url(signature)}.
const signatureIDPrefix = "call_gs1_"

// MaxThoughtSignatureBytes caps a signature read back from a tool call ID,
// far above what Gemini sends. A larger one is treated as absent.
const MaxThoughtSignatureBytes = 64 << 10

// EncodeSignatureID returns a tool call ID carrying sig. The random part keeps
// IDs unique within a turn.
func EncodeSignatureID(sig string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return signatureIDPrefix + hex.EncodeToString(b[:]) + "_" + base64.RawURLEncoding.EncodeToString([]byte(sig))
}

// DecodeSignatureID returns the thought signature a tool call ID carries, or
// ok false for any other ID: one without the prefix, a malformed one (the
// random part must be 8 hex digits, the rest strict unpadded base64url) or one
// whose signature is empty or over MaxThoughtSignatureBytes.
func DecodeSignatureID(id string) (sig string, ok bool) {
	rest, found := strings.CutPrefix(id, signatureIDPrefix)
	if !found {
		return "", false
	}
	random, encoded, found := strings.Cut(rest, "_")
	if !found || encoded == "" || len(random) != 8 {
		return "", false
	}
	if _, err := hex.DecodeString(random); err != nil {
		return "", false
	}
	if base64.RawURLEncoding.DecodedLen(len(encoded)) > MaxThoughtSignatureBytes {
		return "", false
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(raw) == 0 {
		return "", false
	}
	return string(raw), true
}

// ShortSignatureID is a signed tool call ID without its signature
// (call_gs1_{8 hex}), for a vendor other than Gemini, whose IDs are limited
// in length (OpenAI's to 40 characters). The same ID always shortens to the
// same string, so a tool call and its result still pair up. Any other ID is
// returned as it is.
func ShortSignatureID(id string) string {
	rest, found := strings.CutPrefix(id, signatureIDPrefix)
	if !found {
		return id
	}
	random, _, found := strings.Cut(rest, "_")
	if !found || len(random) != 8 {
		return id
	}
	return signatureIDPrefix + random
}

// usableSignature reports whether sig may be sent as a thought signature: a
// non-empty string of at most MaxThoughtSignatureBytes.
func usableSignature(v any) (string, bool) {
	sig, ok := v.(string)
	return sig, ok && sig != "" && len(sig) <= MaxThoughtSignatureBytes
}
