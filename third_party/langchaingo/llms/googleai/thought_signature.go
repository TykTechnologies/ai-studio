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

// EncodeSignatureID returns a tool call ID carrying sig. The random part keeps
// IDs unique within a turn.
func EncodeSignatureID(sig string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return signatureIDPrefix + hex.EncodeToString(b[:]) + "_" + base64.RawURLEncoding.EncodeToString([]byte(sig))
}

// DecodeSignatureID returns the thought signature a tool call ID carries, or
// ok false for any other ID.
func DecodeSignatureID(id string) (sig string, ok bool) {
	rest, found := strings.CutPrefix(id, signatureIDPrefix)
	if !found {
		return "", false
	}
	_, encoded, found := strings.Cut(rest, "_")
	if !found || encoded == "" {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) == 0 {
		return "", false
	}
	return string(raw), true
}
