package extract

import (
	"bytes"
	"strings"
	"unicode"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"golang.org/x/text/unicode/norm"
)

// Domain tags keep the three fingerprint kinds apart under the one HMAC key:
// an argument object and a result text with the same bytes never collide.
const (
	domainArgs   = "tyk-agenttrace/args/v1\x00"
	domainResult = "tyk-agenttrace/result/v1\x00"
	domainText   = "tyk-agenttrace/text/v1\x00"
)

// ArgsFingerprint is the args_fp of a tool call's arguments (R1, R2). Both
// sides of the R2 match call it: the LLM response (arguments as the model
// wrote them) and the executed tool call (MCP tools/call params.arguments,
// REST tool payload). args is the JSON arguments value; a JSON string that
// itself holds JSON (OpenAI's function.arguments) is unwrapped first.
//
// Objects are flattened the way Studio's OpenAPI-to-MCP shim flattens them
// ({"body":{"name":"x"}} becomes {"body_name":"x"}, empty objects drop out),
// so the nested REST shape and the flat MCP shape fingerprint equal. It
// returns nil, with no error, when fp is nil.
func ArgsFingerprint(fp agenttrace.Fingerprinter, args []byte) (agenttrace.Fingerprint, error) {
	if fp == nil {
		return nil, nil
	}
	canon, err := CanonicalArgs(args)
	if err != nil {
		return nil, err
	}
	return fp.Fingerprint(append([]byte(domainArgs), canon...)), nil
}

// CanonicalArgs returns the canonical bytes ArgsFingerprint signs: JCS of the
// flattened arguments. Arguments that are not a JSON object (a custom tool's
// free-form input) are canonicalised as they are.
func CanonicalArgs(args []byte) ([]byte, error) {
	args = bytes.TrimSpace(args)
	if len(args) == 0 {
		args = []byte("{}")
	}
	v, err := parse(args)
	if err != nil {
		return nil, err
	}
	if s, ok := v.(string); ok {
		// A JSON string holding a JSON object or array is the arguments
		// encoded once more (OpenAI function.arguments).
		if inner := strings.TrimSpace(s); strings.HasPrefix(inner, "{") || strings.HasPrefix(inner, "[") {
			iv, err := parse([]byte(inner))
			if err != nil {
				return nil, err
			}
			v = iv
		}
	}
	if obj, ok := v.(object); ok {
		flat := object{}
		flatten(&flat, "", obj)
		v = flat
	}
	var buf bytes.Buffer
	if err := write(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// flatten appends obj's members to out, nested objects as prefix_key.
func flatten(out *object, prefix string, obj object) {
	for _, m := range obj {
		key := m.key
		if prefix != "" {
			key = prefix + "_" + m.key
		}
		if sub, ok := m.val.(object); ok {
			flatten(out, key, sub)
			continue
		}
		*out = append(*out, member{key: key, val: m.val})
	}
}

// ResultFingerprint is the result_fp of a tool result: its text pieces in
// order (a tool_result's text blocks, an MCP result's text content items),
// joined and normalised as NormalizeText does, so splitting the same text
// into blocks differently, or re-wrapping it, does not change it. Nil when fp
// is nil or there is no text.
func ResultFingerprint(fp agenttrace.Fingerprinter, texts ...string) agenttrace.Fingerprint {
	if fp == nil {
		return nil
	}
	t := NormalizeText(strings.Join(texts, " "))
	if t == "" {
		return nil
	}
	return fp.Fingerprint([]byte(domainResult + t))
}

// TextFingerprint is the text_fp of a message text (§4.4 R4). Nil when fp is
// nil or the text is empty after normalisation.
func TextFingerprint(fp agenttrace.Fingerprinter, text string) agenttrace.Fingerprint {
	if fp == nil {
		return nil
	}
	t := NormalizeText(text)
	if t == "" {
		return nil
	}
	return fp.Fingerprint([]byte(domainText + t))
}

// NormalizeText is the §4.4 text normalisation: Unicode NFC, whitespace runs
// collapsed to one space, trimmed.
func NormalizeText(s string) string {
	s = norm.NFC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}
